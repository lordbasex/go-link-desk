// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"log/slog"
	"sync"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inputproto"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// Notice for viewers that may control but the host cannot inject.
const NoticeNoAccessibility = "no_accessibility"

// noPosition is a button packet's display when it carries no position
// (pointer lock): it acts where the pointer is.
const noPosition = 0xFF

// InputService turns a viewer's input packets into the host's mouse and
// keyboard, only for a viewer allowed to control. Every key and button it
// presses is let go when the viewer loses the permission, leaves, hangs up,
// or its connection stops answering.
type InputService struct {
	inj      inject.Injector
	displays func() []screencap.Display // the host's, in the order the viewer knows them
	log      *slog.Logger

	mu      sync.Mutex
	allowed map[string]*inputState
}

type inputState struct {
	seq     uint16 // the last pointer move applied
	haveSeq bool
}

// NewInputService builds the service. displays lists the host's displays
// in the order the "displays" message gave them.
func NewInputService(inj inject.Injector, displays func() []screencap.Display, log *slog.Logger) *InputService {
	if log == nil {
		log = slog.Default()
	}
	return &InputService{inj: inj, displays: displays, log: log, allowed: make(map[string]*inputState)}
}

// Allow gives or takes a viewer's control. Taking it lets go of
// everything the viewer held.
func (s *InputService) Allow(peer string, on bool) {
	s.mu.Lock()
	_, had := s.allowed[peer]
	if on && !had {
		s.allowed[peer] = &inputState{}
	}
	if !on {
		delete(s.allowed, peer)
	}
	s.mu.Unlock()
	if had && !on {
		s.inj.ReleaseAll()
	}
	if on && !had {
		s.log.Info("viewer may control", "peer_id", peer)
	}
}

// Trusted reports whether the system lets the host inject input.
func (s *InputService) Trusted() bool { return s.inj.Trusted() }

// Release lets go of everything held (the viewer's connection stalled,
// or it asked: release_all).
func (s *InputService) Release(peer string) {
	s.mu.Lock()
	_, ok := s.allowed[peer]
	s.mu.Unlock()
	if ok {
		s.inj.ReleaseAll()
	}
}

// Forget ends a viewer's control for good (its session ended).
func (s *InputService) Forget(peer string) { s.Allow(peer, false) }

// HandlePointer applies a pointer move from the unreliable channel; a
// move older than the last one applied came late and is dropped.
func (s *InputService) HandlePointer(peer string, data []byte) {
	p, err := inputproto.Decode(data)
	if err != nil || (p.Type != inputproto.TypeAbs && p.Type != inputproto.TypeRel) {
		return
	}
	s.mu.Lock()
	st := s.allowed[peer]
	if st == nil || (st.haveSeq && !inputproto.SeqNewer(p.Seq, st.seq)) {
		s.mu.Unlock()
		return
	}
	st.seq, st.haveSeq = p.Seq, true
	s.mu.Unlock()
	if p.Type == inputproto.TypeRel {
		s.inj.MoveBy(int(p.DX), int(p.DY))
		return
	}
	x, y := s.point(p.Display, p.X, p.Y)
	s.inj.Move(x, y)
}

// HandleInput applies a packet from the reliable channel.
func (s *InputService) HandleInput(peer string, data []byte) {
	p, err := inputproto.Decode(data)
	if err != nil {
		return
	}
	s.mu.Lock()
	ok := s.allowed[peer] != nil
	s.mu.Unlock()
	if !ok {
		return
	}
	switch p.Type {
	case inputproto.TypeButton:
		if p.Display == noPosition {
			s.inj.Button(int(p.Button), p.Down, 0, 0, false)
			return
		}
		x, y := s.point(p.Display, p.X, p.Y)
		s.inj.Button(int(p.Button), p.Down, x, y, true)
	case inputproto.TypeWheel:
		s.inj.Scroll(int(p.DX), int(p.DY), p.Mode == inputproto.WheelLines)
	case inputproto.TypeKey:
		if k, ok := keymap.Lookup(p.Keycode); ok && k.Mac >= 0 {
			s.inj.Key(k.Mac, p.Down, p.Repeat, p.Mods&(inputproto.ModShift|inputproto.ModCtrl|inputproto.ModAlt|inputproto.ModMeta))
		}
	case inputproto.TypeText:
		s.inj.Text(p.Text)
	case inputproto.TypeReleaseAll:
		s.inj.ReleaseAll()
	}
}

// point maps a position over a display (0..65535) to the desktop's
// points.
func (s *InputService) point(display uint8, x, y uint16) (float64, float64) {
	ds := s.displays()
	d := pickDisplay(ds, 0)
	if int(display) < len(ds) {
		d = ds[display]
	}
	fx := float64(x) / 65535 * float64(d.PointsWidth)
	fy := float64(y) / 65535 * float64(d.PointsHeight)
	// The last point is the display's edge, not past it.
	fx = min(fx, float64(d.PointsWidth)-0.5)
	fy = min(fy, float64(d.PointsHeight)-0.5)
	return float64(d.X) + fx, float64(d.Y) + fy
}
