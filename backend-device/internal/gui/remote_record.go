// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// maxRecording is when a recording stops by itself (as the website's).
const maxRecording = 2 * time.Hour

// recState is a session's recording, while there is one.
type recState struct {
	since time.Time
	path  string
	limit *time.Timer
}

// captureDir is where screenshots and recordings land: Downloads.
func captureDir() (string, error) {
	dir := downloadsDir()
	return dir, os.MkdirAll(dir, 0o755)
}

// captureName names a file after the computer and the time, without
// spaces: "go-link-desk-office-mac-2026-10-14-150405.png".
func captureName(host string, at time.Time, ext string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(host) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteRune('-')
			dash = true
		}
	}
	clean := []rune(b.String())
	if len(clean) > 40 {
		clean = clean[:40]
	}
	name := "go-link-desk-"
	if c := strings.Trim(string(clean), "-"); c != "" {
		name += c + "-"
	}
	return name + at.Format("2006-01-02-150405") + "." + ext
}

// recordMenu is the toolbar's Screenshot and recording menu.
func (r *remote) recordMenu() []remoteview.MenuItem {
	r.mu.Lock()
	rec := r.rec
	r.mu.Unlock()
	record := remoteview.MenuItem{ID: "rec:start", Title: L("Record the screen")}
	if rec != nil {
		record = remoteview.MenuItem{ID: "rec:stop", Title: Lf("Stop recording (%s)", clock(time.Since(rec.since)))}
	}
	return []remoteview.MenuItem{
		{ID: "rec:shot", Title: L("Take a screenshot")},
		record,
		{Separator: true},
		{ID: "rec:folder", Title: L("Show them in Finder")},
		{Title: L("The person at the computer sees when you take a screenshot or record."), Disabled: true},
	}
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (r *remote) chooseRecord(id string) {
	switch id {
	case "rec:shot":
		go r.screenshot()
	case "rec:start":
		go r.startRecording()
	case "rec:stop":
		go r.stopRecording(L("Recording saved in Downloads."))
	case "rec:folder":
		if dir, err := captureDir(); err == nil {
			_ = exec.Command("open", dir).Start()
		}
	}
}

// screenshot asks the computer, then saves the picture as shown.
func (r *remote) screenshot() {
	r.mu.Lock()
	host := r.host.Name
	r.mu.Unlock()
	at := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !r.s.AskScreenshot(ctx) {
		r.notify(L("The computer did not allow it: the person there must tick “Take screenshots and record the screen”."))
		return
	}
	dir, err := captureDir()
	if err != nil {
		r.notify(L("The screenshot could not be saved."))
		return
	}
	r.view.Screenshot(filepath.Join(dir, captureName(host, at, "png")))
}

// startRecording asks the computer, and records only once it said yes.
func (r *remote) startRecording() {
	r.mu.Lock()
	if r.rec != nil {
		r.mu.Unlock()
		return
	}
	host, audio := r.host.Name, r.perms.Audio
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !r.s.StartRecording(ctx) {
		r.notify(L("The computer did not allow it: the person there must tick “Take screenshots and record the screen”."))
		return
	}
	dir, err := captureDir()
	if err != nil {
		r.s.StopRecording()
		r.notify(L("The recording could not be saved."))
		return
	}
	path := filepath.Join(dir, captureName(host, time.Now(), "mp4"))
	rec := &recState{since: time.Now(), path: path}
	r.mu.Lock()
	r.rec = rec
	r.mu.Unlock()
	// The computer draws its pointer into the picture, so the recording
	// has it; a whole picture starts the file.
	r.s.SetCursorInVideo(true)
	r.view.CursorPos(0, 0, false)
	r.view.StartRecording(path, audio)
	r.s.RequestKeyframe()
	r.view.ToolState("record", true, true)
	rec.limit = time.AfterFunc(maxRecording, func() {
		r.stopRecording(L("The recording reached its 2-hour limit and was saved in Downloads."))
	})
}

// stopRecording closes the file and tells the computer; said is shown
// once the file is written.
func (r *remote) stopRecording(said string) {
	r.mu.Lock()
	rec := r.rec
	r.rec = nil
	if rec != nil {
		r.recSaid = said
	}
	allowed := r.perms.Record
	r.mu.Unlock()
	if rec == nil {
		return
	}
	if rec.limit != nil {
		rec.limit.Stop()
	}
	r.view.StopRecording()
	r.s.StopRecording()
	r.s.SetCursorInVideo(false)
	r.view.ToolState("record", false, allowed)
}

// recording says whether this session is being recorded here.
func (r *remote) recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rec != nil
}

// captured takes the window's answers: a screenshot written, a recording
// closed ("" or why not).
func (r *remote) captured(kind, why string) {
	switch {
	case kind == "screenshot" && why == "":
		r.flash(L("Screenshot saved in Downloads."))
	case kind == "screenshot":
		r.notify(L("The screenshot could not be saved.") + " (" + why + ")")
	case why == "":
		r.mu.Lock()
		said := r.recSaid
		r.mu.Unlock()
		r.flash(said)
	default:
		r.notify(L("The recording could not be saved.") + " (" + why + ")")
	}
}

// flash shows a line over the picture for a few seconds.
func (r *remote) flash(text string) {
	r.view.HUD(" " + text + " ")
	time.AfterFunc(4*time.Second, func() {
		r.mu.Lock()
		hud := r.hud
		r.mu.Unlock()
		if hud {
			r.view.HUD(r.hudText())
		} else {
			r.view.HUD("")
		}
	})
}
