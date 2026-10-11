// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"bytes"
	"os"
	"sync"
)

// FileBoard is a clipboard kept in a file, for the end-to-end tests
// (--test-clipboard-file): the test writes the file to "copy on the host"
// and reads it to see what the viewer pasted, and the real clipboard of
// the computer running the tests is never touched.
type FileBoard struct {
	Path string

	mu   sync.Mutex
	last []byte
	n    int
}

// ChangeCount moves on when the file's content changes.
func (b *FileBoard) ChangeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, err := os.ReadFile(b.Path)
	if err == nil && !bytes.Equal(data, b.last) {
		b.last = data
		b.n++
	}
	return b.n
}

// Text is the file's content.
func (b *FileBoard) Text() (string, bool) {
	data, err := os.ReadFile(b.Path)
	return string(data), err == nil
}

// SetText writes the file.
func (b *FileBoard) SetText(s string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	_ = os.WriteFile(b.Path, []byte(s), 0o600)
	b.last = []byte(s)
	b.n++
	return b.n
}
