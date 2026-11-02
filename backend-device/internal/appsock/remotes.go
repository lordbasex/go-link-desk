// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordlive"
)

// remote is one session to another computer. The picture and sound go to the
// system's decoder over a live link, which the app opens with tag when it
// shows the session (ENGINE.md, Live media).
type remote struct {
	e     *Engine
	id    string
	tag   int
	door  viewer.Door
	s     *viewer.Session
	input *viewer.Input
	ctx   context.Context
	stop  context.CancelFunc

	mu       sync.Mutex
	title    string
	status   viewer.Status
	host     *viewer.HostInfo
	perms    *models.Permissions
	hud      bool
	sound    bool
	preset   viewer.Preset
	retina   bool
	displays []viewer.Display
	current  int
	chat     []map[string]any
	notes    []string
	typing   bool
	sent     *viewer.Sent
	tabs     []viewer.TerminalTab
	files    *viewer.Files
	frames   int
	keys     int
	linked   *viewer.NewLink
	logged   bool
	pointer  map[string]any
	live     *lordlive.Stream
}

// statusWord is ENGINE.md's word for a viewer.Status.
func statusWord(st viewer.Status) string {
	switch st.Kind {
	case viewer.StatusJoining, viewer.StatusConnecting:
		return "connecting"
	case viewer.StatusWaiting:
		return "waiting"
	case viewer.StatusPassword:
		switch {
		case st.Checking:
			return "checking"
		case st.PasswordError != "":
			return "wrong"
		}
		return "password"
	case viewer.StatusPasswordRefused:
		return "wrong"
	case viewer.StatusLive:
		return "live"
	case viewer.StatusReconnecting:
		return "reconnecting"
	case viewer.StatusRejected:
		switch st.Reason {
		case models.RejectTimeout:
			return "timeout"
		case models.RejectBusy:
			return "busy"
		case models.RejectUnavailable:
			return "unavailable"
		case models.RejectOff:
			return "off"
		}
		return "declined"
	case viewer.StatusEnded:
		if st.Reason == "" {
			return "ended"
		}
		return "ended_" + st.Reason
	case viewer.StatusNotFound:
		return "notfound"
	case viewer.StatusRateLimited:
		return "ratelimited"
	case viewer.StatusUnreachable:
		return "unreachable"
	case viewer.StatusUnlinked:
		return "unlinked"
	case viewer.StatusOffline:
		return "offline"
	case viewer.StatusUntrusted:
		return "untrusted"
	case viewer.StatusSelf:
		return "self"
	}
	return "connecting"
}

func (r *remote) view() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := map[string]any{
		"id": r.id, "door": r.doorView(), "title": r.title, "status": statusWord(r.status), "left": r.status.Left, "hud": r.hud, "sound": r.sound,
		"preset": string(r.preset), "retina": r.retina, "current": r.current, "chat": orNone(r.chat), "notes": orNone(r.notes), "typing": r.typing,
		"tag": r.tag, "host": nil, "perms": r.perms, "sent": nil, "tabs": orNone(r.tabs), "frames": r.frames, "keyframes": r.keys, "pointer": r.pointer,
	}
	if r.host != nil {
		m["host"] = map[string]any{"name": r.host.Name, "os": r.host.OS, "terminal": r.host.Terminal}
	}
	ds := make([]map[string]any, 0, len(r.displays))
	for _, d := range r.displays {
		ds = append(ds, map[string]any{"id": d.ID, "name": d.Name, "width": d.Width, "height": d.Height})
	}
	m["displays"] = ds
	if s := r.sent; s != nil {
		m["sent"] = map[string]any{"codec": s.Codec, "width": s.Width, "height": s.Height, "fps": s.FPS, "kbps": s.Kbps, "target": s.TargetKbps, "path": s.EstimateKbps}
	}
	return m
}

func orNone[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (r *remote) doorView() map[string]any {
	switch r.door.Kind {
	case viewer.DoorClaim:
		return map[string]any{"kind": "claim", "code": r.door.Code}
	case viewer.DoorReach:
		return map[string]any{"kind": "reach", "device": r.door.Device.DeviceID, "title": r.title}
	}
	return map[string]any{"kind": "join", "code": r.door.Code}
}

func (e *Engine) pushRemotes() {
	e.mu.Lock()
	list := slices.Clone(e.remotes)
	e.mu.Unlock()
	out := make([]map[string]any, 0, len(list))
	for _, r := range list {
		out = append(out, r.view())
	}
	e.Sock.Set("remotes", out)
}

func (e *Engine) findRemote(id string) *remote {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.remotes {
		if r.id == id {
			return r
		}
	}
	return nil
}

func (e *Engine) closeRemotes() {
	e.mu.Lock()
	list := e.remotes
	e.remotes = nil
	e.mu.Unlock()
	for _, r := range list {
		r.close()
	}
}

func (r *remote) close() {
	r.unwatchLive()
	r.s.HangUp()
	r.stop()
}

func (e *Engine) viewerName() string {
	if n := e.Settings.Get().ViewerName; n != "" {
		return n
	}
	if n := os.Getenv("USER"); n != "" {
		return n
	}
	return e.Hostname
}

// open starts a session to another computer. A session of this build asks
// for what the session's own decoders would take; the picture path decides
// what is finally drawn.
func (e *Engine) open(door viewer.Door, title string) *remote {
	cfg := e.Settings.Get()
	signal := e.SignalURL
	if door.Kind == viewer.DoorReach && door.Device != nil && door.Device.SignalURL != "" {
		signal = door.Device.SignalURL
	}
	e.mu.Lock()
	e.seq++
	r := &remote{e: e, id: fmt.Sprintf("r%d", e.seq), tag: firstRemoteTag + e.seq, door: door, title: title, sound: true, preset: viewer.PresetBalanced}
	e.remotes = append(e.remotes, r)
	e.mu.Unlock()
	r.s = viewer.New(viewer.Options{
		Door: door, Name: e.viewerName(), Agent: "go-link Desk " + strings.TrimPrefix(e.Version, "v") + " on LordOS", Codecs: []string{"h264", "vp8"},
		SignalURL: signal, OwnDeviceID: cfg.DeviceID, Logger: e.log, Events: r.events(),
	})
	r.input = viewer.NewInput(r.s)
	r.watchLive()
	r.ctx, r.stop = context.WithCancel(e.Ctx)
	e.pushRemotes()
	r.s.Dial(r.ctx)
	return r
}

func (r *remote) hearing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sound
}

func (r *remote) set(fn func()) {
	r.mu.Lock()
	fn()
	r.mu.Unlock()
	r.e.pushRemotes()
}

func (r *remote) events() viewer.Events {
	start := time.Now()
	return viewer.Events{
		Status: func(st viewer.Status) {
			r.set(func() { r.status = st })
			if st.Kind == viewer.StatusLive {
				r.remember()
			}
			if st.Over() {
				r.set(func() { r.files = nil })
			}
		},
		Host: func(h viewer.HostInfo) {
			r.set(func() {
				r.host = &h
				if h.Name != "" {
					r.title = h.Name
				}
			})
			r.keepLink()
			if r.door.Kind == viewer.DoorReach && r.door.Device != nil {
				r.e.Remotes.Update(r.door.Device.DeviceID, func(d *models.RemoteDevice) {
					if h.Name != "" {
						d.Name = h.Name
					}
					if h.OS != "" {
						d.OS = h.OS
					}
				})
			}
		},
		Permissions: func(p models.Permissions) {
			r.set(func() { r.perms = &p })
			r.input.Allow(p.Control, p.Clipboard)
		},
		Displays: func(d []viewer.Display, current int) {
			r.set(func() { r.displays, r.current = d, current })
		},
		Sent:   func(s viewer.Sent) { r.set(func() { r.sent = &s }) },
		Notice: func(code string) { r.set(func() { r.notes = append(r.notes, code) }) },
		Video: func(f viewer.Frame) {
			r.mu.Lock()
			r.frames++
			if f.Keyframe {
				r.keys++
			}
			live := r.live
			r.mu.Unlock()
			if live != nil && !live.Video(f.Codec, f.Keyframe, time.Since(start), f.Data) {
				r.s.RequestKeyframe()
			}
		},
		Opus: func(pkt []byte) {
			r.mu.Lock()
			live := r.live
			r.mu.Unlock()
			if live != nil && r.hearing() {
				live.Opus(pkt)
			}
		},
		CursorPos: func(x, y float64, visible bool) {
			r.mu.Lock()
			r.pointer = map[string]any{"x": x, "y": y, "visible": visible}
			r.mu.Unlock()
		},
		Chat: func(l viewer.ChatLine) {
			r.set(func() {
				r.chat = append(r.chat, map[string]any{"fromHost": l.FromHost, "text": l.Text, "at": l.At.UnixMilli()})
			})
		},
		Typing: func(on bool) { r.set(func() { r.typing = on }) },
		Linked: func(l viewer.NewLink) {
			r.mu.Lock()
			r.linked = &l
			r.mu.Unlock()
			r.keepLink()
		},
		Unlinked: func() {
			if r.door.Device != nil {
				r.e.Remotes.Forget(r.door.Device.DeviceID)
			}
		},
		Files:        func(f *viewer.Files) { r.set(func() { r.files = f }) },
		TerminalTabs: func(t []viewer.TerminalTab, _ bool) { r.set(func() { r.tabs = t }) },
	}
}

// keepLink saves a link once the computer's name is known.
func (r *remote) keepLink() {
	r.mu.Lock()
	l, h := r.linked, r.host
	if l == nil || h == nil {
		r.mu.Unlock()
		return
	}
	r.linked = nil
	r.mu.Unlock()
	r.e.Remotes.Link(models.RemoteDevice{DeviceID: l.DeviceID, LinkID: l.LinkID, Key: l.Key, HostKey: l.HostKey, SignalURL: r.e.SignalURL, Name: h.Name, OS: h.OS})
}

// remember adds the session to Recent sessions, or dates the linked computer.
func (r *remote) remember() {
	r.mu.Lock()
	if r.logged {
		r.mu.Unlock()
		return
	}
	r.logged = true
	var h viewer.HostInfo
	if r.host != nil {
		h = *r.host
	}
	r.mu.Unlock()
	if r.door.Kind == viewer.DoorJoin {
		r.e.Remotes.AddRecent(models.RecentSession{Code: r.door.Code, Name: h.Name, OS: h.OS})
	}
	if r.door.Device != nil {
		r.e.Remotes.Update(r.door.Device.DeviceID, func(d *models.RemoteDevice) { d.LastUsed = time.Now() })
	}
}

func (e *Engine) remoteRoutes() {
	s := e.Sock
	on := func(fn func(r *remote, m Msg) (any, error)) Handler {
		return func(m Msg) (any, error) {
			r := e.findRemote(m.Remote)
			if r == nil {
				return nil, Reason("no_session")
			}
			return fn(r, m)
		}
	}
	door := func(kind viewer.DoorKind, code string) (any, error) {
		code = digits(code)
		if len(code) != 9 {
			return nil, Reason("bad_code")
		}
		r := e.open(viewer.Door{Kind: kind, Code: code}, services.FormatCode(code))
		return map[string]string{"remote": r.id}, nil
	}
	s.Handle("connect", func(m Msg) (any, error) {
		if m.Kind == "claim" {
			return door(viewer.DoorClaim, m.Code)
		}
		return door(viewer.DoorJoin, m.Code)
	})
	s.Handle("recent.connect", func(m Msg) (any, error) { return door(viewer.DoorJoin, m.Code) })
	s.HandleSlow("demo", func(Msg) (any, error) {
		ctx, cancel := context.WithTimeout(e.Ctx, 10*time.Second)
		defer cancel()
		find := e.Demo
		if find == nil {
			find = demoCode
		}
		code, err := find(ctx)
		if err != nil {
			e.log.Warn("the demo computer is not available", "err", err)
			return nil, Reason("demo_offline")
		}
		return door(viewer.DoorJoin, code)
	})
	s.Handle("device.connect", func(m Msg) (any, error) {
		d, ok := e.Remotes.Device(m.Device)
		if !ok {
			return nil, Reason("not_found")
		}
		r := e.open(viewer.Door{Kind: viewer.DoorReach, Device: &d}, d.Title())
		return map[string]string{"remote": r.id}, nil
	})
	s.Handle("device.favorite", func(m Msg) (any, error) {
		e.Remotes.Update(m.Device, func(d *models.RemoteDevice) { d.Favorite = !d.Favorite })
		return nil, nil
	})
	s.Handle("device.rename", func(m Msg) (any, error) {
		label := strings.TrimSpace(m.Label)
		if r := []rune(label); len(r) > 64 {
			label = string(r[:64])
		}
		e.Remotes.Update(m.Device, func(d *models.RemoteDevice) { d.Label = label })
		return nil, nil
	})
	s.Handle("device.forget", func(m Msg) (any, error) { e.Remotes.Forget(m.Device); return nil, nil })
	s.Handle("recent.remove", func(m Msg) (any, error) { e.Remotes.RemoveRecent(digits(m.Code)); return nil, nil })
	s.Handle("recent.clear", func(Msg) (any, error) { e.Remotes.ClearRecents(); return nil, nil })

	s.Handle("remote.close", on(func(r *remote, _ Msg) (any, error) {
		e.mu.Lock()
		e.remotes = slices.DeleteFunc(e.remotes, func(x *remote) bool { return x == r })
		e.mu.Unlock()
		r.close()
		e.pushRemotes()
		return nil, nil
	}))
	s.Handle("remote.password", on(func(r *remote, m Msg) (any, error) { r.s.SendPassword(m.Password); return nil, nil }))
	s.Handle("remote.chat", on(func(r *remote, m Msg) (any, error) {
		line, ok := r.s.SendChat(m.Text)
		if !ok {
			return nil, Reason("no_session")
		}
		r.set(func() {
			r.chat = append(r.chat, map[string]any{"fromHost": line.FromHost, "text": line.Text, "at": line.At.UnixMilli()})
		})
		return nil, nil
	}))
	s.Handle("remote.typing", on(func(r *remote, m Msg) (any, error) { r.s.SendTyping(m.On); return nil, nil }))
	s.Handle("remote.tool", on(func(r *remote, m Msg) (any, error) { return nil, r.tool(m) }))
	s.Handle("remote.keys", on(func(r *remote, m Msg) (any, error) {
		r.input.Combo(strings.Split(m.Keys, "+")...)
		return nil, nil
	}))
	// The app holds the clipboard here: the engine has none of its own
	// outside a share.
	s.Handle("remote.typeClipboard", on(func(r *remote, m Msg) (any, error) {
		if m.Text == "" {
			return nil, Reason("empty")
		}
		r.input.TypeText(m.Text)
		return nil, nil
	}))
	s.Handle("remote.pointer", on(func(r *remote, m Msg) (any, error) { r.input.Move(m.X, m.Y, m.Inside); return nil, nil }))
	s.Handle("remote.button", on(func(r *remote, m Msg) (any, error) {
		r.input.Button(m.Button, m.Down, m.X, m.Y, m.Inside)
		return nil, nil
	}))
	s.Handle("remote.wheel", on(func(r *remote, m Msg) (any, error) { r.input.Wheel(m.DX, m.DY, m.Lines); return nil, nil }))
	s.Handle("remote.key", on(func(r *remote, m Msg) (any, error) {
		if m.Evdev != 0 {
			r.input.KeyCode(keymap.FromEvdev(m.Evdev), m.Down, m.Repeat, m.Mods)
			return nil, nil
		}
		r.input.Key(m.Keycode, m.Down, m.Repeat, m.Mods)
		return nil, nil
	}))

	s.HandleSlow("files.list", on(func(r *remote, m Msg) (any, error) {
		f, err := r.filesFor(m)
		if err != nil {
			return nil, err
		}
		entries, err := f.List(m.Path)
		if err != nil {
			return nil, Reason(fileReason(err))
		}
		return map[string]any{"path": m.Path, "entries": orNone(entries)}, nil
	}))
	s.HandleSlow("files.download", on(func(r *remote, m Msg) (any, error) {
		f, err := r.filesFor(m)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(e.Downloads, 0o755); err != nil {
			return nil, err
		}
		if !services.InsideHome(e.Home, e.Downloads) {
			return nil, Reason(services.ReasonOutsideHome)
		}
		name := filepath.Base(m.Path)
		saved, err := f.Download(m.Path, e.Downloads, func(got, size int64) { e.pushTransfer(services.Transfer{Name: name, Size: size, Done: got}) }, r.ctxDone())
		if err != nil {
			return nil, Reason(fileReason(err))
		}
		e.pushTransfer(services.Transfer{Name: name, Done: 1, Size: 1, Over: true})
		return map[string]string{"saved": "Downloads/" + filepath.Base(saved)}, nil
	}))
	// What is sent always lands in the other computer's ~/Downloads: there
	// is no destination to name. Path is the file here, in this home.
	s.HandleSlow("files.upload", on(func(r *remote, m Msg) (any, error) {
		rel := m.Path
		if rel == "" {
			rel = m.Name
		}
		m.Path = rel
		f, err := r.filesFor(m)
		if err != nil {
			return nil, err
		}
		local := filepath.Join(e.Home, filepath.FromSlash(rel))
		if !services.InsideHome(e.Home, local) {
			return nil, Reason(services.ReasonOutsideHome)
		}
		name := filepath.Base(local)
		saved, err := f.Upload(local, func(sent, size int64) {
			e.pushTransfer(services.Transfer{Upload: true, Name: name, Size: size, Done: sent})
		}, r.ctxDone())
		if err != nil {
			return nil, Reason(fileReason(err))
		}
		e.pushTransfer(services.Transfer{Upload: true, Name: name, Done: 1, Size: 1, Over: true})
		return map[string]string{"saved": saved}, nil
	}))
	s.Handle("files.reveal", func(Msg) (any, error) { return map[string]string{"path": e.Downloads}, nil })
}

// fileReason is a viewer's file error as the app gets it.
func fileReason(err error) string {
	if strings.Contains(err.Error(), services.ReasonOutsideHome) || strings.Contains(err.Error(), "not a path in the home folder") {
		return services.ReasonOutsideHome
	}
	return err.Error()
}

// filesFor checks a request on this side first: a path that can't be in a
// home folder doesn't leave this computer.
func (r *remote) filesFor(m Msg) (*viewer.Files, error) {
	if !services.CheckHomePath(m.Path) {
		return nil, Reason(services.ReasonOutsideHome)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.files == nil {
		return nil, Reason("no_files")
	}
	return r.files, nil
}

func (r *remote) ctxDone() <-chan struct{} { return r.ctx.Done() }

func (r *remote) tool(m Msg) error {
	switch m.Tool {
	case "info":
		r.set(func() { r.hud = !r.hud })
	case "sound":
		r.set(func() { r.sound = !r.sound })
	case "preset":
		var p string
		if json.Unmarshal(m.Value, &p) != nil {
			return errBadValue
		}
		r.mu.Lock()
		r.preset = viewer.Preset(p)
		retina := r.retina
		r.mu.Unlock()
		r.s.SetQuality(viewer.Preset(p), retina)
		r.e.pushRemotes()
	case "retina":
		r.mu.Lock()
		r.retina = !r.retina
		p, retina := r.preset, r.retina
		r.mu.Unlock()
		r.s.SetQuality(p, retina)
		r.e.pushRemotes()
	case "display":
		var d int
		if json.Unmarshal(m.Value, &d) != nil {
			return errBadValue
		}
		r.s.SelectDisplay(d)
	case "fullscreen": // the window's own
	default:
		return Reason("unknown")
	}
	return nil
}
