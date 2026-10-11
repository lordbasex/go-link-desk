// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
)

// testViewerTimeout bounds a test viewer's run.
const testViewerTimeout = 60 * time.Second

// runTestViewer connects to another computer as the app's viewer does,
// without a window: through the door asked for, it takes the picture until
// it has the frames asked for, keeps a link it is given (remotes.json),
// and leaves. End-to-end tests only.
func runTestViewer(f flags, cfg models.Config, signalURL, configDir string, logger *slog.Logger) error {
	file := repositories.NewRemotesFile(configDir)
	known, err := file.Load()
	if err != nil {
		return err
	}
	remotes := services.NewRemotesService(known, file.Save, logger)
	door := viewer.Door{Code: f.testViewerCode}
	switch f.testViewer {
	case "join":
		door.Kind = viewer.DoorJoin
	case "claim":
		door.Kind = viewer.DoorClaim
	case "reach":
		door.Kind = viewer.DoorReach
		r := remotes.Get()
		if len(r.Devices) == 0 {
			return errors.New("test viewer: no linked computer to reach")
		}
		d := r.Devices[0]
		door.Device = &d
	}
	ctx, cancel := context.WithTimeout(context.Background(), testViewerTimeout)
	defer cancel()
	var (
		mu     sync.Mutex
		frames int
		final  viewer.Status
		host   viewer.HostInfo
	)
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }
	var s *viewer.Session
	s = viewer.New(viewer.Options{
		Door: door, Name: "test viewer", Agent: "go-link Desk " + version, Codecs: []string{"h264-high", "h264", "vp8"},
		SignalURL: signalURL, OwnDeviceID: cfg.DeviceID, Logger: logger,
		Events: viewer.Events{
			Status: func(st viewer.Status) {
				logger.Info("viewer status", "status", st.Kind, "reason", st.Reason, "password_error", st.PasswordError)
				if st.Kind == viewer.StatusPassword && !st.Checking {
					if st.PasswordError != "" {
						// One try: a wrong password ends the run.
						mu.Lock()
						final = st
						mu.Unlock()
						finish()
						return
					}
					go s.SendPassword(f.testViewerPassword)
				}
				if st.Over() {
					mu.Lock()
					final = st
					mu.Unlock()
					finish()
				}
			},
			Host: func(h viewer.HostInfo) {
				mu.Lock()
				host = h
				mu.Unlock()
				logger.Info("viewer host", "name", h.Name, "os", h.OS)
			},
			Video: func(fr viewer.Frame) {
				mu.Lock()
				frames++
				n := frames
				mu.Unlock()
				if n == f.testViewerFrames {
					logger.Info("viewer frames", "frames", n, "codec", fr.Codec)
					finish()
				}
			},
			Linked: func(l viewer.NewLink) {
				mu.Lock()
				h := host
				mu.Unlock()
				remotes.Link(models.RemoteDevice{DeviceID: l.DeviceID, LinkID: l.LinkID, Token: l.Token, SignalURL: signalURL, Name: h.Name, OS: h.OS})
				logger.Info("viewer linked", "device_id", l.DeviceID)
			},
			Unlinked: func() {
				if door.Device != nil {
					remotes.Forget(door.Device.DeviceID)
				}
			},
		},
	})
	s.Dial(ctx)
	select {
	case <-done:
	case <-ctx.Done():
	}
	mu.Lock()
	got, st := frames, final
	mu.Unlock()
	if got >= f.testViewerFrames && f.testViewerFrames > 0 {
		// Leave as a viewer does, after the link (if any) was kept.
		time.Sleep(300 * time.Millisecond)
		s.HangUp()
		time.Sleep(300 * time.Millisecond)
		return nil
	}
	s.HangUp()
	if st.Kind == "" {
		st = s.Status()
	}
	return fmt.Errorf("test viewer: %d frames, ended %s %s %s", got, st.Kind, st.Reason, st.PasswordError)
}
