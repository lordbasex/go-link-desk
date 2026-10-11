// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// The files channel's pace, as the website keeps it (docs/protocol.md,
// section 6).
const (
	uploadChunk      = 16 << 10
	uploadWindow     = 1 << 20
	downloadChunk    = 60 << 10
	downloadInFlight = 16
	filesReplyWait   = 30 * time.Second
	maxEntries       = 5000
)

// ErrFilesClosed: the session's files channel closed.
var ErrFilesClosed = errors.New("viewer: the connection closed")

// FileEntry is one entry of a folder on the computer.
type FileEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// filesChannel is what Files needs of a DataChannel (tests stand in).
type filesChannel interface {
	SendText(string) error
	Send([]byte) error
	BufferedAmount() uint64
	SetBufferedAmountLowThreshold(uint64)
	OnBufferedAmountLow(func())
	ReadyState() webrtc.DataChannelState
}

// Files moves files to and from the computer, on the session's "files"
// channel: one upload and one download at a time, as the computer allows.
type Files struct {
	ch filesChannel

	mu      sync.Mutex
	seq     int
	waiting map[string]chan filesReply
	binary  func([]byte) // the download being read
	low     chan struct{}
	done    bool
	upMu    sync.Mutex // one upload at a time
	downMu  sync.Mutex // one download at a time
}

type filesReply struct {
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	OK      bool        `json:"ok"`
	Name    string      `json:"name"`
	Size    int64       `json:"size"`
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
	Error   string      `json:"error"`
}

func newFiles(dc *webrtc.DataChannel) *Files {
	return newFilesOn(dc)
}

func newFilesOn(ch filesChannel) *Files {
	f := &Files{ch: ch, waiting: map[string]chan filesReply{}, low: make(chan struct{}, 1)}
	ch.SetBufferedAmountLowThreshold(uploadWindow / 2)
	ch.OnBufferedAmountLow(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.done {
			return
		}
		select {
		case f.low <- struct{}{}:
		default:
		}
	})
	return f
}

func (f *Files) onMessage(m webrtc.DataChannelMessage) {
	if !m.IsString {
		f.mu.Lock()
		fn := f.binary
		f.mu.Unlock()
		if fn != nil {
			fn(m.Data)
		}
		return
	}
	var r filesReply
	if json.Unmarshal(m.Data, &r) != nil {
		return
	}
	f.mu.Lock()
	ch := f.waiting[r.ID]
	f.mu.Unlock()
	if ch != nil {
		select {
		case ch <- r:
		default:
		}
	}
}

// closed fails everything waiting.
func (f *Files) closed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done = true
	for id, ch := range f.waiting {
		select {
		case ch <- filesReply{Type: "files_error", ID: id, Error: ErrFilesClosed.Error()}:
		default:
		}
	}
	close(f.low)
}

func (f *Files) open(prefix string) (string, chan filesReply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return "", nil, ErrFilesClosed
	}
	f.seq++
	id := fmt.Sprintf("%s%d", prefix, f.seq)
	ch := make(chan filesReply, 4)
	f.waiting[id] = ch
	return id, ch, nil
}

func (f *Files) forget(id string) {
	f.mu.Lock()
	delete(f.waiting, id)
	f.mu.Unlock()
}

func (f *Files) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if f.ch.ReadyState() != webrtc.DataChannelStateOpen {
		return ErrFilesClosed
	}
	return f.ch.SendText(string(b))
}

// wait takes the reply of one of types (or an error) to id.
func wait(ch chan filesReply, types ...string) (filesReply, error) {
	timeout := time.NewTimer(filesReplyWait)
	defer timeout.Stop()
	for {
		select {
		case r := <-ch:
			if r.Type == "files_error" || r.Type == "download_error" {
				return r, errors.New(r.Error)
			}
			for _, t := range types {
				if r.Type == t {
					return r, nil
				}
			}
		case <-timeout.C:
			return filesReply{}, errors.New("the computer did not answer")
		}
	}
}

// List lists a folder of the computer's home folder ("" is home itself).
func (f *Files) List(path string) ([]FileEntry, error) {
	id, ch, err := f.open("l")
	if err != nil {
		return nil, err
	}
	defer f.forget(id)
	if err := f.send(map[string]any{"type": "list", "id": id, "path": path}); err != nil {
		return nil, err
	}
	r, err := wait(ch, "listing")
	if err != nil {
		return nil, err
	}
	if len(r.Entries) > maxEntries {
		r.Entries = r.Entries[:maxEntries]
	}
	return r.Entries, nil
}

// Upload sends a local file; progress gets the bytes sent. It returns the
// name the computer saved it as. stop (closed) stops it.
func (f *Files) Upload(path string, progress func(sent, size int64), stop <-chan struct{}) (string, error) {
	f.upMu.Lock()
	defer f.upMu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("folders cannot be sent")
	}
	id, ch, err := f.open("u")
	if err != nil {
		return "", err
	}
	defer f.forget(id)
	size := info.Size()
	if err := f.send(map[string]any{"type": "begin", "id": id, "name": filepath.Base(path), "size": size}); err != nil {
		return "", err
	}
	buf := make([]byte, uploadChunk)
	var sent int64
	for sent < size {
		select {
		case <-stop:
			_ = f.send(map[string]any{"type": "abort", "id": id})
			return "", errors.New("stopped")
		case r := <-ch: // the computer refused it: no point sending the rest
			if !r.OK {
				return "", errors.New(r.Error)
			}
		default:
		}
		if err := f.drained(); err != nil {
			return "", err
		}
		n, err := io.ReadFull(file, buf)
		if n > 0 {
			if err := f.ch.Send(buf[:n]); err != nil {
				return "", ErrFilesClosed
			}
			sent += int64(n)
			if progress != nil {
				progress(sent, size)
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			_ = f.send(map[string]any{"type": "abort", "id": id})
			return "", err
		}
	}
	if err := f.send(map[string]any{"type": "end", "id": id}); err != nil {
		return "", err
	}
	r, err := wait(ch, "upload_result")
	if err != nil {
		return "", err
	}
	if !r.OK {
		return "", errors.New(r.Error)
	}
	if r.Name == "" {
		r.Name = filepath.Base(path)
	}
	return r.Name, nil
}

// drained waits until the channel holds at most uploadWindow: the
// session's control and input messages pass the upload.
func (f *Files) drained() error {
	for f.ch.BufferedAmount() > uploadWindow {
		if f.ch.ReadyState() != webrtc.DataChannelStateOpen {
			return ErrFilesClosed
		}
		select {
		case _, ok := <-f.low:
			if !ok {
				return ErrFilesClosed
			}
		case <-time.After(time.Second):
		}
	}
	return nil
}

// Download fetches a file of the computer into dir, never over an existing
// file; progress gets the bytes received. It returns the path written.
func (f *Files) Download(path, dir string, progress func(got, size int64), stop <-chan struct{}) (string, error) {
	f.downMu.Lock()
	defer f.downMu.Unlock()
	id, ch, err := f.open("d")
	if err != nil {
		return "", err
	}
	defer f.forget(id)
	if err := f.send(map[string]any{"type": "download", "id": id, "path": path}); err != nil {
		return "", err
	}
	ready, err := wait(ch, "download_ready")
	if err != nil {
		return "", err
	}
	name := safeName(ready.Name)
	if name == "" {
		name = safeName(filepath.Base(path))
	}
	tmp, err := os.CreateTemp(dir, ".go-link-desk-*.part")
	if err != nil {
		_ = f.send(map[string]any{"type": "cancel", "id": id})
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	size := ready.Size
	type piece struct {
		off  int64
		data []byte
	}
	pieces := make(chan piece, downloadInFlight*2)
	f.mu.Lock()
	f.binary = func(b []byte) {
		if len(b) < 8 {
			return
		}
		data := make([]byte, len(b)-8)
		copy(data, b[8:])
		select {
		case pieces <- piece{off: int64(binary.BigEndian.Uint64(b)), data: data}:
		default:
		}
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.binary = nil
		f.mu.Unlock()
	}()
	var next, got int64
	inFlight := 0
	ask := func() error {
		for inFlight < downloadInFlight && next < size {
			n := min(int64(downloadChunk), size-next)
			if err := f.send(map[string]any{"type": "read", "id": id, "offset": next, "length": n}); err != nil {
				return err
			}
			next += n
			inFlight++
		}
		return nil
	}
	if err := ask(); err != nil {
		return "", err
	}
	idle := time.NewTimer(filesReplyWait)
	defer idle.Stop()
	for got < size {
		select {
		case <-stop:
			_ = f.send(map[string]any{"type": "cancel", "id": id})
			return "", errors.New("stopped")
		case r := <-ch:
			if r.Type == "download_error" || r.Type == "files_error" {
				return "", errors.New(r.Error)
			}
		case p := <-pieces:
			if p.off < 0 || p.off+int64(len(p.data)) > size {
				_ = f.send(map[string]any{"type": "cancel", "id": id})
				return "", errors.New("the computer sent a bad piece")
			}
			if _, err := tmp.WriteAt(p.data, p.off); err != nil {
				_ = f.send(map[string]any{"type": "cancel", "id": id})
				return "", err
			}
			got += int64(len(p.data))
			inFlight--
			if progress != nil {
				progress(got, size)
			}
			if err := ask(); err != nil {
				return "", err
			}
			idle.Reset(filesReplyWait)
		case <-idle.C:
			return "", errors.New("the computer stopped sending")
		}
	}
	_ = f.send(map[string]any{"type": "done", "id": id})
	if err := tmp.Close(); err != nil {
		return "", err
	}
	dest := freePath(dir, name)
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	ok = true
	return dest, nil
}

// safeName keeps a file name without folders, control characters or
// leading dots.
func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == ':' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimLeft(name, ".")
	if name == "" || name == "/" {
		return ""
	}
	return name
}

// freePath is dir/name, or dir/name (2).ext and so on if taken.
func freePath(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}
