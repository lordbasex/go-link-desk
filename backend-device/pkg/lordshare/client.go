// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package lordshare

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// State is the system's answer to a query, and what it sends when a share
// starts or its grants change.
type State struct {
	Blocked Reason // what a start would be refused with now (OK: nothing)
	Granted int    // Flag*
	Codecs  int    // bit 0 H.264, bit 1 VP8
	W, H    int
	Hz      int
}

// Video is one encoded frame. Data is only valid during the call.
type Video struct {
	Codec int
	Key   bool
	Ms    uint32
	Data  []byte
}

// Shape is the pointer's image: 0xAARRGGBB pixels, little endian in memory.
type Shape struct {
	W, H, HotX, HotY int
	ARGB             []byte
	Gen              int // moves on with every new image
}

// Pointer is where the session's pointer is, in session pixels.
type Pointer struct {
	X, Y int
	Mode int // 0 arrow, 1 image, 2 hidden
	Seen bool
}

// Media takes what a running share sends. Each call comes from the
// connection's one reader, so a slow one holds the next message back.
type Media struct {
	Video func(Video)
	Audio func(first uint32, pkt []byte)
	// Size is the session's picture size.
	Size func(w, h int)
	// Started: the share runs (the indicator is on screen).
	Started func(State)
	// Refused: a start was turned down.
	Refused func(Reason)
	// Ended: a running share is over.
	Ended func(Reason)
	// State: a query's answer, or grants that changed.
	State func(State)
}

// Client is one connection to the API. Every method is safe from any
// goroutine; on a connection that is gone they do nothing.
type Client struct {
	rw   io.ReadWriteCloser
	out  chan []byte // whole messages, in order
	done chan struct{}
	once sync.Once

	mu       sync.Mutex
	media    Media
	state    State
	haveSt   bool
	starting bool // SHARE_START sent, no answer yet
	running  bool
	hear     bool
	awake    bool
	pointer  Pointer
	shape    Shape
	clip     string
	clipGen  int
	err      error
	dropped  atomic.Int64
}

// queue is how many messages wait for the socket. Pointer moves are dropped
// beyond it; anything else waits, because a lost release sticks a key.
const queue = 256

// New starts a client on a connected stream: its reader and writer run
// until the connection ends.
func New(rw io.ReadWriteCloser) *Client {
	c := &Client{rw: rw, out: make(chan []byte, queue), done: make(chan struct{})}
	go c.read()
	go c.write()
	return c
}

// Close ends the connection, and the share with it.
func (c *Client) Close() {
	c.stop(errClosed)
}

func (c *Client) stop(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		ended, ran := c.media.Ended, c.running
		c.running, c.starting = false, false
		c.mu.Unlock()
		close(c.done)
		_ = c.rw.Close()
		if ran && ended != nil {
			ended(EndedByApp)
		}
	})
}

// Done closes when the connection is gone.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err says why the connection ended.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// SetMedia sets who takes what a share sends.
func (c *Client) SetMedia(m Media) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.media = m
}

func (c *Client) write() {
	for {
		select {
		case <-c.done:
			return
		case b := <-c.out:
			if _, err := c.rw.Write(b); err != nil {
				c.stop(err)
				return
			}
		}
	}
}

// send queues one message and waits for room.
func (c *Client) send(t byte, parts ...[]byte) {
	b := frame(nil, t, parts...)
	select {
	case c.out <- b:
	case <-c.done:
	}
}

// trySend queues one message, or drops it.
func (c *Client) trySend(t byte, parts ...[]byte) {
	select {
	case c.out <- frame(nil, t, parts...):
	default:
		c.dropped.Add(1)
	}
}

func onOff(on bool) []byte {
	if on {
		return []byte{1}
	}
	return []byte{0}
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// Query asks for a State, starts nothing and prompts nobody.
func (c *Client) Query() { c.send(msgQuery) }

// Start asks for a share. The answer is Media.Started or Media.Refused. A
// flag that isn't granted is dropped by the system; watch State.Granted.
func (c *Client) Start(flags, codec, hz, kbps int) {
	d := make([]byte, 8)
	d[0] = byte(flags&FlagAll | FlagVideo)
	d[1] = byte(clamp(codec, CodecH264, CodecVP8))
	binary.LittleEndian.PutUint16(d[2:], uint16(clamp(hz, MinHz, MaxHz)))
	binary.LittleEndian.PutUint32(d[4:], uint32(clamp(kbps, MinKbps, MaxKbps)))
	c.mu.Lock()
	c.starting = true
	c.mu.Unlock()
	c.send(msgStart, d)
}

// Stop ends the share.
func (c *Client) Stop() { c.send(msgStop) }

// Lock locks the session, which ends the share.
func (c *Client) Lock() { c.send(msgLock) }

// Keyframe asks for one; the system rations them.
func (c *Client) Keyframe() { c.trySend(msgKeyframe) }

// Rate tells the encoder what the network takes.
func (c *Client) Rate(kbps int) {
	d := binary.LittleEndian.AppendUint32(nil, uint32(clamp(kbps, MinKbps, MaxKbps)))
	c.trySend(msgRate, d)
}

// Resize asks for a session size; a session on a monitor keeps its own.
func (c *Client) Resize(w, h int) {
	if w < 640 || h < 480 || w > 3840 || h > 2400 {
		return
	}
	d := binary.LittleEndian.AppendUint16(nil, uint16(w))
	c.send(msgResize, binary.LittleEndian.AppendUint16(d, uint16(h)))
}

// Hear turns the sound on or off for this share. It is kept: a share that
// isn't running yet gets it when it does.
func (c *Client) Hear(on bool) {
	c.mu.Lock()
	c.hear = on
	running := c.running
	c.mu.Unlock()
	if running {
		c.send(msgHear, onOff(on))
	}
}

// Awake keeps the display from going idle. Kept as Hear is.
func (c *Client) Awake(on bool) {
	c.mu.Lock()
	c.awake = on
	running := c.running
	c.mu.Unlock()
	if running {
		c.send(msgAwake, onOff(on))
	}
}

// Move puts the pointer at x, y, in session pixels. Moves are the one
// message that may be dropped.
func (c *Client) Move(x, y int) {
	d := binary.LittleEndian.AppendUint16(nil, uint16(clamp(x, 0, 32767)))
	c.trySend(msgPtr, binary.LittleEndian.AppendUint16(d, uint16(clamp(y, 0, 32767))))
}

// Button presses or releases 0 left, 1 middle or 2 right.
func (c *Client) Button(b int, down bool) {
	if b < 0 || b > 2 {
		return
	}
	c.send(msgBtn, []byte{byte(b)}, onOff(down))
}

// Wheel scrolls by steps; positive is up.
func (c *Client) Wheel(steps int) {
	if steps != 0 {
		c.send(msgWheel, []byte{byte(int8(clamp(steps, -127, 127)))})
	}
}

// Scroll is fine scrolling: phase 1 begins, 2 continues, 3 ends; deltas in
// pixels.
func (c *Client) Scroll(phase int, dx, dy float64) {
	if phase < 1 || phase > 3 {
		return
	}
	const lim = 1 << 24
	d := []byte{byte(phase)}
	d = binary.LittleEndian.AppendUint32(d, uint32(int32(clamp(int(dx*256), -lim, lim))))
	d = binary.LittleEndian.AppendUint32(d, uint32(int32(clamp(int(dy*256), -lim, lim))))
	c.send(msgScroll, d)
}

// Key sends an evdev code: value 1 pressed, 0 released, 2 repeat.
func (c *Client) Key(code, value int) {
	if code <= 0 || code >= maxKey || value < 0 || value > 2 {
		return
	}
	d := binary.LittleEndian.AppendUint16(nil, uint16(code))
	c.send(msgKey, append(d, byte(value)))
}

// Text types s through the active keyboard layout.
func (c *Client) Text(s string) {
	for _, p := range splitText(cleanText(s), MaxText) {
		c.send(msgText, []byte(p))
	}
}

// SetClipboard replaces the session's clipboard with text.
func (c *Client) SetClipboard(s string) {
	s = cleanText(s)
	if len(s) > MaxClip {
		s = splitText(s, MaxClip)[0]
	}
	c.send(msgSetClip, []byte(s))
}

// State is the last one the system sent.
func (c *Client) State() (State, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state, c.haveSt
}

// Running reports whether a share runs now.
func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Pointer is the last pointer the system sent.
func (c *Client) Pointer() Pointer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pointer
}

// Shape is the last pointer image the system sent.
func (c *Client) Shape() Shape {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shape
}

// Clipboard is what the session last copied and how many copies arrived.
func (c *Client) Clipboard() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clip, c.clipGen
}

func (c *Client) read() {
	var buf []byte
	for {
		t, d, err := readFrame(c.rw, buf)
		if err != nil {
			c.stop(err)
			return
		}
		buf = d
		c.handle(t, d)
	}
}

func (c *Client) handle(t byte, d []byte) {
	c.mu.Lock()
	m := c.media
	c.mu.Unlock()
	switch t {
	case msgVFrame:
		if len(d) >= 8 && m.Video != nil {
			m.Video(Video{Codec: int(d[0]), Key: d[1] != 0, Ms: binary.LittleEndian.Uint32(d[4:]), Data: d[8:]})
		}
	case msgAFrame:
		if len(d) >= 4 && m.Audio != nil {
			m.Audio(binary.LittleEndian.Uint32(d), d[4:])
		}
	case msgSize:
		if len(d) == 4 {
			c.onSize(int(binary.LittleEndian.Uint16(d)), int(binary.LittleEndian.Uint16(d[2:])), m)
		}
	case msgState:
		if len(d) == 10 {
			c.onState(State{
				Blocked: Reason(d[0]), Granted: int(d[1]), Codecs: int(d[2]),
				W: int(binary.LittleEndian.Uint16(d[4:])), H: int(binary.LittleEndian.Uint16(d[6:])), Hz: int(binary.LittleEndian.Uint16(d[8:])),
			}, m)
		}
	case msgRefused:
		if len(d) == 1 {
			c.mu.Lock()
			c.starting = false
			c.mu.Unlock()
			if m.Refused != nil {
				m.Refused(Reason(d[0]))
			}
		}
	case msgEnd:
		if len(d) == 1 {
			c.mu.Lock()
			c.running, c.starting = false, false
			c.mu.Unlock()
			if m.Ended != nil {
				m.Ended(Reason(d[0]))
			}
		}
	case msgCursor:
		if len(d) == 5 {
			c.mu.Lock()
			c.pointer = Pointer{X: int(int16(binary.LittleEndian.Uint16(d))), Y: int(int16(binary.LittleEndian.Uint16(d[2:]))), Mode: int(d[4]), Seen: true}
			c.mu.Unlock()
		}
	case msgShape:
		if len(d) >= 8 {
			s := Shape{W: int(binary.LittleEndian.Uint16(d)), H: int(binary.LittleEndian.Uint16(d[2:])), HotX: int(binary.LittleEndian.Uint16(d[4:])), HotY: int(binary.LittleEndian.Uint16(d[6:]))}
			if len(d)-8 == s.W*s.H*4 {
				s.ARGB = append([]byte(nil), d[8:]...)
				c.mu.Lock()
				s.Gen = c.shape.Gen + 1
				c.shape = s
				c.mu.Unlock()
			}
		}
	case msgClip:
		c.mu.Lock()
		c.clip = string(d)
		c.clipGen++
		c.mu.Unlock()
	}
}

// onState keeps the state; a query's answer may arrive while a start
// waits, so a state says nothing about the share running.
func (c *Client) onState(st State, m Media) {
	c.mu.Lock()
	if st.W == 0 {
		st.W, st.H = c.state.W, c.state.H
	}
	c.state, c.haveSt = st, true
	c.mu.Unlock()
	if m.State != nil {
		m.State(st)
	}
}

// onSize keeps the size. The first size after a start is the sign the share
// runs (the system sends it only then): what was asked before is sent now,
// because the system ignores it until this moment.
func (c *Client) onSize(w, h int, m Media) {
	c.mu.Lock()
	c.state.W, c.state.H = w, h
	st := c.state
	started := c.starting
	if started {
		c.starting, c.running = false, true
	}
	hear, awake := c.hear, c.awake
	c.mu.Unlock()
	if started {
		c.send(msgHear, onOff(hear))
		c.send(msgAwake, onOff(awake))
		if m.Started != nil {
			m.Started(st)
		}
	}
	if m.Size != nil {
		m.Size(w, h)
	}
}

// String is for the logs.
func (s State) String() string {
	return fmt.Sprintf("blocked=%s granted=%d codecs=%d %dx%d@%d", s.Blocked, s.Granted, s.Codecs, s.W, s.H, s.Hz)
}
