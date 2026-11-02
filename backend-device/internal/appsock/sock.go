// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package appsock is the local socket between the headless engine and
// the React app of LordOS (lordos/app/ENGINE.md): u32 length (little
// endian), then that many bytes of UTF-8 JSON, 64 KB at most. The engine
// sends one kind of message, state, whose fields replace what the app holds;
// the app sends requests. No text for people ever crosses it.
package appsock

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"sync"
)

// MaxFrame is the longest frame. Anything longer closes the socket.
const MaxFrame = 64 << 10

// ErrFrame: a frame longer than MaxFrame arrived, or not JSON.
var ErrFrame = errors.New("appsock: bad frame")

// Msg is a message from the app: every field of every type, those that
// don't apply empty.
type Msg struct {
	T  string `json:"t"`
	ID *int64 `json:"id"`

	Accept      bool             `json:"accept"`
	Consent     string           `json:"consent"`
	Permissions *json.RawMessage `json:"permissions"`
	Text        string           `json:"text"`
	On          bool             `json:"on"`
	Camera      *bool            `json:"camera"`
	Mic         *bool            `json:"mic"`
	Key         string           `json:"key"`
	Value       json.RawMessage  `json:"value"`
	Link        string           `json:"link"`
	What        string           `json:"what"`
	Perm        string           `json:"perm"`
	Kind        string           `json:"kind"`
	Code        string           `json:"code"`
	Device      string           `json:"device"`
	Label       string           `json:"label"`
	Remote      string           `json:"remote"`
	Password    string           `json:"password"`
	Tool        string           `json:"tool"`
	Keys        string           `json:"keys"`
	Path        string           `json:"path"`
	Name        string           `json:"name"`

	// A pointer or key of a session to another computer.
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	DX      float64 `json:"dx"`
	DY      float64 `json:"dy"`
	Button  int     `json:"button"`
	Down    bool    `json:"down"`
	Repeat  bool    `json:"repeat"`
	Lines   bool    `json:"lines"`
	Inside  bool    `json:"inside"`
	Mods    int     `json:"mods"`
	Keycode int     `json:"keycode"`
}

// Handler answers one message: data for the answer, or an error whose
// text is the reason the app words.
type Handler func(m Msg) (any, error)

// Reason is a refusal the app words.
type Reason string

func (r Reason) Error() string { return string(r) }

// Sock is the engine's end of the socket.
type Sock struct {
	rw  io.ReadWriteCloser
	log *slog.Logger

	handlers map[string]Handler
	slow     map[string]bool

	mu      sync.Mutex
	last    map[string]string // each field as last sent, to send only changes
	dirty   map[string]json.RawMessage
	replies [][]byte
	wake    chan struct{}
	closed  chan struct{}
	once    sync.Once
}

// New builds the end of a socket; Serve runs it.
func New(rw io.ReadWriteCloser, log *slog.Logger) *Sock {
	if log == nil {
		log = slog.Default()
	}
	return &Sock{
		rw: rw, log: log, handlers: map[string]Handler{}, slow: map[string]bool{},
		last: map[string]string{}, dirty: map[string]json.RawMessage{}, wake: make(chan struct{}, 1), closed: make(chan struct{}),
	}
}

// Handle answers messages of type t, on the reader: in order, and quickly.
func (s *Sock) Handle(t string, h Handler) { s.handlers[t] = h }

// HandleSlow answers messages of type t on a goroutine of their own.
func (s *Sock) HandleSlow(t string, h Handler) {
	s.handlers[t] = h
	s.slow[t] = true
}

// Closed closes when the socket is gone.
func (s *Sock) Closed() <-chan struct{} { return s.closed }

// Close ends the socket.
func (s *Sock) Close() {
	s.once.Do(func() {
		close(s.closed)
		_ = s.rw.Close()
	})
}

// Set replaces one state field. It goes out soon, together with what else
// changed, and not at all if it is what the app holds already. A list too
// long for a frame loses its oldest elements, never the frame.
func (s *Sock) Set(field string, v any) {
	b, err := fit(field, v)
	if err != nil {
		s.log.Warn("appsock: a field that can't be sent", "field", field, "err", err)
		return
	}
	s.mu.Lock()
	if s.last[field] == string(b) {
		s.mu.Unlock()
		return
	}
	s.last[field] = string(b)
	s.dirty[field] = b
	s.mu.Unlock()
	s.poke()
}

// room is a frame less the envelope of a state message.
const room = MaxFrame - 64

func fit(field string, v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b)+len(field)+8 <= room {
		return b, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%d bytes", len(b))
	}
	for n := 1; n < rv.Len(); n += max(rv.Len()/16, 1) {
		if b, err = json.Marshal(rv.Slice(n, rv.Len()).Interface()); err == nil && len(b)+len(field)+8 <= room {
			return b, nil
		}
	}
	return []byte("[]"), nil
}

func (s *Sock) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Sock) reply(id int64, data any, err error) {
	m := map[string]any{"t": "res", "id": id}
	if err != nil {
		m["err"] = err.Error()
	} else {
		m["data"] = data
	}
	b, merr := json.Marshal(m)
	if merr != nil || len(b) > MaxFrame {
		b, _ = json.Marshal(map[string]any{"t": "res", "id": id, "err": "too_long"})
	}
	s.mu.Lock()
	s.replies = append(s.replies, b)
	s.mu.Unlock()
	s.poke()
}

// frames takes what is waiting and packs the state into frames: all of it
// in one when it fits, which the first one does.
func (s *Sock) frames() [][]byte {
	s.mu.Lock()
	dirty := s.dirty
	s.dirty = map[string]json.RawMessage{}
	out := s.replies
	s.replies = nil
	s.mu.Unlock()
	if len(dirty) == 0 {
		return out
	}
	head := []byte(`{"t":"state","set":{`)
	cur := slices.Clone(head)
	n := 0
	flush := func() {
		if n > 0 {
			out = append(out, append(cur, "}}"...))
		}
		cur, n = slices.Clone(head), 0
	}
	for _, k := range slices.Sorted(maps.Keys(dirty)) {
		q, _ := json.Marshal(k)
		item := append(append(q, ':'), dirty[k]...)
		if n > 0 && len(cur)+len(item)+3 > MaxFrame {
			flush()
		}
		if n > 0 {
			cur = append(cur, ',')
		}
		cur = append(cur, item...)
		n++
	}
	flush()
	return out
}

func (s *Sock) write() {
	for {
		select {
		case <-s.closed:
			return
		case <-s.wake:
		}
		for _, f := range s.frames() {
			if err := WriteFrame(s.rw, f); err != nil {
				s.Close()
				return
			}
		}
	}
}

// WriteFrame writes one frame.
func WriteFrame(w io.Writer, b []byte) error {
	if len(b) > MaxFrame {
		return ErrFrame
	}
	_, err := w.Write(append(binary.LittleEndian.AppendUint32(make([]byte, 0, len(b)+4), uint32(len(b))), b...))
	return err
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hd [4]byte
	if _, err := io.ReadFull(r, hd[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hd[:])
	if n > MaxFrame {
		return nil, ErrFrame
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

// Serve runs the socket until it ends: end of file, a bad frame, or Close.
func (s *Sock) Serve() error {
	defer s.Close()
	go s.write()
	s.poke()
	for {
		b, err := ReadFrame(s.rw)
		if err != nil {
			return err
		}
		var m Msg
		d := json.NewDecoder(bytes.NewReader(b))
		if d.Decode(&m) != nil || m.T == "" {
			return ErrFrame
		}
		h := s.handlers[m.T]
		switch {
		case h == nil:
			s.log.Debug("appsock: unknown message", "t", m.T)
			if m.ID != nil {
				s.reply(*m.ID, nil, Reason("unknown"))
			}
		case s.slow[m.T]:
			go s.run(h, m)
		default:
			s.run(h, m)
		}
	}
}

func (s *Sock) run(h Handler, m Msg) {
	data, err := h(m)
	if err != nil {
		s.log.Debug("appsock: refused", "t", m.T, "err", err)
	}
	if m.ID != nil {
		s.reply(*m.ID, data, err)
	}
}
