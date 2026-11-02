// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// guarded is FilesService behind FilesGuard, wired as the engine does, with a
// secret beside the home folder.
func guarded(t *testing.T) (g *FilesGuard, w *filesWire, home, outside string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, outside = filepath.Join(root, "home"), filepath.Join(root, "outside")
	for _, d := range []string{home, outside, filepath.Join(home, "Documents")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(outside, "secret.txt"), "secret")
	write(t, filepath.Join(home, "Documents", "ok.txt"), "fine")
	w = &filesWire{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g = &FilesGuard{Home: home, Downloads: filepath.Join(home, "Downloads"), Send: w.send, Log: log}
	f := NewFilesService(home, g.Downloads, g.SendFiles, log)
	f.Allow("p", true)
	g.Next = f.Handle
	return g, w, home, outside
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, p string) {
	t.Helper()
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
}

func ask(g *FilesGuard, v map[string]any) {
	b, _ := json.Marshal(v)
	g.HandleFiles("p", true, b)
}

func TestCheckHomePath(t *testing.T) {
	for _, p := range []string{"/", "/etc", "/etc/passwd", "..", "../..", "../outside", "Documents/../..", "a/../../b", "~", "~/x", "~root", "\\etc", "a\\..\\..", "a\x00b"} {
		if CheckHomePath(p) {
			t.Errorf("%q passed", p)
		}
	}
	for _, p := range []string{"", ".", "Documents", "Documents/a b.txt", "a..b", "..a", "a~"} {
		if !CheckHomePath(p) {
			t.Errorf("%q refused", p)
		}
	}
}

func TestVisitorCannotClimb(t *testing.T) {
	g, w, _, _ := guarded(t)
	for i, p := range []string{"/", "/etc", "..", "../outside", "../outside/secret.txt", "Documents/../../outside", "~", "~/Documents"} {
		for _, kind := range []string{"list", "download"} {
			id := fmt.Sprintf("%s%d", kind, i)
			ask(g, map[string]any{"type": kind, "id": id, "path": p})
			m := w.last()
			if m["id"] != id || m["error"] != ReasonOutsideHome {
				t.Errorf("%s %q: %v", kind, p, m)
			}
		}
	}
}

func TestSymlinksOutOfHomeAreRefused(t *testing.T) {
	g, w, home, outside := guarded(t)
	link(t, outside, filepath.Join(home, "out"))
	link(t, filepath.Join(outside, "secret.txt"), filepath.Join(home, "Documents", "s.txt"))
	link(t, "../../outside", filepath.Join(home, "Documents", "rel"))
	link(t, "out", filepath.Join(home, "chain"))
	for _, p := range []string{"out", "out/secret.txt", "Documents/s.txt", "Documents/rel", "Documents/rel/secret.txt", "chain/secret.txt"} {
		ask(g, map[string]any{"type": "download", "id": "d", "path": p})
		if m := w.last(); m["error"] != ReasonOutsideHome {
			t.Errorf("download %q: %v", p, m)
		}
		ask(g, map[string]any{"type": "list", "id": "l", "path": p})
		if m := w.last(); m["error"] != ReasonOutsideHome {
			t.Errorf("list %q: %v", p, m)
		}
	}
}

func TestListingHidesLinksOut(t *testing.T) {
	g, w, home, outside := guarded(t)
	link(t, outside, filepath.Join(home, "out"))
	link(t, filepath.Join(home, "Documents"), filepath.Join(home, "docs"))
	ask(g, map[string]any{"type": "list", "id": "l", "path": ""})
	m := w.last()
	names := map[string]bool{}
	for _, e := range m["entries"].([]any) {
		names[e.(map[string]any)["name"].(string)] = true
	}
	if names["out"] || !names["docs"] || !names["Documents"] {
		t.Fatal(names)
	}
}

func TestLinkInsideHomeWorks(t *testing.T) {
	g, w, home, _ := guarded(t)
	link(t, filepath.Join(home, "Documents"), filepath.Join(home, "docs"))
	ask(g, map[string]any{"type": "list", "id": "l", "path": "docs"})
	if m := w.last(); m["type"] != "listing" {
		t.Fatal(m)
	}
}

func TestHiddenFoldersStayHidden(t *testing.T) {
	g, w, home, _ := guarded(t)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".ssh", "id"), "key")
	ask(g, map[string]any{"type": "download", "id": "d", "path": ".ssh/id"})
	if m := w.last(); m["type"] == "download_ready" {
		t.Fatal(m)
	}
}

func send(g *FilesGuard, name, body string) {
	ask(g, map[string]any{"type": "begin", "id": "u", "name": name, "size": len(body)})
	g.HandleFiles("p", false, []byte(body))
	ask(g, map[string]any{"type": "end", "id": "u"})
}

func TestSentFilesGoToDownloads(t *testing.T) {
	g, w, home, outside := guarded(t)
	for _, name := range []string{"../../outside/evil.txt", "/etc/evil.txt", "evil.txt", "..\\..\\evil.txt"} {
		send(g, name, "x")
		if m := w.last(); m["ok"] != true {
			t.Fatalf("%q: %v", name, m)
		}
	}
	ents, _ := os.ReadDir(filepath.Join(home, "Downloads"))
	if len(ents) != 4 {
		t.Fatalf("%d files in Downloads", len(ents))
	}
	for _, p := range []string{filepath.Join(outside, "evil.txt"), filepath.Join(home, "evil.txt")} {
		if _, err := os.Lstat(p); err == nil {
			t.Fatalf("%s written", p)
		}
	}
}

func TestDownloadsLinkedOutOfHomeTakesNothing(t *testing.T) {
	g, w, home, outside := guarded(t)
	link(t, outside, filepath.Join(home, "Downloads"))
	send(g, "evil.txt", "x")
	if _, err := os.Lstat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Fatal("written outside the home folder")
	}
	found := false
	for _, m := range w.text {
		found = found || m["error"] == ReasonOutsideHome
	}
	if !found {
		t.Fatal("not refused")
	}
}

func TestUploadNeverFollowsALinkInDownloads(t *testing.T) {
	g, _, home, outside := guarded(t)
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, filepath.Join(outside, "secret.txt"), filepath.Join(home, "Downloads", "a.txt"))
	send(g, "a.txt", "overwritten")
	b, _ := os.ReadFile(filepath.Join(outside, "secret.txt"))
	if string(b) != "secret" {
		t.Fatal("wrote through the link")
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads", "a (2).txt")); err != nil {
		t.Fatal(err)
	}
}

func TestInsideHomeJudgesAMissingPathByItsFolder(t *testing.T) {
	_, _, home, outside := guarded(t)
	link(t, outside, filepath.Join(home, "out"))
	link(t, filepath.Join(home, "nowhere"), filepath.Join(home, "broken"))
	if !InsideHome(home, filepath.Join(home, "Downloads")) {
		t.Fatal("a folder yet to be made")
	}
	if InsideHome(home, filepath.Join(home, "out", "new")) {
		t.Fatal("under a link out")
	}
	if InsideHome(home, filepath.Join(home, "broken")) {
		t.Fatal("a broken link")
	}
	if InsideHome(home, outside) {
		t.Fatal("outside")
	}
}
