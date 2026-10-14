// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// wire joins a Files client to the host's FilesService, as the files
// channel would, in order.
type wire struct {
	host  *services.FilesService
	files *Files
	mu    sync.Mutex
	queue chan func()
}

func (w *wire) SendText(s string) error {
	w.queue <- func() { w.host.Handle(viewerID, true, []byte(s)) }
	return nil
}

func (w *wire) Send(b []byte) error {
	c := append([]byte(nil), b...)
	w.queue <- func() { w.host.Handle(viewerID, false, c) }
	return nil
}

func (w *wire) BufferedAmount() uint64               { return 0 }
func (w *wire) SetBufferedAmountLowThreshold(uint64) {}
func (w *wire) OnBufferedAmountLow(func())           {}
func (w *wire) ReadyState() webrtc.DataChannelState  { return webrtc.DataChannelStateOpen }

func newWire(t *testing.T) (*Files, string) {
	t.Helper()
	home := t.TempDir()
	w := &wire{queue: make(chan func(), 4096)}
	w.host = services.NewFilesService(home, filepath.Join(home, "Downloads", services.ReceivedFolder), func(_ string, isString bool, data []byte) bool {
		c := append([]byte(nil), data...)
		go w.files.onMessage(webrtc.DataChannelMessage{IsString: isString, Data: c})
		return true
	}, quiet)
	w.host.Allow(viewerID, true)
	w.files = newFilesOn(w)
	go func() {
		for fn := range w.queue {
			fn()
		}
	}()
	return w.files, home
}

func TestFilesGoBothWays(t *testing.T) {
	f, home := newWire(t)
	local := t.TempDir()

	// Upload: the computer keeps it in Downloads/go-link-desk.
	data := make([]byte, 300<<10+123)
	_, _ = rand.Read(data)
	src := filepath.Join(local, "report.pdf")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var sent int64
	name, err := f.Upload(src, func(n, _ int64) { sent = n }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name != "report.pdf" || sent != int64(len(data)) {
		t.Fatalf("saved as %q, sent %d", name, sent)
	}
	got, err := os.ReadFile(filepath.Join(home, "Downloads", services.ReceivedFolder, "report.pdf"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("the computer's copy differs (%v)", err)
	}

	// Browse and download it back, next to the original without
	// overwriting it.
	entries, err := f.List("Downloads/" + services.ReceivedFolder)
	if err != nil || len(entries) != 1 || entries[0].Name != "report.pdf" || entries[0].Size != int64(len(data)) {
		t.Fatalf("listing %+v %v", entries, err)
	}
	dest, err := f.Download("Downloads/"+services.ReceivedFolder+"/report.pdf", local, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "report (2).pdf" {
		t.Fatalf("saved at %s", dest)
	}
	back, _ := os.ReadFile(dest)
	if !bytes.Equal(back, data) {
		t.Fatal("the download differs")
	}

	// The computer refuses what leaves its home folder.
	if _, err := f.List("../"); err == nil {
		t.Fatal("listed outside the home folder")
	}
	if _, err := f.Download("../etc/passwd", local, nil, nil); err == nil {
		t.Fatal("downloaded outside the home folder")
	}
}

func TestDownloadNamesAreMadeSafe(t *testing.T) {
	for in, want := range map[string]string{
		"a.txt":         "a.txt",
		"../../evil.sh": "evil.sh",
		".hidden":       "hidden",
		"dir\\x.txt":    "x.txt",
		"bad\x00name":   "badname",
		"..":            "",
	} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}
