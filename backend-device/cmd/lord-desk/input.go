package main

import (
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

// injector sends a viewer's input as remote.sock messages. Text and
// relative moves are out of scope for stage 0.
type injector struct {
	s    *session
	mu   sync.Mutex
	keys map[uint16]bool
	btns map[byte]bool
	acc  int
}

func (i *injector) Move(x, y float64) {
	d := make([]byte, 4)
	binary.LittleEndian.PutUint16(d, uint16(int16(x)))
	binary.LittleEndian.PutUint16(d[2:], uint16(int16(y)))
	i.s.send(rpPtr, d)
}

func (i *injector) MoveBy(int, int) {}

func (i *injector) Button(b int, down bool, x, y float64, at bool) {
	if b < 0 || b > 2 {
		return
	}
	if at {
		i.Move(x, y)
	}
	i.mu.Lock()
	if down {
		i.btns[byte(b)] = true
	} else {
		delete(i.btns, byte(b))
	}
	i.mu.Unlock()
	i.btn(byte(b), down)
}

func (i *injector) btn(b byte, down bool) {
	v := byte(0)
	if down {
		v = 1
	}
	i.s.send(rpBtn, []byte{b, v})
}

func (i *injector) Scroll(_, dy int, lines bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	n := dy
	if !lines {
		i.acc += dy
		n = i.acc / 100
		i.acc -= n * 100
	}
	if n != 0 {
		i.s.send(rpWheel, []byte{byte(int8(max(min(-n, 10), -10)))}) // + is up here, down there
	}
}

func (i *injector) key(c uint16, v byte) {
	d := make([]byte, 3)
	binary.LittleEndian.PutUint16(d, c)
	d[2] = v
	i.s.send(rpKey, d)
}

func (i *injector) Key(sym int, down, repeat bool, _ uint8) {
	code := keymap.EvdevOfX11(sym)
	if code <= 0 {
		return
	}
	c := uint16(code)
	i.mu.Lock()
	if down {
		i.keys[c] = true
	} else {
		delete(i.keys, c)
	}
	i.mu.Unlock()
	switch {
	case !down:
		i.key(c, 0)
	case repeat:
		i.key(c, 2)
	default:
		i.key(c, 1)
	}
}

func (i *injector) Text(string) {}

func (i *injector) ReleaseAll() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for c := range i.keys {
		i.key(c, 0)
	}
	for b := range i.btns {
		i.btn(b, false)
	}
	i.keys, i.btns = map[uint16]bool{}, map[byte]bool{}
}

func (i *injector) Trusted() bool { return true }

var _ inject.Injector = (*injector)(nil)

func wireInput(stream *services.StreamService, session *services.SessionService, s *session, log *slog.Logger) {
	in := services.NewInputService(&injector{s: s, keys: map[uint16]bool{}, btns: map[byte]bool{}}, stream.Displays, log)
	stream.OnInput(func(peer, channel string, data []byte) {
		if channel == "pointer" {
			in.HandlePointer(peer, data)
		} else {
			in.HandleInput(peer, data)
		}
	})
	stream.OnStall(in.Release)
	session.OnPermissions(func(peer string, p models.Permissions) { in.Allow(peer, p.Control) })
}
