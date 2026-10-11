// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/camera"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/testpattern"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/voice"
)

// SystemCallMedia is the computer's camera and its call sound (the
// microphone with echo cancellation, and the speaker).
type SystemCallMedia struct{}

// StartCamera opens the default camera, asking for the permission the
// first time.
func (SystemCallMedia) StartCamera(onFrame func(Frame)) (func(), error) {
	if !camera.HasAccess() && !camera.RequestAccess() {
		return nil, errNoCameraAccess
	}
	s, err := camera.Start(camera.Config{Width: callCamWidth, Height: callCamHeight, FPS: callCamFPS}, func(f camera.Frame) {
		onFrame(Frame{PixelBuffer: f.PixelBuffer, Width: f.Width, Height: f.Height, PTS: f.PTS})
	})
	if err != nil {
		return nil, err
	}
	return s.Stop, nil
}

// StartVoice opens the speaker and the microphone, asking for the
// microphone's permission the first time (without it, the call only
// plays the other side).
func (SystemCallMedia) StartVoice(mic bool, onMic func([]int16)) (CallVoice, error) {
	if mic && !voice.HasMicAccess() && !voice.RequestMicAccess() {
		mic = false
	}
	e, err := voice.Start(mic, onMic)
	if err != nil {
		return nil, err
	}
	return e, nil
}

type callMediaError string

func (e callMediaError) Error() string { return string(e) }

const errNoCameraAccess = callMediaError("call: this app may not use the camera (System Settings › Privacy & Security › Camera)")

// TestCallMedia stands in for the camera and the microphone (end-to-end
// tests, and hosts without them): the test card at 15 frames a second and
// a 440 Hz tone; what the other side says is counted and logged, not
// played.
type TestCallMedia struct {
	Log *slog.Logger
}

// StartCamera sends the test card.
func (t TestCallMedia) StartCamera(onFrame func(Frame)) (func(), error) {
	ctx, stop := context.WithCancel(context.Background())
	go func() {
		pattern := testpattern.New(callCamWidth, callCamHeight)
		tick := time.NewTicker(time.Second / 15)
		defer tick.Stop()
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				onFrame(Frame{I420: pattern.Frame(now, testpattern.Controls{}), Width: callCamWidth, Height: callCamHeight, PTS: now.Sub(start)})
			}
		}
	}()
	return stop, nil
}

// StartVoice plays a 440 Hz tone as the microphone.
func (t TestCallMedia) StartVoice(mic bool, onMic func([]int16)) (CallVoice, error) {
	ctx, stop := context.WithCancel(context.Background())
	v := &testVoice{stop: stop, log: t.Log}
	if mic {
		go func() {
			tone := testpattern.NewTone(440)
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					v.mu.Lock()
					muted := v.muted
					v.mu.Unlock()
					if !muted {
						onMic(tone.Frame())
					}
				}
			}
		}()
	}
	return v, nil
}

type testVoice struct {
	stop   context.CancelFunc
	log    *slog.Logger
	mu     sync.Mutex
	muted  bool
	played int
}

func (v *testVoice) Play(samples []int16) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.played += len(samples)
}

func (v *testVoice) Mute(m bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.muted = m
}

func (v *testVoice) Stop() {
	v.stop()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.log != nil {
		v.log.Info("call sound stopped", "played_ms", v.played/48)
	}
}
