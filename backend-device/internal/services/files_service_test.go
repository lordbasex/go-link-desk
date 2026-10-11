// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type filesWire struct {
	mu   sync.Mutex
	text []map[string]any
	bin  [][]byte
}

func (w *filesWire) send(_ string, isString bool, data []byte) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if isString {
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		w.text = append(w.text, m)
	} else {
		w.bin = append(w.bin, append([]byte(nil), data...))
	}
	return true
}

func (w *filesWire) last() map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.text) == 0 {
		return nil
	}
	return w.text[len(w.text)-1]
}

func newFiles(t *testing.T) (*FilesService, string, *filesWire) {
	t.Helper()
	home := t.TempDir()
	w := &filesWire{}
	f := NewFilesService(home, filepath.Join(home, "Downloads", ReceivedFolder), w.send, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.Allow("P", true)
	return f, home, w
}

func msg(v map[string]any) []byte { b, _ := json.Marshal(v); return b }

func sendFile(f *FilesService, id, name string, content []byte) {
	f.Handle("P", true, msg(map[string]any{"type": "begin", "id": id, "name": name, "size": len(content)}))
	for len(content) > 0 {
		n := min(len(content), 16<<10)
		f.Handle("P", false, content[:n])
		content = content[n:]
	}
	f.Handle("P", true, msg(map[string]any{"type": "end", "id": id}))
}

func TestUploadsLandWithoutOverwriting(t *testing.T) {
	f, home, w := newFiles(t)
	dir := filepath.Join(home, "Downloads", ReceivedFolder)
	data := bytes.Repeat([]byte("ñandú "), 10000)
	sendFile(f, "u1", "../../informe.pdf", data) // a folder in the name is dropped
	if r := w.last(); r["type"] != "upload_result" || r["ok"] != true || r["name"] != "informe.pdf" {
		t.Fatalf("%v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "informe.pdf")); !bytes.Equal(got, data) {
		t.Fatal("the file is not what was sent")
	}
	sendFile(f, "u2", "informe.pdf", []byte("second"))
	if r := w.last(); r["name"] != "informe (2).pdf" {
		t.Fatalf("%v", r)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "informe.pdf")); !bytes.Equal(got, data) {
		t.Fatal("the first file was overwritten")
	}
	// Incomplete: nothing stays behind, not even the temporary file.
	f.Handle("P", true, msg(map[string]any{"type": "begin", "id": "u3", "name": "x.bin", "size": 100}))
	f.Handle("P", false, []byte("short"))
	f.Handle("P", true, msg(map[string]any{"type": "end", "id": "u3"}))
	if r := w.last(); r["ok"] != false {
		t.Fatalf("%v", r)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 2 {
		t.Fatalf("left in the folder: %v", ents)
	}
	// Too big is refused at once.
	f.Handle("P", true, msg(map[string]any{"type": "begin", "id": "u4", "name": "huge", "size": int64(MaxUpload) + 1}))
	if r := w.last(); r["ok"] != false {
		t.Fatalf("%v", r)
	}
}

func TestFilesNeedThePermission(t *testing.T) {
	f, home, w := newFiles(t)
	f.Allow("P", false)
	sendFile(f, "u1", "a.txt", []byte("hi"))
	if r := w.last(); r["type"] != "files_error" || r["code"] != "not_allowed" {
		t.Fatalf("%v", r)
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads")); !os.IsNotExist(err) {
		t.Fatal("something was written without the permission")
	}
}

func TestBrowseAndDownloadInsideHomeOnly(t *testing.T) {
	f, home, w := newFiles(t)
	_ = os.MkdirAll(filepath.Join(home, "Documents"), 0o755)
	content := bytes.Repeat([]byte{7}, MaxFileChunk+100)
	_ = os.WriteFile(filepath.Join(home, "Documents", "big.bin"), content, 0o644)
	_ = os.WriteFile(filepath.Join(home, ".secret"), []byte("x"), 0o600)
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "passwd"), []byte("root"), 0o644)
	_ = os.Symlink(outside, filepath.Join(home, "escape"))

	f.Handle("P", true, msg(map[string]any{"type": "list", "id": "l1", "path": ""}))
	r := w.last()
	if r["type"] != "listing" {
		t.Fatalf("%v", r)
	}
	names := []string{}
	for _, e := range r["entries"].([]any) {
		names = append(names, e.(map[string]any)["name"].(string))
	}
	if len(names) != 2 || names[0] != "Documents" || names[1] != "escape" {
		t.Fatalf("listing %v (hidden files must not show)", names)
	}
	for _, p := range []string{"../", "escape/passwd", "escape", "/etc/passwd"} {
		f.Handle("P", true, msg(map[string]any{"type": "download", "id": "x", "path": p}))
		if got := w.last(); got["type"] != "download_error" {
			t.Fatalf("%q: %v", p, got)
		}
		f.Handle("P", true, msg(map[string]any{"type": "list", "id": "x", "path": p}))
		if got := w.last(); got["type"] != "files_error" {
			t.Fatalf("list %q: %v", p, got)
		}
	}

	f.Handle("P", true, msg(map[string]any{"type": "download", "id": "d1", "path": "Documents/big.bin"}))
	if r := w.last(); r["type"] != "download_ready" || r["size"] != float64(len(content)) {
		t.Fatalf("%v", r)
	}
	var got []byte
	for off := 0; off < len(content); off += MaxFileChunk {
		f.Handle("P", true, msg(map[string]any{"type": "read", "id": "d1", "offset": off, "length": MaxFileChunk}))
		b := w.bin[len(w.bin)-1]
		if binary.BigEndian.Uint64(b) != uint64(off) {
			t.Fatalf("offset %d", binary.BigEndian.Uint64(b))
		}
		got = append(got, b[8:]...)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded %d bytes of %d", len(got), len(content))
	}
	f.Handle("P", true, msg(map[string]any{"type": "done", "id": "d1"}))
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{"a.pdf": "a.pdf", "../../etc/passwd": "passwd", `C:\x\y.txt`: "y.txt", ".hidden": "hidden", "a\u202eb.exe": "ab.exe", "": "file"} {
		if got := cleanName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
