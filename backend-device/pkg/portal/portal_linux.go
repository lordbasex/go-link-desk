// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

// Package portal drives a Wayland desktop through its remote desktop
// portal (xdg-desktop-portal, on the session's D-Bus): one session shares
// the screen as PipeWire streams and takes the mouse and keyboard. The
// desktop asks the person at the computer the first time; the restore
// token it hands back lets later sessions start without asking again.
// GNOME and KDE Plasma implement it; a desktop that does not leaves the
// host on the test card.
package portal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName      = "org.freedesktop.portal.Desktop"
	busPath      = "/org/freedesktop/portal/desktop"
	remoteIface  = "org.freedesktop.portal.RemoteDesktop"
	castIface    = "org.freedesktop.portal.ScreenCast"
	requestIface = "org.freedesktop.portal.Request"
	sessionIface = "org.freedesktop.portal.Session"
)

// The portal's device types and cursor modes (bit flags).
const (
	deviceKeyboard = 1
	devicePointer  = 2
	sourceMonitor  = 1
	cursorEmbedded = 2
	cursorMetadata = 4
)

var (
	// ErrUnavailable: the desktop has no remote desktop portal.
	ErrUnavailable = errors.New("portal: the desktop has no remote desktop portal")
	// ErrDenied: the person at the computer did not allow sharing.
	ErrDenied = errors.New("portal: sharing the screen was not allowed")
)

// Wayland reports whether the desktop is a Wayland session.
func Wayland() bool {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "wayland":
		return true
	case "x11":
		return false
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

var available struct {
	once sync.Once
	ok   bool
}

// Available reports whether the session bus has the remote desktop and
// screen cast portals.
func Available() bool {
	available.once.Do(func() {
		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			return
		}
		defer conn.Close()
		obj := conn.Object(busName, busPath)
		_, err1 := obj.GetProperty(remoteIface + ".AvailableDeviceTypes")
		_, err2 := obj.GetProperty(castIface + ".AvailableSourceTypes")
		available.ok = err1 == nil && err2 == nil
	})
	return available.ok
}

var inputMethod struct {
	once sync.Once
	ok   bool
}

// InputMethod reports whether the session runs an input method that
// types characters by their code point (IBus, Fcitx: Ctrl+Shift+U).
func InputMethod() bool {
	inputMethod.once.Do(func() {
		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			return
		}
		defer conn.Close()
		var names []string
		if conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names) != nil {
			return
		}
		for _, n := range names {
			if n == "org.freedesktop.IBus" || strings.HasPrefix(n, "org.fcitx.Fcitx") {
				inputMethod.ok = true
			}
		}
	})
	return inputMethod.ok
}

// Stream is one shared monitor: a PipeWire node and its place on the
// desktop, in the desktop's (logical) pixels.
type Stream struct {
	Node       uint32
	X, Y, W, H int
}

// Session is a running remote desktop session.
type Session struct {
	conn    *dbus.Conn
	obj     dbus.BusObject
	handle  dbus.ObjectPath
	streams []Stream
	cursor  bool // the streams carry the pointer as metadata
	control bool // the person allowed the mouse and keyboard
	closed  chan struct{}
	refs    int
	input   chan event
}

// event is one Notify call.
type event struct {
	method string
	args   []any
	motion bool // may be dropped when the desktop falls behind
	pause  time.Duration
}

var (
	mu       sync.Mutex
	starting sync.Mutex // one start at a time (the desktop may be asking)
	active   *Session
)

// Acquire returns the running session, or starts one, which may wait for
// the person at the computer to allow it. Every Acquire takes a Release.
func Acquire(ctx context.Context) (*Session, error) {
	starting.Lock()
	defer starting.Unlock()
	mu.Lock()
	if s := active; s != nil && !s.isClosed() {
		s.refs++
		mu.Unlock()
		return s, nil
	}
	mu.Unlock()
	if !Available() {
		return nil, ErrUnavailable
	}
	s, err := start(ctx)
	if err != nil {
		return nil, err
	}
	s.refs = 1
	mu.Lock()
	active = s
	mu.Unlock()
	return s, nil
}

// Release lets go of the session: the last one closes it, and the desktop
// stops showing that the screen is shared.
func (s *Session) Release() {
	mu.Lock()
	s.refs--
	last := s.refs <= 0
	if last && active == s {
		active = nil
	}
	mu.Unlock()
	if last {
		s.close()
	}
}

// Active is the running session (nil: none). Input goes to it.
func Active() *Session {
	mu.Lock()
	defer mu.Unlock()
	if active == nil || active.isClosed() {
		return nil
	}
	return active
}

// Streams are the shared monitors, left to right.
func (s *Session) Streams() []Stream { return s.streams }

// Control reports whether the person allowed the mouse and keyboard too
// (GNOME asks it apart: "Allow Remote Interaction").
func (s *Session) Control() bool { return s.control }

// CursorMetadata reports whether the streams carry the pointer apart from
// the picture (else it is drawn into it).
func (s *Session) CursorMetadata() bool { return s.cursor }

// Closed is closed when the session ends, also when the desktop ends it
// (the person at the computer stopped sharing).
func (s *Session) Closed() <-chan struct{} { return s.closed }

func (s *Session) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// PipeWire opens the connection to the session's streams: a file
// descriptor of the caller's.
func (s *Session) PipeWire() (int, error) {
	var fd dbus.UnixFD
	err := s.obj.Call(castIface+".OpenPipeWireRemote", 0, s.handle, map[string]dbus.Variant{}).Store(&fd)
	if err != nil {
		return -1, fmt.Errorf("portal: open the PipeWire remote: %w", err)
	}
	return int(fd), nil
}

func (s *Session) close() {
	if s.isClosed() {
		return
	}
	close(s.closed)
	s.conn.Object(busName, s.handle).Call(sessionIface+".Close", 0)
	s.conn.Close()
}

// start creates the session, picks keyboard, pointer and the monitors,
// and starts it.
func start(ctx context.Context) (*Session, error) {
	conn, err := dbus.ConnectSessionBus(dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
	if err != nil {
		return nil, fmt.Errorf("portal: %w", err)
	}
	s := &Session{conn: conn, obj: conn.Object(busName, busPath), closed: make(chan struct{})}
	ok := false
	defer func() {
		if !ok {
			s.close()
		}
	}()
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	r := requester{s: s, signals: signals}

	res, err := r.call(ctx, remoteIface+".CreateSession", map[string]dbus.Variant{
		"session_handle_token": dbus.MakeVariant(token()),
	})
	if err != nil {
		return nil, err
	}
	handle, _ := res["session_handle"].Value().(string)
	if handle == "" {
		return nil, errors.New("portal: no session handle")
	}
	s.handle = dbus.ObjectPath(handle)
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(s.handle), dbus.WithMatchInterface(sessionIface), dbus.WithMatchMember("Closed")); err != nil {
		return nil, fmt.Errorf("portal: %w", err)
	}

	devices := uint32(deviceKeyboard | devicePointer)
	if v, err := s.obj.GetProperty(remoteIface + ".AvailableDeviceTypes"); err == nil {
		if have, ok := v.Value().(uint32); ok {
			devices &= have
		}
	}
	opts := map[string]dbus.Variant{
		"types":        dbus.MakeVariant(devices),
		"persist_mode": dbus.MakeVariant(uint32(2)), // until revoked
	}
	if t := loadToken(); t != "" {
		opts["restore_token"] = dbus.MakeVariant(t)
	}
	if _, err := r.call(ctx, remoteIface+".SelectDevices", s.handle, opts); err != nil {
		return nil, err
	}

	cursorMode := uint32(cursorEmbedded)
	if v, err := s.obj.GetProperty(castIface + ".AvailableCursorModes"); err == nil {
		if have, ok := v.Value().(uint32); ok && have&cursorMetadata != 0 {
			cursorMode = cursorMetadata
		}
	}
	s.cursor = cursorMode == cursorMetadata
	if _, err := r.call(ctx, castIface+".SelectSources", s.handle, map[string]dbus.Variant{
		"types":       dbus.MakeVariant(uint32(sourceMonitor)),
		"multiple":    dbus.MakeVariant(true),
		"cursor_mode": dbus.MakeVariant(cursorMode),
	}); err != nil {
		return nil, err
	}

	res, err = r.call(ctx, remoteIface+".Start", s.handle, "", map[string]dbus.Variant{})
	if err != nil {
		return nil, err
	}
	s.streams = parseStreams(res["streams"])
	if len(s.streams) == 0 {
		return nil, errors.New("portal: the session shares no monitor")
	}
	granted, _ := res["devices"].Value().(uint32)
	s.control = granted&(deviceKeyboard|devicePointer) != 0
	// The choice is remembered only with the mouse and keyboard: shared
	// without them, the desktop asks again next time.
	if t, _ := res["restore_token"].Value().(string); t != "" && s.control {
		saveToken(t)
	} else if !s.control {
		saveToken("")
	}
	s.input = make(chan event, 256)
	slog.Info("wayland desktop shared", "monitors", len(s.streams), "keyboard", granted&deviceKeyboard != 0, "pointer", granted&devicePointer != 0, "cursor_apart", s.cursor, "remembered", s.control && res["restore_token"].Value() != nil)
	go s.watch(signals)
	go s.send()
	ok = true
	return s, nil
}

// watch ends the session when the desktop closes it.
func (s *Session) watch(signals <-chan *dbus.Signal) {
	for {
		select {
		case sig, ok := <-signals:
			if !ok {
				return
			}
			if sig.Path == s.handle && sig.Name == sessionIface+".Closed" {
				mu.Lock()
				if active == s {
					active = nil
				}
				mu.Unlock()
				s.close()
				return
			}
		case <-s.closed:
			return
		}
	}
}

func parseStreams(v dbus.Variant) []Stream {
	var raw []struct {
		Node  uint32
		Props map[string]dbus.Variant
	}
	if err := dbus.Store([]any{v.Value()}, &raw); err != nil {
		return nil
	}
	out := make([]Stream, 0, len(raw))
	next := 0
	for _, r := range raw {
		st := Stream{Node: r.Node}
		if w, h, ok := pair(r.Props["size"]); ok {
			st.W, st.H = w, h
		}
		if x, y, ok := pair(r.Props["position"]); ok {
			st.X, st.Y = x, y
		} else {
			// No place given: side by side.
			st.X = next
		}
		next = st.X + st.W
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Y < out[j].Y
	})
	return out
}

// pair reads a stream property of two integers, a D-Bus (ii).
func pair(v dbus.Variant) (int, int, bool) {
	if v.Value() == nil {
		return 0, 0, false
	}
	var p struct{ A, B int32 }
	if err := dbus.Store([]any{v.Value()}, &p); err != nil {
		return 0, 0, false
	}
	return int(p.A), int(p.B), true
}

// requester makes the portal's asynchronous calls: each answers through a
// Request object whose Response signal carries the result.
type requester struct {
	s       *Session
	signals chan *dbus.Signal
}

var tokens atomic.Uint64

func token() string { return fmt.Sprintf("desk%d_%d", os.Getpid(), tokens.Add(1)) }

func (r requester) call(ctx context.Context, method string, args ...any) (map[string]dbus.Variant, error) {
	opts, _ := args[len(args)-1].(map[string]dbus.Variant)
	if opts == nil {
		return nil, errors.New("portal: the last argument must be the options")
	}
	t := token()
	opts["handle_token"] = dbus.MakeVariant(t)
	names := r.s.conn.Names()
	if len(names) == 0 {
		return nil, errors.New("portal: no bus name")
	}
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	path := dbus.ObjectPath(busPath + "/request/" + sender + "/" + t)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(requestIface), dbus.WithMatchMember("Response")}
	if err := r.s.conn.AddMatchSignal(match...); err != nil {
		return nil, fmt.Errorf("portal: %w", err)
	}
	defer r.s.conn.RemoveMatchSignal(match...)
	if err := r.s.obj.CallWithContext(ctx, method, 0, args...).Err; err != nil {
		return nil, fmt.Errorf("portal: %s: %w", method, err)
	}
	// The desktop may be asking the person: give them time.
	timeout := time.NewTimer(3 * time.Minute)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, fmt.Errorf("portal: %s: no answer", method)
		case sig := <-r.signals:
			if sig.Path != path || sig.Name != requestIface+".Response" || len(sig.Body) < 2 {
				continue
			}
			code, _ := sig.Body[0].(uint32)
			res, _ := sig.Body[1].(map[string]dbus.Variant)
			switch code {
			case 0:
				return res, nil
			case 1:
				return nil, ErrDenied
			default:
				return nil, fmt.Errorf("portal: %s failed", method)
			}
		}
	}
}

// The restore token lives next to the host's settings, for this user only.
func tokenPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "go-link-desk", "wayland-restore-token")
}

func loadToken() string {
	p := tokenPath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func saveToken(t string) {
	p := tokenPath()
	if p == "" {
		return
	}
	if t == "" {
		_ = os.Remove(p)
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	_ = os.WriteFile(p, []byte(t+"\n"), 0o600)
}

// Input: the portal's Notify calls, one at a time and each after the
// previous one's answer. The portal handles calls on a pool of threads,
// so calls sent without waiting can reach the desktop out of order (keys
// typed as "hlelo"). A goroutine of the session's sends them; moves are
// dropped while its queue is full, keys and buttons never.

func (s *Session) notify(method string, args ...any) {
	if s.isClosed() {
		return
	}
	ev := event{method: method, args: append([]any{s.handle, map[string]dbus.Variant{}}, args...)}
	select {
	case s.input <- ev:
	case <-s.closed:
	}
}

func (s *Session) move(method string, args ...any) {
	if s.isClosed() {
		return
	}
	ev := event{method: method, args: append([]any{s.handle, map[string]dbus.Variant{}}, args...), motion: true}
	select {
	case s.input <- ev:
	default: // behind: the next move puts the pointer where it goes
	}
}

func (s *Session) send() {
	warned := false
	for {
		select {
		case ev := <-s.input:
			if ev.pause > 0 {
				time.Sleep(ev.pause)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := s.obj.CallWithContext(ctx, remoteIface+"."+ev.method, 0, ev.args...).Err
			cancel()
			if err != nil && !warned {
				warned = true
				slog.Warn("the desktop refused input", "call", ev.method, "err", err)
			}
		case <-s.closed:
			return
		}
	}
}

// Pause waits d between the calls before and after it (for an input
// method that reads keys at its own pace).
func (s *Session) Pause(d time.Duration) {
	select {
	case s.input <- event{pause: d}:
	case <-s.closed:
	}
}

// MoveTo puts the pointer at x, y on the desktop (logical pixels), on the
// stream that holds that point.
func (s *Session) MoveTo(x, y float64) {
	st := s.streamAt(x, y)
	if st == nil {
		return
	}
	lx := min(max(x-float64(st.X), 0), float64(st.W)-0.5)
	ly := min(max(y-float64(st.Y), 0), float64(st.H)-0.5)
	s.move("NotifyPointerMotionAbsolute", st.Node, lx, ly)
}

// MoveBy moves the pointer by dx, dy.
func (s *Session) MoveBy(dx, dy float64) { s.move("NotifyPointerMotion", dx, dy) }

// Button presses or releases a button by its evdev code (BTN_*).
func (s *Session) Button(code int32, down bool) {
	s.notify("NotifyPointerButton", code, state(down))
}

// Scroll turns the wheel by steps on an axis (0: vertical, 1: horizontal);
// positive scrolls down or right.
func (s *Session) Scroll(axis uint32, steps int32) {
	s.notify("NotifyPointerAxisDiscrete", axis, steps)
}

// Keycode presses or releases a key by its evdev code (KEY_*): the key at
// that place, whatever the layout.
func (s *Session) Keycode(code int32, down bool) {
	s.notify("NotifyKeyboardKeycode", code, state(down))
}

// Keysym presses or releases the key that types an X11 keysym in the
// desktop's layout.
func (s *Session) Keysym(sym int32, down bool) {
	s.notify("NotifyKeyboardKeysym", sym, state(down))
}

func state(down bool) uint32 {
	if down {
		return 1
	}
	return 0
}

func (s *Session) streamAt(x, y float64) *Stream {
	if len(s.streams) == 0 {
		return nil
	}
	for i := range s.streams {
		st := &s.streams[i]
		if x >= float64(st.X) && x < float64(st.X+st.W) && y >= float64(st.Y) && y < float64(st.Y+st.H) {
			return st
		}
	}
	return &s.streams[0]
}
