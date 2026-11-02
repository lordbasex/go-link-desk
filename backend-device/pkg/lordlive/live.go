// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build unix

// Package lordlive is a companion's side of LordOS's live media links
// (docs/PACKAGED-APPS.md there, "Live media"). The engine is pure Go and has
// no decoder or encoder: pictures and Opus packets go over a socket to a
// sandboxed helper of the system, which shows and plays them in the app's
// window, and the camera and microphone come back the same way, encoded.
package lordlive

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// InheritedFD is where LordOS hands a companion its links.
const InheritedFD = 6

// The messages (media/live.h in LordOS).
const (
	TypeCodec    = 1
	TypeVideo    = 2
	TypeOpus     = 3
	TypeKeyframe = 4
	TypeWantKey  = 64
	TypeMic      = 65
	TypeCamera   = 66
	FlagKey      = 1

	headerSize = 16
	maxMessage = 8 << 20
	// What may wait for the helper. Past it pictures are dropped until the
	// next keyframe, since a picture without those before it decodes wrong.
	queueLen = 16
)

// Kinds of link.
const (
	KindLive   = "live"   // pictures and sound to show, the microphone back
	KindCamera = "camera" // the camera back
)

// Message is one message from the helper.
type Message struct {
	Type  byte
	Flags byte
	PTS   time.Duration
	Data  []byte
}

// Stream is one link. Its methods never wait for the helper.
type Stream struct {
	conn    net.Conn
	out     chan []byte
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	skip    bool
	codec   string
	dropped int
}

// NewStream takes a link's connection.
func NewStream(conn net.Conn) *Stream {
	s := &Stream{conn: conn, out: make(chan []byte, queueLen), done: make(chan struct{})}
	go s.write()
	return s
}

func (s *Stream) write() {
	for {
		select {
		case <-s.done:
			return
		case b := <-s.out:
			// A helper that doesn't read for this long is gone.
			_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			if _, err := s.conn.Write(b); err != nil {
				s.Close()
				return
			}
		}
	}
}

func message(typ, flags byte, pts time.Duration, data []byte) []byte {
	b := make([]byte, headerSize+len(data))
	binary.LittleEndian.PutUint32(b, uint32(len(data)))
	b[4], b[5] = typ, flags
	binary.LittleEndian.PutUint64(b[8:], uint64(pts.Milliseconds()))
	copy(b[headerSize:], data)
	return b
}

func (s *Stream) push(b []byte) bool {
	select {
	case <-s.done:
		return false
	case s.out <- b:
		return true
	default:
		return false
	}
}

// Video sends one picture. It reports false if it was dropped; asking for a
// keyframe is then the caller's to do.
func (s *Stream) Video(codec string, key bool, pts time.Duration, data []byte) bool {
	if len(data) == 0 || len(data) > maxMessage {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if codec != s.codec {
		// A new codec starts with a keyframe or not at all.
		if !s.push(message(TypeCodec, 0, 0, []byte(codec))) {
			return false
		}
		s.codec = codec
	}
	if s.skip && !key {
		s.dropped++
		return false
	}
	var flags byte
	if key {
		flags = FlagKey
	}
	if !s.push(message(TypeVideo, flags, pts, data)) {
		s.skip = true
		s.dropped++
		return false
	}
	s.skip = false
	return true
}

// Opus sends one packet to be played. Sound isn't kept for later: dropped
// is better than late.
func (s *Stream) Opus(data []byte) {
	if len(data) > 0 && len(data) <= 4000 {
		s.push(message(TypeOpus, 0, 0, data))
	}
}

// Keyframe asks a camera for one.
func (s *Stream) Keyframe() { s.push(message(TypeKeyframe, 0, 0, nil)) }

// Read gives the helper's next message.
func (s *Stream) Read() (Message, error) {
	var h [headerSize]byte
	if _, err := io.ReadFull(s.conn, h[:]); err != nil {
		return Message{}, err
	}
	n := binary.LittleEndian.Uint32(h[:])
	if n > maxMessage {
		return Message{}, errors.New("lordlive: message too long")
	}
	m := Message{Type: h[4], Flags: h[5], PTS: time.Duration(int64(binary.LittleEndian.Uint64(h[8:]))) * time.Millisecond, Data: make([]byte, n)}
	if _, err := io.ReadFull(s.conn, m.Data); err != nil {
		return Message{}, err
	}
	return m, nil
}

// Done is closed when the link has ended.
func (s *Stream) Done() <-chan struct{} { return s.done }

// Close ends the link.
func (s *Stream) Close() {
	s.once.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

// Hub takes the links LordOS hands over and gives each to whoever waits for
// its kind and tag.
type Hub struct {
	mu   sync.Mutex
	want map[string]func(*Stream)
	conn *net.UnixConn
}

// NewHub makes a hub.
func NewHub() *Hub { return &Hub{want: map[string]func(*Stream){}} }

func key(kind string, tag int) string { return fmt.Sprintf("%s %d", kind, tag) }

// On says who gets a link of that kind and tag, every time one comes. A nil
// fn takes it back. A link nobody waits for is closed.
func (h *Hub) On(kind string, tag int, fn func(*Stream)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if fn == nil {
		delete(h.want, key(kind, tag))
		return
	}
	h.want[key(kind, tag)] = fn
}

// Deliver gives a link to who waits for it.
func (h *Hub) Deliver(line string, conn net.Conn) error {
	kind, tagText, ok := strings.Cut(line, " ")
	tag, err := strconv.Atoi(tagText)
	if !ok || err != nil || (kind != KindLive && kind != KindCamera) {
		_ = conn.Close()
		return fmt.Errorf("lordlive: bad line %q", line)
	}
	h.mu.Lock()
	fn := h.want[key(kind, tag)]
	h.mu.Unlock()
	if fn == nil {
		_ = conn.Close()
		return nil
	}
	go fn(NewStream(conn))
	return nil
}

// Take claims fd 6. It runs first thing in the program: later on the number
// may be one of the program's own, when LordOS left it free.
func (h *Hub) Take() error {
	var st syscall.Stat_t
	if err := syscall.Fstat(InheritedFD, &st); err != nil {
		return err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return errors.New("lordlive: fd 6 is not a socket")
	}
	f := os.NewFile(InheritedFD, "live")
	defer f.Close()
	c, err := net.FileConn(f)
	if err != nil {
		return err
	}
	u, ok := c.(*net.UnixConn)
	if !ok {
		_ = c.Close()
		return errors.New("lordlive: fd 6 is not a local socket")
	}
	h.conn = u
	return nil
}

// Serve hands out links until LordOS closes the socket. With nothing taken
// it returns at once, and no link ever comes.
func (h *Hub) Serve() error {
	if h.conn == nil {
		return errors.New("lordlive: no fd 6")
	}
	defer h.conn.Close()
	return h.serve(h.conn)
}

func (h *Hub) serve(u *net.UnixConn) error {
	buf := make([]byte, 128)
	oob := make([]byte, syscall.CmsgSpace(4*4))
	for {
		n, oobn, _, _, err := u.ReadMsgUnix(buf, oob)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.EOF
		}
		fd := takeFD(oob[:oobn])
		if fd < 0 {
			continue
		}
		f := os.NewFile(uintptr(fd), "live-link")
		conn, err := net.FileConn(f)
		_ = f.Close()
		if err != nil {
			continue
		}
		_ = h.Deliver(string(buf[:n]), conn)
	}
}

// takeFD gives the one descriptor of a message; any others are closed.
func takeFD(oob []byte) int {
	msgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return -1
	}
	fd := -1
	for i := range msgs {
		fds, err := syscall.ParseUnixRights(&msgs[i])
		if err != nil {
			continue
		}
		for _, x := range fds {
			if fd < 0 {
				fd = x
			} else {
				_ = syscall.Close(x)
			}
		}
	}
	return fd
}

// IsKeyframe tells a whole picture from its first bytes. H.264 is Annex B:
// an IDR slice, or a parameter set before one.
func IsKeyframe(codec string, d []byte) bool {
	if codec == "vp8" {
		return len(d) > 0 && d[0]&1 == 0
	}
	for i := 0; i+3 < len(d); i++ {
		if d[i] == 0 && d[i+1] == 0 && d[i+2] == 1 {
			switch d[i+3] & 0x1f {
			case 5, 7:
				return true
			case 1:
				return false
			}
			i += 3
		}
	}
	return false
}
