// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// File transfer limits.
const (
	// MaxUpload is the largest file a viewer may send.
	MaxUpload = 20 << 30
	// MaxFileChunk is the most a download read returns (plus its 8-byte
	// offset): under the 64 KiB every WebRTC stack takes in one message.
	MaxFileChunk = 60 << 10
	// maxListing bounds a folder's entries sent at once.
	maxListing = 5000
	// fileIdle closes a transfer nobody moves any more.
	fileIdle = 2 * time.Minute
	// ReceivedFolder is where a viewer's files land, inside Downloads.
	ReceivedFolder = "go-link Desk"
)

// Transfer is a file going one way, for the window.
type Transfer struct {
	Upload bool // true: from the viewer to the host
	Name   string
	Size   int64
	Done   int64
	Over   bool  // finished, or stopped
	Err    error // why it stopped
}

// FilesService moves files between the viewer allowed to (Permissions
// .Files) and the host, on the "files" DataChannel. Uploads land in the
// host's Downloads/go-link Desk folder, written to a temporary file and
// renamed when complete, never over an existing file. Downloads come from
// the host's home folder, which a viewer can browse but never leave; the
// browser pulls them piece by piece, so it sets the pace. Every file that
// moves is written to the log (who, which, how big, which way).
//
// Protocol (text messages are JSON; one upload and one download at a
// time per viewer):
//
//	viewer  {"type":"begin","id":"u1","name":"a.pdf","size":123}
//	viewer  binary chunks, in order
//	viewer  {"type":"end","id":"u1"}            or {"type":"abort","id":"u1"}
//	host    {"type":"upload_result","id":"u1","ok":true,"name":"a (2).pdf"}
//
//	viewer  {"type":"list","id":"l1","path":"Documents"}
//	host    {"type":"listing","id":"l1","path":"Documents","entries":[{"name","dir","size","mtime"}]}
//	viewer  {"type":"download","id":"d1","path":"Documents/a.pdf"}
//	host    {"type":"download_ready","id":"d1","name":"a.pdf","size":123}
//	viewer  {"type":"read","id":"d1","offset":0,"length":61440}
//	host    binary: offset (8 bytes, big endian) + up to length bytes
//	viewer  {"type":"done","id":"d1"} or {"type":"cancel","id":"d1"}
//	host    {"type":"download_error","id":"d1","error":"…"}  /  {"type":"files_error","id":"l1","error":"…"}
type FilesService struct {
	home     string // what a viewer may browse
	received string // where uploads land
	send     func(peerID string, isString bool, data []byte) bool
	log      *slog.Logger

	mu        sync.Mutex
	allowed   map[string]bool
	uploads   map[string]*upload
	downloads map[string]*download
	onChange  func(Transfer)
}

type upload struct {
	id, name string // name: the one it is saved as
	size     int64
	received int64
	tmp      *os.File
	final    string
	failed   error
	timer    *time.Timer
}

type download struct {
	id    string
	name  string
	file  *os.File
	size  int64
	sent  int64
	timer *time.Timer
}

// NewFilesService builds the service: home is the folder a viewer may
// browse, received where its files land (created when the first comes).
func NewFilesService(home, received string, send func(peerID string, isString bool, data []byte) bool, log *slog.Logger) *FilesService {
	if log == nil {
		log = slog.Default()
	}
	return &FilesService{
		home: filepath.Clean(home), received: received, send: send, log: log,
		allowed: map[string]bool{}, uploads: map[string]*upload{}, downloads: map[string]*download{},
	}
}

// OnChange is told of each transfer's progress (a few times a second at
// most) and end.
func (f *FilesService) OnChange(fn func(Transfer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onChange = fn
}

// Allow gives or takes a viewer's files permission. Taking it stops its
// transfers.
func (f *FilesService) Allow(peerID string, on bool) {
	f.mu.Lock()
	had := f.allowed[peerID]
	if on {
		f.allowed[peerID] = true
	} else {
		delete(f.allowed, peerID)
	}
	f.mu.Unlock()
	if had && !on {
		f.Abort(peerID)
	}
}

type filesMsg struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
}

// Handle takes one message of a viewer's files channel.
func (f *FilesService) Handle(peerID string, isString bool, data []byte) {
	f.mu.Lock()
	ok := f.allowed[peerID]
	f.mu.Unlock()
	if !isString {
		if ok {
			f.chunk(peerID, data)
		}
		return
	}
	var m filesMsg
	if json.Unmarshal(data, &m) != nil || len(m.ID) > 64 {
		return
	}
	if !ok {
		f.reply(peerID, map[string]any{"type": "files_error", "id": m.ID, "error": "not allowed", "code": "not_allowed"})
		return
	}
	switch m.Type {
	case "begin":
		f.begin(peerID, m)
	case "end":
		f.end(peerID, m.ID)
	case "abort":
		f.abortUpload(peerID, errors.New("stopped by the viewer"))
	case "list":
		f.list(peerID, m)
	case "download":
		f.download(peerID, m)
	case "read":
		f.read(peerID, m)
	case "done", "cancel":
		f.closeDownload(peerID, m.ID, m.Type == "done")
	}
}

func (f *FilesService) reply(peerID string, v any) {
	if b, err := json.Marshal(v); err == nil {
		f.send(peerID, true, b)
	}
}

func (f *FilesService) changed(t Transfer) {
	f.mu.Lock()
	fn := f.onChange
	f.mu.Unlock()
	if fn != nil {
		fn(t)
	}
}

// ---- uploads ----

// cleanName keeps a file name a file name: no folders, no control or
// format characters, not hidden, at most 200 bytes.
func cleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == ':' || !unicode.IsPrint(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimLeft(strings.TrimSpace(b.String()), ".")
	for len(name) > 200 {
		_, size := lastRune(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		name = "file"
	}
	return name
}

func lastRune(s string) (rune, int) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i]&0xC0 != 0x80 {
			return rune(s[i]), len(s) - i
		}
	}
	return 0, 1
}

// freeName is name in dir, or "name (2).ext" and on if taken.
func freeName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; ; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		if _, err := os.Lstat(filepath.Join(dir, candidate)); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func (f *FilesService) begin(peerID string, m filesMsg) {
	f.abortUpload(peerID, errors.New("replaced by a new upload"))
	fail := func(err error) {
		f.reply(peerID, map[string]any{"type": "upload_result", "id": m.ID, "ok": false, "error": err.Error()})
		f.mu.Lock()
		f.uploads[peerID] = &upload{id: m.ID, failed: err} // swallow its chunks
		f.mu.Unlock()
	}
	if m.Size < 0 || m.Size > MaxUpload {
		fail(fmt.Errorf("files are limited to %d GB", MaxUpload>>30))
		return
	}
	if err := os.MkdirAll(f.received, 0o755); err != nil {
		fail(err)
		return
	}
	tmp, err := os.CreateTemp(f.received, ".go-link-desk-*.part")
	if err != nil {
		fail(err)
		return
	}
	up := &upload{id: m.ID, name: cleanName(m.Name), size: m.Size, tmp: tmp}
	up.timer = time.AfterFunc(fileIdle, func() { f.abortUpload(peerID, errors.New("nothing arrived for too long")) })
	f.mu.Lock()
	f.uploads[peerID] = up
	f.mu.Unlock()
	f.changed(Transfer{Upload: true, Name: up.name, Size: up.size})
}

func (f *FilesService) chunk(peerID string, data []byte) {
	f.mu.Lock()
	up := f.uploads[peerID]
	f.mu.Unlock()
	if up == nil || up.failed != nil || up.tmp == nil {
		return
	}
	up.timer.Reset(fileIdle)
	before := up.received
	up.received += int64(len(data))
	if up.received > up.size {
		up.failed = errors.New("more arrived than announced")
		return
	}
	if _, err := up.tmp.Write(data); err != nil {
		up.failed = err
		return
	}
	// Progress a few times per percent, not on every chunk.
	if step := max(up.size/200, 1); before/step != up.received/step {
		f.changed(Transfer{Upload: true, Name: up.name, Size: up.size, Done: up.received})
	}
}

func (f *FilesService) end(peerID, id string) {
	f.mu.Lock()
	up := f.uploads[peerID]
	if up != nil && up.id == id {
		delete(f.uploads, peerID)
	}
	f.mu.Unlock()
	if up == nil || up.id != id || up.tmp == nil {
		return // unknown, or already answered at begin
	}
	up.timer.Stop()
	err := up.failed
	if err == nil && up.received != up.size {
		err = errors.New("the file arrived incomplete")
	}
	if cerr := up.tmp.Close(); err == nil && cerr != nil {
		err = cerr
	}
	name := up.name
	if err == nil {
		// Never over an existing file: a free name, claimed by the rename.
		name = freeName(f.received, up.name)
		err = os.Rename(up.tmp.Name(), filepath.Join(f.received, name))
	}
	if err != nil {
		_ = os.Remove(up.tmp.Name())
		f.log.Warn("file from the viewer failed", "peer_id", peerID, "name", up.name, "err", err)
		f.reply(peerID, map[string]any{"type": "upload_result", "id": id, "ok": false, "error": err.Error()})
		f.changed(Transfer{Upload: true, Name: up.name, Size: up.size, Done: up.received, Over: true, Err: err})
		return
	}
	f.log.Info("file received from the viewer", "peer_id", peerID, "name", name, "bytes", up.size, "folder", f.received)
	f.reply(peerID, map[string]any{"type": "upload_result", "id": id, "ok": true, "name": name})
	f.changed(Transfer{Upload: true, Name: name, Size: up.size, Done: up.size, Over: true})
}

func (f *FilesService) abortUpload(peerID string, reason error) {
	f.mu.Lock()
	up := f.uploads[peerID]
	delete(f.uploads, peerID)
	f.mu.Unlock()
	if up == nil || up.tmp == nil {
		return
	}
	up.timer.Stop()
	_ = up.tmp.Close()
	_ = os.Remove(up.tmp.Name())
	f.changed(Transfer{Upload: true, Name: up.name, Size: up.size, Done: up.received, Over: true, Err: reason})
}

// ---- browsing and downloads ----

// resolve maps a path relative to home to the file it names, refusing
// anything that leaves home (.., absolute paths, symbolic links out).
func (f *FilesService) resolve(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) || filepath.IsAbs(rel) {
		return "", errors.New("not a path in the home folder")
	}
	p := filepath.Join(f.home, filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", errors.New("not found")
	}
	home, err := filepath.EvalSymlinks(f.home)
	if err != nil {
		return "", err
	}
	if real != home && !strings.HasPrefix(real, home+string(filepath.Separator)) {
		return "", errors.New("not a path in the home folder")
	}
	return real, nil
}

// fileEntry is one row of a listing.
type fileEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"` // unix seconds
}

func (f *FilesService) list(peerID string, m filesMsg) {
	dir, err := f.resolve(m.Path)
	if err != nil {
		f.reply(peerID, map[string]any{"type": "files_error", "id": m.ID, "error": err.Error()})
		return
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		f.reply(peerID, map[string]any{"type": "files_error", "id": m.ID, "error": err.Error()})
		return
	}
	entries := make([]fileEntry, 0, len(des))
	for _, d := range des {
		if strings.HasPrefix(d.Name(), ".") {
			continue // hidden files stay hidden
		}
		info, err := d.Info()
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if info.Mode()&os.ModeSymlink != 0 {
			st, err := os.Stat(filepath.Join(dir, d.Name()))
			if err != nil {
				continue
			}
			isDir = st.IsDir()
		}
		e := fileEntry{Name: d.Name(), Dir: isDir, MTime: info.ModTime().Unix()}
		if !isDir {
			e.Size = info.Size()
		}
		entries = append(entries, e)
		if len(entries) >= maxListing {
			break
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	f.reply(peerID, map[string]any{"type": "listing", "id": m.ID, "path": m.Path, "entries": entries})
}

func (f *FilesService) download(peerID string, m filesMsg) {
	f.closeDownload(peerID, "", false)
	fail := func(err error) {
		f.reply(peerID, map[string]any{"type": "download_error", "id": m.ID, "error": err.Error()})
	}
	p, err := f.resolve(m.Path)
	if err != nil {
		fail(err)
		return
	}
	file, err := os.Open(p)
	if err != nil {
		fail(err)
		return
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		fail(errors.New("not a file"))
		return
	}
	dl := &download{id: m.ID, name: info.Name(), file: file, size: info.Size()}
	dl.timer = time.AfterFunc(fileIdle, func() { f.closeDownload(peerID, m.ID, false) })
	f.mu.Lock()
	f.downloads[peerID] = dl
	f.mu.Unlock()
	f.reply(peerID, map[string]any{"type": "download_ready", "id": m.ID, "name": dl.name, "size": dl.size})
	f.changed(Transfer{Name: dl.name, Size: dl.size})
}

func (f *FilesService) read(peerID string, m filesMsg) {
	f.mu.Lock()
	dl := f.downloads[peerID]
	f.mu.Unlock()
	if dl == nil || dl.id != m.ID || m.Offset < 0 || m.Offset > dl.size {
		return
	}
	dl.timer.Reset(fileIdle)
	n := min(max(m.Length, 0), MaxFileChunk)
	buf := make([]byte, 8+n)
	binary.BigEndian.PutUint64(buf, uint64(m.Offset))
	got, err := dl.file.ReadAt(buf[8:], m.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		f.reply(peerID, map[string]any{"type": "download_error", "id": m.ID, "error": err.Error()})
		return
	}
	f.send(peerID, false, buf[:8+got])
	f.mu.Lock()
	before := dl.sent
	dl.sent += int64(got)
	sent := dl.sent
	f.mu.Unlock()
	if step := max(dl.size/200, 1); before/step != sent/step {
		f.changed(Transfer{Name: dl.name, Size: dl.size, Done: min(sent, dl.size)})
	}
}

func (f *FilesService) closeDownload(peerID, id string, done bool) {
	f.mu.Lock()
	dl := f.downloads[peerID]
	if dl == nil || (id != "" && dl.id != id) {
		f.mu.Unlock()
		return
	}
	delete(f.downloads, peerID)
	f.mu.Unlock()
	dl.timer.Stop()
	_ = dl.file.Close()
	if done {
		f.log.Info("file sent to the viewer", "peer_id", peerID, "name", dl.name, "bytes", dl.size)
		f.changed(Transfer{Name: dl.name, Size: dl.size, Done: dl.size, Over: true})
		return
	}
	f.changed(Transfer{Name: dl.name, Size: dl.size, Done: min(dl.sent, dl.size), Over: true, Err: errors.New("stopped")})
}

// Abort stops a viewer's transfers (it left, or lost the permission).
func (f *FilesService) Abort(peerID string) {
	f.abortUpload(peerID, errors.New("connection closed"))
	f.closeDownload(peerID, "", false)
}
