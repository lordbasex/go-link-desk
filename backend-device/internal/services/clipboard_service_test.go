// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBoard is a clipboard in memory.
type fakeBoard struct {
	mu    sync.Mutex
	n     int
	text  string
	other bool // holds something that is not text
}

func (b *fakeBoard) ChangeCount() int { b.mu.Lock(); defer b.mu.Unlock(); return b.n }
func (b *fakeBoard) Text() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text, !b.other
}
func (b *fakeBoard) SetText(s string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n++
	b.text, b.other = s, false
	return b.n
}

// copyOnHost is the person at the host copying text.
func (b *fakeBoard) copyOnHost(s string) { b.SetText(s) }

type sentMsgs struct {
	mu   sync.Mutex
	msgs []clipboardMsg
}

func (s *sentMsgs) send(peer string, b []byte) bool {
	var m clipboardMsg
	_ = json.Unmarshal(b, &m)
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()
	return true
}

func (s *sentMsgs) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, m := range s.msgs {
		b.WriteString(m.Text)
	}
	return b.String()
}

func newClipboard(t *testing.T) (*ClipboardService, *fakeBoard, *sentMsgs, *[]string) {
	b := &fakeBoard{n: 1, text: "already there"}
	sent := &sentMsgs{}
	var pasted []string
	c := NewClipboardService(b, sent.send, func(p string) { pasted = append(pasted, p) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.every = 5 * time.Millisecond
	t.Cleanup(func() { c.Allow("P", false) })
	return c, b, sent, &pasted
}

func TestClipboardGoesToTheViewerOnlyWhenCopiedAndAllowed(t *testing.T) {
	c, b, sent, _ := newClipboard(t)
	b.copyOnHost("before")
	time.Sleep(30 * time.Millisecond)
	if sent.text() != "" {
		t.Fatal("sent before the viewer was allowed")
	}
	c.Allow("P", true)
	time.Sleep(30 * time.Millisecond)
	if sent.text() != "" {
		t.Fatal("what was already on the clipboard was sent")
	}
	long := strings.Repeat("ñ", ClipboardPiece+1) // 2 bytes each: two pieces and a bit
	b.copyOnHost(long)
	deadline := time.Now().Add(2 * time.Second)
	for sent.text() != long && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sent.text() != long {
		t.Fatalf("the viewer got %d bytes of %d", len(sent.text()), len(long))
	}
	if n := len(sent.msgs); n != 3 || sent.msgs[0].Parts != 3 || sent.msgs[2].Part != 2 {
		t.Fatalf("pieces %d", n)
	}
	c.Allow("P", false)
	before := len(sent.msgs)
	b.copyOnHost("after")
	time.Sleep(30 * time.Millisecond)
	if len(sent.msgs) != before {
		t.Fatal("sent after the permission was taken back")
	}
}

func TestClipboardFromTheViewerLandsAndPastes(t *testing.T) {
	c, b, sent, pasted := newClipboard(t)
	c.Allow("P", true)
	text := strings.Repeat("a", ClipboardPiece) + "日本"
	pieces := clipboardPieces(7, text, true)
	for _, m := range pieces {
		c.Handle("Q", m) // another peer: ignored
	}
	if b.text != "already there" {
		t.Fatal("a peer without the permission wrote the clipboard")
	}
	for _, m := range pieces {
		c.Handle("P", m)
	}
	if b.text != text || len(*pasted) != 1 || (*pasted)[0] != "P" {
		t.Fatalf("clipboard %d bytes, pasted %v", len(b.text), *pasted)
	}
	// The host's own write does not echo back to the viewer.
	time.Sleep(30 * time.Millisecond)
	if sent.text() != "" {
		t.Fatal("the viewer's own text came back")
	}
	// A piece out of order drops the text.
	ms := clipboardPieces(8, text+"x", false)
	c.Handle("P", ms[1])
	c.Handle("P", ms[0])
	if b.text != text {
		t.Fatal("a text with a missing piece was written")
	}
}

func TestClipboardPiecesNeverSplitACharacter(t *testing.T) {
	text := "a" + strings.Repeat("日", ClipboardPiece) // 3 bytes each, misaligned
	var got strings.Builder
	for _, b := range clipboardPieces(1, text, false) {
		var m clipboardMsg
		if err := json.Unmarshal(b, &m); err != nil || len(m.Text) > ClipboardPiece || strings.ContainsRune(m.Text, '�') {
			t.Fatalf("piece %d bytes, err %v", len(m.Text), err)
		}
		got.WriteString(m.Text)
	}
	if got.String() != text {
		t.Fatal("the pieces do not make the text")
	}
}

func TestFileBoardChangesWithTheFile(t *testing.T) {
	path := t.TempDir() + "/clip.txt"
	b := &FileBoard{Path: path}
	n := b.ChangeCount()
	if err := os.WriteFile(path, []byte("copied on the host"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b.ChangeCount() == n {
		t.Fatal("a new file did not count as a copy")
	}
	m := b.SetText("pasted")
	if b.ChangeCount() != m {
		t.Fatal("the host's own write counted twice")
	}
	if s, ok := b.Text(); !ok || s != "pasted" {
		t.Fatalf("%q %v", s, ok)
	}
}
