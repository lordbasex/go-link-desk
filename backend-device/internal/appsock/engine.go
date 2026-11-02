// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// Perm is one line of `perms`: ids screen and accessibility.
type Perm struct {
	ID      string `json:"id"`
	Granted bool   `json:"granted"`
}

// Deps is the host the socket drives: uiOptions, plus what the window
// did by itself.
type Deps struct {
	Sock           *Sock
	Ctx            context.Context
	Quit           func()
	Status         *services.StatusService
	NewCode        func()
	EndSession     func()
	SetPermissions func(models.Permissions)
	Consent        *services.SocketConsent
	CallAsk        *services.SocketCallConsent
	Chat           *services.ChatService
	Call           *services.CallService
	Files          *services.FilesService
	Settings       *services.SettingsService
	Links          *services.LinkService
	Remotes        *services.RemotesService
	Ports          *services.PortMapper

	Version, Hostname, LocalIP, WebURL, SignalURL string
	DataDir                                       string // the app's data folder: thumbnails
	Home                                          string
	Downloads                                     string
	// Perms answers from the system's two permissions.
	Perms  func() []Perm
	Logger *slog.Logger
	// Demo finds the demonstration computer's code (nil: the public one).
	Demo func(ctx context.Context) (string, error)
}

// Engine binds the host's services to the socket.
type Engine struct {
	Deps
	log *slog.Logger

	mu      sync.Mutex
	remotes []*remote
	seq     int
	active  bool
}

// Bind fills the state and wires every message of ENGINE.md. Call it before
// Sock.Serve, so the first frame has every field.
func Bind(d Deps) *Engine {
	e := &Engine{Deps: d, log: d.Logger}
	if e.log == nil {
		e.log = slog.Default()
	}
	e.state()
	e.routes()
	return e
}

// Run keeps what no service announces up to date until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if e.Perms != nil {
				e.Sock.Set("perms", e.Perms())
			}
			e.pushRemotes()
		}
	}
}

// AppGone turns down whoever is being asked: the app was the one who could
// answer.
func (e *Engine) AppGone() {
	e.Consent.Gone()
	e.CallAsk.Gone()
	e.closeRemotes()
}

func (e *Engine) state() {
	s := e.Sock
	s.Set("version", e.Version)
	s.Set("hostname", e.Hostname)
	s.Set("localIP", e.LocalIP)
	s.Set("webURL", e.WebURL)
	s.Set("signalURL", e.SignalURL)
	s.Set("consent", nil)
	s.Set("callAsk", nil)
	s.Set("call", nil)
	s.Set("typing", false)
	s.Set("transfer", nil)
	s.Set("update", map[string]any{"state": "idle"})
	s.Set("assistants", map[string]any{"clients": []any{}, "saved": map[string]any{}, "errs": map[string]any{}})
	s.Set("netTest", nil)
	s.Set("remotes", []any{})
	s.Set("chat", []any{})
	if e.Perms != nil {
		s.Set("perms", e.Perms())
	} else {
		s.Set("perms", []Perm{{ID: "screen"}, {ID: "accessibility"}})
	}
	e.pushStatus(e.Status.Snapshot())
	e.Status.OnChange(e.pushStatus)
	e.pushConfig(e.Settings.Get())
	e.Settings.OnChange(e.pushConfig)
	e.pushLinks()
	e.Links.OnChange(e.pushLinks)
	e.pushRemoteList(e.Remotes.Get())
	e.Remotes.OnChange(e.pushRemoteList)
	e.pushPorts()
	if e.Ports != nil {
		e.Ports.OnChange(e.pushPorts)
	}
	e.Chat.OnMessage(func(services.ChatMessage) { e.pushChat() })
	e.Chat.OnTyping(func(on bool) { s.Set("typing", on) })
	e.Call.OnStatus(e.pushCall)
	e.Files.OnChange(e.pushTransfer)
	e.Consent.Show = func(p *services.ConsentPrompt) {
		if p == nil {
			s.Set("consent", nil)
			return
		}
		s.Set("consent", p)
	}
	e.CallAsk.Show = func(name string, video bool) {
		if name == "" {
			s.Set("callAsk", nil)
			return
		}
		s.Set("callAsk", map[string]any{"name": name, "video": video})
	}
}

func ms(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMilli()
}

func (e *Engine) pushStatus(st models.Status) {
	s := e.Sock
	signal := "disconnected"
	switch st.Signal.State {
	case models.SignalConnected:
		signal = "connected"
	case models.SignalConnecting:
		signal = "connecting"
	}
	s.Set("signal", signal)
	s.Set("pairing", map[string]string{"code": st.Pairing.Code})
	var active *models.ActiveSession
	if st.Session == nil {
		s.Set("session", nil)
	} else {
		s.Set("session", map[string]string{"code": st.Session.Code, "invite": st.Session.Invite})
		active = st.Session.Active
	}
	if active == nil {
		s.Set("active", nil)
		e.mu.Lock()
		was := e.active
		e.active = false
		e.mu.Unlock()
		if was {
			s.Set("chat", []any{})
			s.Set("typing", false)
		}
		return
	}
	e.mu.Lock()
	e.active = true
	e.mu.Unlock()
	s.Set("active", map[string]any{
		"peer": active.PeerID, "name": active.Name, "browser": active.Browser, "since": ms(active.Since), "permissions": active.Permissions,
		"unattended": active.Unattended, "recordingSince": ms(active.RecordingSince), "screenshots": active.Screenshots, "lastScreenshot": ms(active.LastScreenshot),
	})
}

// pushConfig never sends the password's hash.
func (e *Engine) pushConfig(c models.Config) {
	e.Sock.Set("config", map[string]any{
		"theme": c.Theme, "app_icon": c.AppIcon, "tray_icon": c.TrayIcon, "signal_url": c.SignalURL, "web_url": c.WebURL,
		"udp_port": c.UDPPort, "map_udp_port": c.MapUDPPort, "announce_ips": orEmpty(c.AnnounceIPs), "language": c.Language,
		"no_update_check": c.NoUpdateCheck, "autostart": false, "idle_minutes": c.IdleMinutes, "lock_on_end": c.LockOnEnd,
		"request_ticks": c.RequestTicks, "viewer_name": c.ViewerName, "session_windows": c.SessionWindows,
		"unattended": map[string]bool{"enabled": c.Unattended.Enabled, "has_password": c.Unattended.PasswordHash != ""},
		"assistants": map[string]bool{"enabled": c.Assistants.Enabled, "act": c.Assistants.Act},
	})
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (e *Engine) pushLinks() {
	links := e.Links.Links()
	out := make([]map[string]any, 0, len(links))
	for _, l := range links {
		out = append(out, map[string]any{"id": l.ID, "name": l.Name, "created": ms(l.CreatedAt), "last_seen": ms(l.LastSeen), "permissions": l.Permissions})
	}
	e.Sock.Set("links", out)
}

// thumb keeps a picture as a file: pictures don't travel on the socket.
func (e *Engine) thumb(jpeg []byte) any {
	if len(jpeg) == 0 || e.DataDir == "" {
		return nil
	}
	sum := sha256.Sum256(jpeg)
	dir := filepath.Join(e.DataDir, "thumbs")
	p := filepath.Join(dir, hex.EncodeToString(sum[:12])+".jpg")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if os.MkdirAll(dir, 0o700) != nil || os.WriteFile(p, jpeg, 0o600) != nil {
		return nil
	}
	return p
}

// pushRemoteList never sends a key.
func (e *Engine) pushRemoteList(r models.Remotes) {
	devices := make([]map[string]any, 0, len(r.Devices))
	for _, d := range r.Devices {
		devices = append(devices, map[string]any{"id": d.DeviceID, "name": d.Name, "label": d.Label, "os": d.OS, "last_used": ms(d.LastUsed), "favorite": d.Favorite, "thumb": e.thumb(d.Thumb)})
	}
	recents := make([]map[string]any, 0, len(r.Recents))
	for _, x := range r.Recents {
		recents = append(recents, map[string]any{"code": x.Code, "name": x.Name, "os": x.OS, "at": ms(x.At), "thumb": e.thumb(x.Thumb)})
	}
	e.Sock.Set("devices", devices)
	e.Sock.Set("recents", recents)
}

func (e *Engine) pushPorts() {
	if e.Ports == nil {
		e.Sock.Set("ports", nil)
		return
	}
	st := e.Ports.State()
	outside := ""
	if st.Outside.IsValid() {
		outside = st.Outside.String()
	}
	e.Sock.Set("ports", map[string]any{"asked": st.Asked, "open": st.Open(), "via": st.Via, "outside": outside, "port": st.Port})
}

func (e *Engine) pushChat() {
	msgs := e.Chat.Messages()
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{"fromViewer": m.FromViewer, "text": m.Text, "at": ms(m.At)})
	}
	e.Sock.Set("chat", out)
}

func (e *Engine) pushCall(c services.CallStatus) {
	if c.State != services.CallActive {
		e.Sock.Set("call", nil)
		return
	}
	e.Sock.Set("call", map[string]any{"name": c.Name, "hostCamera": c.HostCamera, "hostMic": c.HostMic, "viewerCamera": c.ViewerCamera, "viewerMic": c.ViewerMic})
}

func (e *Engine) pushTransfer(t services.Transfer) {
	msg := ""
	if t.Err != nil {
		msg = t.Err.Error()
	}
	e.Sock.Set("transfer", map[string]any{"upload": t.Upload, "name": t.Name, "size": t.Size, "done": t.Done, "over": t.Over, "err": msg})
}

func (e *Engine) routes() {
	s := e.Sock
	done := func(fn func(m Msg)) Handler { return func(m Msg) (any, error) { fn(m); return nil, nil } }
	s.Handle("code.new", done(func(Msg) { e.NewCode() }))
	s.Handle("session.end", done(func(Msg) { e.EndSession() }))
	s.Handle("session.permissions", func(m Msg) (any, error) {
		p, err := perms(m)
		if err != nil {
			return nil, err
		}
		e.SetPermissions(p)
		return nil, nil
	})
	s.Handle("consent.answer", func(m Msg) (any, error) {
		p := models.Permissions{}
		if m.Accept {
			var err error
			if p, err = perms(m); err != nil {
				return nil, err
			}
		}
		if !e.Consent.Answer(m.Consent, m.Accept, p) {
			return nil, Reason("too_late")
		}
		return nil, nil
	})
	s.Handle("chat.send", func(m Msg) (any, error) {
		if !e.Chat.Send(m.Text) {
			return nil, Reason("no_session")
		}
		e.pushChat()
		return nil, nil
	})
	s.Handle("chat.typing", done(func(m Msg) { e.Chat.Typing(m.On) }))
	s.Handle("call.answer", func(m Msg) (any, error) {
		a := services.CallAnswer{Accept: m.Accept, Camera: m.Camera != nil && *m.Camera, Mic: m.Mic != nil && *m.Mic}
		if !e.CallAsk.Answer(a) {
			return nil, Reason("too_late")
		}
		return nil, nil
	})
	s.Handle("call.set", done(func(m Msg) {
		if m.Camera != nil {
			e.Call.SetCamera(*m.Camera)
		}
		if m.Mic != nil {
			e.Call.SetMic(*m.Mic)
		}
	}))
	s.Handle("call.end", done(func(Msg) { e.Call.End() }))
	s.Handle("settings.set", func(m Msg) (any, error) { return nil, e.setting(m.Key, m.Value) })
	s.Handle("link.set", func(m Msg) (any, error) {
		ok := e.Links.SetLink(m.Link, func(l *models.Link) {
			switch m.What {
			case "control":
				l.Permissions.Control = m.On
			case "audio":
				l.Permissions.Audio = m.On
			case "clipboard":
				l.Permissions.Clipboard = m.On
			case "files":
				l.Permissions.Files = m.On
			case "record":
				l.Permissions.Record = m.On
			}
		})
		if !ok {
			return nil, Reason("not_found")
		}
		e.pushLinks()
		return nil, nil
	})
	s.Handle("link.revoke", done(func(m Msg) { e.Links.Revoke(m.Link); e.pushLinks() }))
	// The system asks by itself on the first share; here it can only be
	// looked at again.
	s.Handle("perm.allow", func(Msg) (any, error) {
		if e.Perms != nil {
			s.Set("perms", e.Perms())
		}
		return nil, Reason("system_asks")
	})
	s.Handle("update.check", done(func(Msg) { s.Set("update", map[string]any{"state": "idle"}) }))
	s.HandleSlow("network.test", e.networkTest)
	s.Handle("quit", done(func(Msg) { e.Quit() }))
	e.remoteRoutes()
}

func perms(m Msg) (models.Permissions, error) {
	var p models.Permissions
	if m.Permissions == nil || json.Unmarshal(*m.Permissions, &p) != nil {
		return p, Reason("bad_permissions")
	}
	p.View = true
	return p, nil
}

var errBadValue = Reason("bad_value")

// setting is the window's Settings: the same rules, the same refusals.
func (e *Engine) setting(key string, raw json.RawMessage) error {
	set := func(fn func(*models.Config)) error { return e.Settings.Update(fn) }
	var str string
	var on bool
	var num int
	_ = json.Unmarshal(raw, &str)
	_ = json.Unmarshal(raw, &on)
	if json.Unmarshal(raw, &num) != nil && str != "" {
		n, err := strconv.Atoi(strings.TrimSpace(str))
		if err == nil {
			num = n
		}
	}
	switch key {
	case "theme":
		return set(func(c *models.Config) { c.Theme = str })
	case "app_icon", "tray_icon":
		if str != "dark" {
			str = ""
		}
		return set(func(c *models.Config) {
			if key == "app_icon" {
				c.AppIcon = str
			} else {
				c.TrayIcon = str
			}
		})
	case "signal_url":
		v := strings.TrimSpace(str)
		if v != "" {
			if _, err := signalclient.CheckURL(v); err != nil {
				return Reason("bad_server")
			}
		}
		if v == models.DefaultSignalURL {
			v = ""
		}
		return set(func(c *models.Config) { c.SignalURL = v })
	case "web_url":
		v := strings.TrimRight(strings.TrimSpace(str), "/")
		if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
			return Reason("bad_website")
		}
		if v == models.DefaultWebURL {
			v = ""
		}
		return set(func(c *models.Config) { c.WebURL = v })
	case "server_default":
		return set(func(c *models.Config) { c.SignalURL, c.WebURL = "", "" })
	case "udp_port":
		if num != 0 && (num < 1024 || num > 65535) {
			return Reason("bad_port")
		}
		return set(func(c *models.Config) { c.UDPPort = num })
	case "map_udp_port":
		return set(func(c *models.Config) { c.MapUDPPort = on })
	case "announce_ips":
		var ips []string
		if json.Unmarshal(raw, &ips) != nil {
			ips = strings.FieldsFunc(str, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
		}
		for _, ip := range ips {
			if net.ParseIP(ip) == nil {
				return Reason("bad_address")
			}
		}
		return set(func(c *models.Config) { c.AnnounceIPs = ips })
	case "language":
		return set(func(c *models.Config) { c.Language = str })
	case "no_update_check":
		return set(func(c *models.Config) { c.NoUpdateCheck = on })
	case "idle_minutes":
		if num < 0 {
			return errBadValue
		}
		return set(func(c *models.Config) { c.IdleMinutes = num })
	case "lock_on_end":
		return set(func(c *models.Config) { c.LockOnEnd = on })
	case "request_ticks":
		if str != "view" {
			str = ""
		}
		return set(func(c *models.Config) { c.RequestTicks = str })
	case "viewer_name":
		name := strings.TrimSpace(str)
		if r := []rune(name); len(r) > models.MaxKnockName {
			name = string(r[:models.MaxKnockName])
		}
		return set(func(c *models.Config) { c.ViewerName = name })
	case "session_windows":
		return set(func(c *models.Config) { c.SessionWindows = on })
	case "unattended":
		// Unattended access always has a password.
		if on && e.Settings.Get().Unattended.PasswordHash == "" {
			return Reason("password_first")
		}
		return set(func(c *models.Config) { c.Unattended.Enabled = on })
	case "password":
		pw, again, twice := strings.Cut(str, "\n")
		if twice && pw != again {
			return Reason("password_mismatch")
		}
		hash, err := services.HashPassword(pw)
		if errors.Is(err, services.ErrPasswordLength) {
			return Reason("password_length")
		}
		if err != nil {
			return err
		}
		return set(func(c *models.Config) { c.Unattended.PasswordHash = hash })
	case "password_clear":
		return set(func(c *models.Config) { c.Unattended.PasswordHash, c.Unattended.Enabled = "", false })
	case "assistants":
		return set(func(c *models.Config) { c.Assistants.Enabled = on })
	case "assistants_act":
		return set(func(c *models.Config) { c.Assistants.Act = on })
	case "relaunch":
		e.Quit() // the app starts the engine again
		return nil
	}
	return Reason("not_available") // autostart, assistant_pass, assistant_forget
}

// networkTest looks at the network and answers with the page that shows it.
func (e *Engine) networkTest(Msg) (any, error) {
	e.Sock.Set("netTest", "running")
	defer e.Sock.Set("netTest", nil)
	ctx, cancel := context.WithTimeout(e.Ctx, 8*time.Second)
	defer cancel()
	host := "signal.go-link.org"
	if u, err := url.Parse(e.SignalURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	r := netprobe.Probe(ctx, host)
	if e.Ports != nil {
		if st := e.Ports.State(); st.Open() {
			r.Open = st.Via
		}
	}
	web := e.WebURL
	if web == "" {
		web = models.DefaultWebURL
	}
	return map[string]string{"url": strings.TrimRight(web, "/") + "/network" + r.Fragment()}, nil
}

// digits keeps a code's numbers.
var notDigit = regexp.MustCompile(`\D`)

func digits(s string) string { return notDigit.ReplaceAllString(s, "") }

// demoCode reads demo.json, the demonstration computer's code.
func demoCode(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://desk.go-link.org/demo.json", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var f struct {
		Online bool   `json:"online"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10)).Decode(&f); err != nil {
		return "", err
	}
	if code := digits(f.Code); f.Online && len(code) == 9 {
		return code, nil
	}
	return "", errors.New("demo: offline")
}
