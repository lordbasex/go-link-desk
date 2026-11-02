// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"errors"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordlive"
)

// CallTag is the call's number on the live links. The app opens the stream
// and the camera under it; a session to another computer has its own, from
// firstRemoteTag on.
const (
	CallTag        = 1
	firstRemoteTag = 100
)

// watchLive waits for the app to open r's stream, and keeps the helper's
// requests coming to r until it ends.
func (r *remote) watchLive() {
	hub := r.e.Live
	if hub == nil {
		return
	}
	hub.On(lordlive.KindLive, r.tag, func(s *lordlive.Stream) {
		r.mu.Lock()
		old := r.live
		r.live = s
		r.mu.Unlock()
		if old != nil {
			old.Close()
		}
		r.s.RequestKeyframe() // what's on its way is no use to a new decoder
		for {
			m, err := s.Read()
			if err != nil {
				break
			}
			if m.Type == lordlive.TypeWantKey {
				r.s.RequestKeyframe()
			}
		}
		r.mu.Lock()
		if r.live == s {
			r.live = nil
		}
		r.mu.Unlock()
		s.Close()
	})
}

// unwatchLive ends r's stream.
func (r *remote) unwatchLive() {
	if r.e.Live != nil {
		r.e.Live.On(lordlive.KindLive, r.tag, nil)
	}
	r.mu.Lock()
	s := r.live
	r.live = nil
	r.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// LiveCallMedia is a call's camera and sound on LordOS: the system captures,
// encodes, decodes and plays, each behind its own permission, and the engine
// only passes what travels. The app opens the links when the call's state
// says so, because only it can ask the person.
type LiveCallMedia struct {
	Hub *lordlive.Hub
	mu  sync.Mutex
	s   *lordlive.Stream // the call's stream: the viewer's camera and voice
	ask func()           // asks the viewer's camera for a keyframe
}

var errNoLive = errors.New("appsock: no live links")

// StartCamera isn't used: frames never reach the engine raw.
func (l *LiveCallMedia) StartCamera(func(services.Frame)) (func(), error) { return nil, errNoLive }

// StartVoice isn't used: sound never reaches the engine raw.
func (l *LiveCallMedia) StartVoice(bool, func([]int16)) (services.CallVoice, error) {
	return nil, errNoLive
}

// StartEncodedCamera waits for the app's camera on the call's tag.
func (l *LiveCallMedia) StartEncodedCamera(_ string, wantKey func() bool, onPicture func([]byte, time.Duration)) (func(), error) {
	if l.Hub == nil {
		return nil, errNoLive
	}
	var mu sync.Mutex
	var cur *lordlive.Stream
	l.Hub.On(lordlive.KindCamera, CallTag, func(s *lordlive.Stream) {
		mu.Lock()
		old := cur
		cur = s
		mu.Unlock()
		if old != nil {
			old.Close()
		}
		defer s.Close()
		for {
			m, err := s.Read()
			if err != nil {
				return
			}
			if m.Type == lordlive.TypeCamera {
				onPicture(m.Data, m.PTS)
				if wantKey() {
					s.Keyframe()
				}
			}
		}
	})
	return func() {
		l.Hub.On(lordlive.KindCamera, CallTag, nil)
		mu.Lock()
		s := cur
		cur = nil
		mu.Unlock()
		if s != nil {
			s.Close()
		}
	}, nil
}

// StartEncodedVoice waits for the app's stream on the call's tag.
func (l *LiveCallMedia) StartEncodedVoice(onMic func([]byte)) (services.EncodedVoice, error) {
	if l.Hub == nil {
		return nil, errNoLive
	}
	v := &liveVoice{l: l}
	l.Hub.On(lordlive.KindLive, CallTag, func(s *lordlive.Stream) {
		l.mu.Lock()
		old := l.s
		l.s = s
		l.mu.Unlock()
		if old != nil {
			old.Close()
		}
		defer s.Close()
		for {
			m, err := s.Read()
			if err != nil {
				return
			}
			if m.Type == lordlive.TypeMic && !v.isMuted() {
				onMic(m.Data)
			}
			if m.Type == lordlive.TypeWantKey {
				l.mu.Lock()
				ask := l.ask
				l.mu.Unlock()
				if ask != nil {
					ask()
				}
			}
		}
	})
	return v, nil
}

// ShowCamera passes one picture of the viewer's camera to the app's stream.
func (l *LiveCallMedia) ShowCamera(codec string, pts time.Duration, data []byte, askKey func()) {
	l.mu.Lock()
	s := l.s
	l.ask = askKey
	l.mu.Unlock()
	if s == nil {
		askKey() // the stream isn't open yet: the first picture it sees must be whole
		return
	}
	if !s.Video(codec, lordlive.IsKeyframe(codec, data), pts, data) {
		askKey()
	}
}

type liveVoice struct {
	l     *LiveCallMedia
	mu    sync.Mutex
	muted bool
}

func (v *liveVoice) isMuted() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.muted
}

// Play passes one packet of the viewer's voice.
func (v *liveVoice) Play(pkt []byte) {
	v.l.mu.Lock()
	s := v.l.s
	v.l.mu.Unlock()
	if s != nil {
		s.Opus(pkt)
	}
}

// Mute drops the microphone here too: the app closes it as soon as the
// call's state says so, and nothing in between may leave.
func (v *liveVoice) Mute(m bool) {
	v.mu.Lock()
	v.muted = m
	v.mu.Unlock()
}

// Stop ends the stream.
func (v *liveVoice) Stop() {
	v.l.Hub.On(lordlive.KindLive, CallTag, nil)
	v.l.mu.Lock()
	s := v.l.s
	v.l.s = nil
	v.l.mu.Unlock()
	if s != nil {
		s.Close()
	}
}
