// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/appsock"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
)

// The test binary is also the engine: nothing a test needs lives in the
// program itself.
func TestMain(m *testing.M) {
	if os.Getenv("DESK_ENGINE") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

func socketpair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	return os.NewFile(uintptr(fds[0]), "a"), os.NewFile(uintptr(fds[1]), "b")
}

// engine is a running engine and the app's end of its socket.
type engine struct {
	t    *testing.T
	home string
	cmd  *exec.Cmd
	app  net.Conn
	done chan struct{}

	mu    sync.Mutex
	state map[string]json.RawMessage
	res   map[int64]map[string]json.RawMessage
	seq   int64
}

// start runs an engine with a home of its own; share is the far end of
// fd 4 (nil: none, so an engine with no API).
func start(t *testing.T, signal string, share *os.File) *engine {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".local", "share", "lordos", "appdata", "go-link-desk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if signal != "" {
		store := repositories.NewConfigFile(filepath.Join(dir, "device.json"))
		cfg, _, err := store.LoadOrCreate()
		if err != nil {
			t.Fatal(err)
		}
		cfg.SignalURL = signal
		if err := store.Save(cfg); err != nil {
			t.Fatal(err)
		}
	}
	mine, theirs := socketpair(t)
	cmd := exec.Command(os.Args[0])
	cmd.Env = []string{"DESK_ENGINE=1", "HOME=" + home, "PATH=/usr/bin:/bin", "USER=tester"}
	cmd.ExtraFiles = []*os.File{theirs}
	if share != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, share)
	}
	if os.Getenv("DESK_ENGINE_LOG") != "" {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = theirs.Close()
	conn, err := net.FileConn(mine)
	if err != nil {
		t.Fatal(err)
	}
	_ = mine.Close()
	e := &engine{t: t, home: home, cmd: cmd, app: conn, done: make(chan struct{}), state: map[string]json.RawMessage{}, res: map[int64]map[string]json.RawMessage{}}
	go func() { _ = cmd.Wait(); close(e.done) }()
	go e.read()
	t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-e.done:
		case <-time.After(6 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	return e
}

func (e *engine) read() {
	for {
		b, err := appsock.ReadFrame(e.app)
		if err != nil {
			return
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		e.mu.Lock()
		if string(m["t"]) == `"state"` {
			var set map[string]json.RawMessage
			_ = json.Unmarshal(m["set"], &set)
			for k, v := range set {
				e.state[k] = v
			}
		} else {
			var id int64
			_ = json.Unmarshal(m["id"], &id)
			e.res[id] = m
		}
		e.mu.Unlock()
	}
}

func (e *engine) field(k string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return string(e.state[k])
}

func (e *engine) send(t string, f map[string]any) {
	e.t.Helper()
	m := map[string]any{"t": t}
	for k, v := range f {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	if err := appsock.WriteFrame(e.app, b); err != nil {
		e.t.Fatal(err)
	}
}

// ask sends a request and waits for its answer: data, reason.
func (e *engine) ask(t string, f map[string]any) (string, string) {
	e.t.Helper()
	e.mu.Lock()
	e.seq++
	id := e.seq
	e.mu.Unlock()
	if f == nil {
		f = map[string]any{}
	}
	f["id"] = id
	e.send(t, f)
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		e.mu.Lock()
		r := e.res[id]
		e.mu.Unlock()
		if r != nil {
			var reason string
			_ = json.Unmarshal(r["err"], &reason)
			return string(r["data"]), reason
		}
	}
	e.t.Fatalf("no answer to %s", t)
	return "", ""
}

func (e *engine) wait(what string, ok func() bool) {
	e.t.Helper()
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	e.t.Fatalf("waiting for %s; remotes %s; active %s; signal %s; consent %s", what, e.field("remotes"), e.field("active"), e.field("signal"), e.field("consent"))
}

func TestWontStartWithoutTheApp(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = []string{"DESK_ENGINE=1", "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "fd 3") && !strings.Contains(string(out), "no app") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestNoTestFlagInThisBuild(t *testing.T) {
	for _, f := range []string{"--test-auto-accept", "--test-unattended", "--test-input-log", "--headless", "--source=testpattern", "--test-viewer=join"} {
		cmd := exec.Command(os.Args[0], f)
		cmd.Env = []string{"DESK_ENGINE=1", "HOME=" + t.TempDir()}
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "not defined") {
			t.Fatalf("%s: %v: %s", f, err, out)
		}
	}
}

// offline is a signaling server nobody answers at.
const offline = "ws://127.0.0.1:9/ws"

func TestFirstStateHasEveryField(t *testing.T) {
	e := start(t, offline, nil)
	e.wait("the first frame", func() bool { return e.field("version") != "" })
	for _, k := range []string{"version", "hostname", "localIP", "webURL", "signalURL", "signal", "session", "active", "pairing", "perms", "config", "links", "devices", "recents", "assistants", "ports", "consent", "callAsk", "call", "chat", "typing", "transfer", "remotes", "update"} {
		if e.field(k) == "" {
			t.Errorf("no %s", k)
		}
	}
	cfg := e.field("config")
	if strings.Contains(cfg, "hash") {
		t.Fatal("the password's hash:", cfg)
	}
	var c struct {
		Unattended struct {
			Enabled     bool `json:"enabled"`
			HasPassword bool `json:"has_password"`
		} `json:"unattended"`
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		t.Fatal(err)
	}
	if c.Unattended.Enabled || c.Unattended.HasPassword {
		t.Fatal("unattended access isn't off by default:", cfg)
	}
	if strings.Contains(e.field("update"), "installing") || !strings.Contains(e.field("update"), "idle") {
		t.Fatal(e.field("update"))
	}
}

func TestEndOfFileEndsTheEngine(t *testing.T) {
	e := start(t, offline, nil)
	e.wait("the first frame", func() bool { return e.field("version") != "" })
	_ = e.app.Close()
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		t.Fatal("it's still running")
	}
}

func TestQuit(t *testing.T) {
	e := start(t, offline, nil)
	e.wait("the first frame", func() bool { return e.field("version") != "" })
	e.send("quit", nil)
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		t.Fatal("it's still running")
	}
}

func TestUnattendedNeedsAPassword(t *testing.T) {
	e := start(t, offline, nil)
	if _, r := e.ask("settings.set", map[string]any{"key": "unattended", "value": true}); r != "password_first" {
		t.Fatal(r)
	}
	if _, r := e.ask("settings.set", map[string]any{"key": "password", "value": "short\nshort"}); r != "password_length" {
		t.Fatal(r)
	}
	if _, r := e.ask("settings.set", map[string]any{"key": "password", "value": "a long one\nanother one"}); r != "password_mismatch" {
		t.Fatal(r)
	}
	if _, r := e.ask("settings.set", map[string]any{"key": "password", "value": "a long one\na long one"}); r != "" {
		t.Fatal(r)
	}
	if _, r := e.ask("settings.set", map[string]any{"key": "unattended", "value": true}); r != "" {
		t.Fatal(r)
	}
	e.wait("the switch", func() bool { return strings.Contains(e.field("config"), `"enabled":true,"has_password":true`) })
	if strings.Contains(e.field("config"), "argon") || strings.Contains(e.field("config"), "$") {
		t.Fatal("hash in config")
	}
	if _, r := e.ask("settings.set", map[string]any{"key": "password_clear"}); r != "" {
		t.Fatal(r)
	}
	e.wait("off with its password", func() bool { return strings.Contains(e.field("config"), `"enabled":false,"has_password":false`) })
}

func TestSettingsRefusals(t *testing.T) {
	e := start(t, offline, nil)
	for _, c := range [][3]any{
		{"signal_url", "http://nope", "bad_server"}, {"web_url", "nope", "bad_website"}, {"udp_port", 80, "bad_port"},
		{"announce_ips", "x.y", "bad_address"}, {"autostart", true, "not_available"}, {"assistant_pass:d", "x", "not_available"},
	} {
		if _, r := e.ask("settings.set", map[string]any{"key": c[0], "value": c[1]}); r != c[2] {
			t.Errorf("%v: %q", c[0], r)
		}
	}
}

func TestOldConsentAnswerCounts(t *testing.T) {
	e := start(t, offline, nil)
	if _, r := e.ask("consent.answer", map[string]any{"consent": "c1", "accept": true, "permissions": models.ViewOnly}); r != "too_late" {
		t.Fatal(r)
	}
}

func TestFilesRuleOnTheAppSide(t *testing.T) {
	e := start(t, offline, nil)
	if _, r := e.ask("connect", map[string]any{"kind": "join", "code": "12"}); r != "bad_code" {
		t.Fatal(r)
	}
	if _, r := e.ask("connect", map[string]any{"kind": "join", "code": "123456789"}); r != "" {
		t.Fatal(r)
	}
	for _, p := range []string{"/etc", "..", "a/../..", "~", "~/x"} {
		for _, kind := range []string{"files.list", "files.download", "files.upload"} {
			if _, r := e.ask(kind, map[string]any{"remote": "r1", "path": p}); r != "outside_home" {
				t.Errorf("%s %q: %q", kind, p, r)
			}
		}
	}
}

// ---- a stand-in for the system's end of the API ----

// fakeShare answers as lord-server does, with a grant and an indicator that
// are always there, and counts what arrives.
type fakeShare struct {
	c net.Conn

	mu      sync.Mutex
	got     map[byte]int
	starts  [][]byte
	busy    byte
	stopped chan struct{}
	running bool
	last    map[byte][]byte
}

func newFakeShare(t *testing.T) (*fakeShare, *os.File) {
	t.Helper()
	mine, theirs := socketpair(t)
	c, err := net.FileConn(mine)
	if err != nil {
		t.Fatal(err)
	}
	_ = mine.Close()
	f := &fakeShare{c: c, got: map[byte]int{}, last: map[byte][]byte{}, stopped: make(chan struct{}, 8)}
	go f.serve()
	t.Cleanup(func() { _ = c.Close() })
	return f, theirs
}

func (f *fakeShare) put(t byte, d []byte) {
	b := binary.LittleEndian.AppendUint32(nil, uint32(1+len(d)))
	b = append(b, t)
	_, _ = f.c.Write(append(b, d...))
}

func (f *fakeShare) count(t byte) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got[t]
}

func (f *fakeShare) serve() {
	var hd [5]byte
	for {
		if _, err := io.ReadFull(f.c, hd[:]); err != nil {
			return
		}
		d := make([]byte, binary.LittleEndian.Uint32(hd[:])-1)
		if _, err := io.ReadFull(f.c, d); err != nil {
			return
		}
		f.mu.Lock()
		f.got[hd[4]]++
		f.last[hd[4]] = d
		busy := f.busy
		f.mu.Unlock()
		switch hd[4] {
		case 51: // query
			f.put(60, []byte{busy, 15, 3, 0, 0, 5, 32, 3, 60, 0})
		case 50: // start
			f.mu.Lock()
			f.running = true
			f.starts = append(f.starts, d)
			f.mu.Unlock()
			f.put(60, []byte{0, 15, 3, 0, 0, 5, 32, 3, 60, 0})
			f.put(1, []byte{0, 5, 32, 3})
			go f.pictures(d[1])
		case 52: // stop
			f.mu.Lock()
			f.running = false
			f.mu.Unlock()
			f.put(62, []byte{13})
			f.stopped <- struct{}{}
		}
	}
}

// pictures sends frames while the share runs: every one a keyframe,
// which is all a viewer's assembler looks at.
func (f *fakeShare) pictures(codec byte) {
	for ms := uint32(0); ; ms += 33 {
		f.mu.Lock()
		on := f.running
		f.mu.Unlock()
		if !on {
			return
		}
		h := []byte{codec, 1, 0, 0}
		h = binary.LittleEndian.AppendUint32(h, ms)
		if codec == 1 {
			h = append(h, 0, 0, 0, 1, 0x65, 0x88, 0x84, 0x21, 0xA0)
		} else {
			h = append(h, 0x10, 0x02, 0x00, 0x9d, 0x01, 0x2a, 0x00, 0x05, 0x20, 0x03)
		}
		f.put(40, h)
		f.put(41, append(binary.LittleEndian.AppendUint32(nil, ms*48), 0xF8, 0xFF, 0xFE))
		time.Sleep(33 * time.Millisecond)
	}
}

func (f *fakeShare) end(reason byte) {
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()
	f.put(62, []byte{reason})
}

// ---- through go-link's real signaling: set DESK_E2E_SIGNAL ----

func hub(t *testing.T) string {
	u := os.Getenv("DESK_E2E_SIGNAL")
	if u == "" {
		t.Skip("DESK_E2E_SIGNAL isn't set")
	}
	return u
}

func (e *engine) code() string {
	e.t.Helper()
	var s struct {
		Code string `json:"code"`
	}
	e.wait("the session code", func() bool {
		_ = json.Unmarshal([]byte(e.field("session")), &s)
		return len(s.Code) == 9
	})
	return s.Code
}

type remoteView struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Frames int    `json:"frames"`
	Chat   []struct {
		FromHost bool   `json:"fromHost"`
		Text     string `json:"text"`
	} `json:"chat"`
}

func (e *engine) remote() remoteView {
	var l []remoteView
	_ = json.Unmarshal([]byte(e.field("remotes")), &l)
	if len(l) == 0 {
		return remoteView{}
	}
	return l[len(l)-1]
}

func (e *engine) consentID() string {
	var c struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(e.field("consent")), &c)
	return c.ID
}

var everything = models.Permissions{View: true, Control: true, Audio: true, Clipboard: true, Files: true}

// A visitor from one engine to another: the consent over the socket, the
// picture from the share, chat, files and input, and Stop.
func TestSession(t *testing.T) {
	u := hub(t)
	share, end := newFakeShare(t)
	host := start(t, u, end)
	_ = end.Close()
	guest := start(t, u, nil)
	code := host.code()

	if d, r := guest.ask("connect", map[string]any{"kind": "join", "code": code}); r != "" {
		t.Fatal(r, d)
	}
	host.wait("the request", func() bool { return strings.Contains(host.field("consent"), `"deadline"`) })
	if !strings.Contains(host.field("consent"), `"warning":true`) || !strings.Contains(host.field("consent"), `"name":"tester"`) {
		t.Fatal(host.field("consent"))
	}
	if share.count(50) != 0 {
		t.Fatal("the screen was shared before anyone accepted")
	}
	if _, r := host.ask("consent.answer", map[string]any{"consent": host.consentID(), "accept": true, "permissions": everything}); r != "" {
		t.Fatal(r)
	}
	guest.wait("live", func() bool { return guest.remote().Status == "live" })
	host.wait("the panel", func() bool { return strings.Contains(host.field("active"), `"name":"tester"`) })
	guest.wait("frames", func() bool { return guest.remote().Frames > 5 })

	// Chat, both ways.
	r := guest.remote()
	guest.send("remote.chat", map[string]any{"remote": r.ID, "text": "hello from the visitor"})
	host.wait("the chat", func() bool { return strings.Contains(host.field("chat"), "hello from the visitor") })
	host.send("chat.send", map[string]any{"text": "hello back"})
	guest.wait("the answer", func() bool {
		for _, c := range guest.remote().Chat {
			if c.FromHost && c.Text == "hello back" {
				return true
			}
		}
		return false
	})

	// Input reaches the share.
	guest.wait("control", func() bool { return strings.Contains(guest.field("remotes"), `"control":true`) })
	guest.send("remote.pointer", map[string]any{"remote": r.ID, "x": 0.5, "y": 0.5, "inside": true})
	host.wait("the pointer", func() bool { return share.count(11) > 0 })
	guest.send("remote.keys", map[string]any{"remote": r.ID, "keys": "KeyA"})
	host.wait("the key", func() bool { return share.count(14) >= 2 })

	// Files: only the home folder, and what's sent goes to Downloads.
	guest.wait("the files channel", func() bool {
		_, reason := guest.ask("files.list", map[string]any{"remote": r.ID, "path": ""})
		return reason != "no_files"
	})
	if err := os.WriteFile(filepath.Join(host.home, "hello.txt"), []byte("from the host"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", filepath.Join(host.home, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(host.home, "pw")); err != nil {
		t.Fatal(err)
	}
	list, reason := guest.ask("files.list", map[string]any{"remote": r.ID, "path": ""})
	if reason != "" || !strings.Contains(list, "hello.txt") || strings.Contains(list, `"out"`) || strings.Contains(list, `"pw"`) {
		t.Fatal(list, reason)
	}
	for _, p := range []string{"out", "pw"} {
		if _, reason := guest.ask("files.download", map[string]any{"remote": r.ID, "path": p}); reason != "outside_home" {
			t.Errorf("%s: %q", p, reason)
		}
	}
	if _, reason := guest.ask("files.download", map[string]any{"remote": r.ID, "path": "hello.txt"}); reason != "" {
		t.Fatal(reason)
	}
	if b, err := os.ReadFile(filepath.Join(guest.home, "Downloads", "hello.txt")); err != nil || string(b) != "from the host" {
		t.Fatal(err, string(b))
	}
	if err := os.WriteFile(filepath.Join(guest.home, "up.txt"), []byte("from the visitor"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d, reason := guest.ask("files.upload", map[string]any{"remote": r.ID, "path": "up.txt"}); reason != "" || !strings.Contains(d, "up.txt") {
		t.Fatal(d, reason)
	}
	if b, err := os.ReadFile(filepath.Join(host.home, "Downloads", "up.txt")); err != nil || string(b) != "from the visitor" {
		t.Fatal(err, string(b))
	}

	// The person presses Stop on the indicator: the visit ends.
	share.end(11)
	guest.wait("the end", func() bool { return strings.HasPrefix(guest.remote().Status, "ended") })
	host.wait("the panel closing", func() bool { return host.field("active") == "null" })
	share.mu.Lock()
	n := len(share.starts)
	share.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d starts", n)
	}
}

func TestDeclined(t *testing.T) {
	u := hub(t)
	share, end := newFakeShare(t)
	host := start(t, u, end)
	_ = end.Close()
	guest := start(t, u, nil)
	guest.ask("connect", map[string]any{"kind": "join", "code": host.code()})
	host.wait("the request", func() bool { return strings.Contains(host.field("consent"), `"deadline"`) })
	host.ask("consent.answer", map[string]any{"consent": host.consentID(), "accept": false})
	guest.wait("the refusal", func() bool { return guest.remote().Status == "declined" })
	if share.count(50) != 0 {
		t.Fatal("shared")
	}
}

// The app gone while the question is open: refused, not accepted, not left
// to run out.
func TestAppGoneRefusesTheVisitor(t *testing.T) {
	u := hub(t)
	share, end := newFakeShare(t)
	host := start(t, u, end)
	_ = end.Close()
	guest := start(t, u, nil)
	guest.ask("connect", map[string]any{"kind": "join", "code": host.code()})
	host.wait("the request", func() bool { return strings.Contains(host.field("consent"), `"deadline"`) })
	_ = host.app.Close()
	guest.wait("the refusal", func() bool {
		s := guest.remote().Status
		return s != "waiting" && s != "connecting"
	})
	if s := guest.remote().Status; s != "unavailable" {
		t.Fatal(s)
	}
	if share.count(50) != 0 {
		t.Fatal("shared")
	}
}

// The session is taken at another door: the person isn't bothered.
func TestSessionInUseAsksNobody(t *testing.T) {
	u := hub(t)
	share, end := newFakeShare(t)
	share.busy = 8 // remote_desktop
	host := start(t, u, end)
	_ = end.Close()
	guest := start(t, u, nil)
	guest.ask("connect", map[string]any{"kind": "join", "code": host.code()})
	guest.wait("the refusal", func() bool { return guest.remote().Status == "unavailable" })
	if host.field("consent") != "null" {
		t.Fatal(host.field("consent"))
	}
}

// Linked computers and unattended access: off by default, then on with a
// password, nobody asked.
func TestLinkAndUnattended(t *testing.T) {
	u := hub(t)
	share, end := newFakeShare(t)
	host := start(t, u, end)
	_ = end.Close()
	guest := start(t, u, nil)
	host.wait("the pairing code", func() bool {
		return strings.Contains(host.field("pairing"), "code") && !strings.Contains(host.field("pairing"), `""`)
	})
	var p struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(host.field("pairing")), &p)

	const pw = "a long password"
	if _, r := host.ask("settings.set", map[string]any{"key": "password", "value": pw + "\n" + pw}); r != "" {
		t.Fatal(r)
	}
	if _, r := host.ask("settings.set", map[string]any{"key": "unattended", "value": true}); r != "" {
		t.Fatal(r)
	}
	guest.ask("connect", map[string]any{"kind": "claim", "code": p.Code})
	host.wait("the request", func() bool { return strings.Contains(host.field("consent"), `"link":true`) })
	host.ask("consent.answer", map[string]any{"consent": host.consentID(), "accept": true, "permissions": everything})
	guest.wait("the link", func() bool { return strings.Contains(guest.field("devices"), `"id"`) })
	host.wait("the link there", func() bool { return strings.Contains(host.field("links"), `"id"`) })
	var devices []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(guest.field("devices")), &devices)
	if !strings.Contains(guest.field("devices"), `"favorite"`) || strings.Contains(guest.field("devices"), "host_key") || strings.Contains(guest.field("devices"), `"key"`) {
		t.Fatal(guest.field("devices"))
	}
	guest.send("remote.close", map[string]any{"remote": guest.remote().ID})
	host.wait("the visit over", func() bool { return host.field("active") == "null" })

	// Turned off, nobody comes back.
	if _, r := host.ask("settings.set", map[string]any{"key": "unattended", "value": false}); r != "" {
		t.Fatal(r)
	}
	guest.ask("device.connect", map[string]any{"device": devices[0].ID})
	guest.wait("off", func() bool { return guest.remote().Status == "off" })
	guest.send("remote.close", map[string]any{"remote": guest.remote().ID})

	// On again, with its password; a wrong one first.
	if _, r := host.ask("settings.set", map[string]any{"key": "unattended", "value": true}); r != "" {
		t.Fatal(r)
	}
	guest.ask("device.connect", map[string]any{"device": devices[0].ID})
	guest.wait("the question", func() bool { return guest.remote().Status == "password" })
	guest.send("remote.password", map[string]any{"remote": guest.remote().ID, "password": "wrong password"})
	guest.wait("wrong", func() bool { return guest.remote().Status == "wrong" })
	guest.send("remote.password", map[string]any{"remote": guest.remote().ID, "password": pw})
	guest.wait("live", func() bool { return guest.remote().Status == "live" })
	if host.field("consent") != "null" {
		t.Fatal("asked:", host.field("consent"))
	}
	host.wait("the panel", func() bool { return strings.Contains(host.field("active"), `"unattended":true`) })
	guest.wait("frames", func() bool { return guest.remote().Frames > 5 })
	var links []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(host.field("links")), &links)
	host.send("link.revoke", map[string]any{"link": links[0].ID})
	guest.wait("revoked", func() bool { return guest.remote().Status == "ended_revoked" || guest.remote().Status == "unlinked" })
	_ = share
}

func TestProtocolAgainstViewOfFakeEngine(t *testing.T) {
	// Every message the app's views send has an answer here: nothing falls
	// through to "unknown".
	e := start(t, offline, nil)
	for _, m := range []string{"code.new", "session.end", "session.permissions", "consent.answer", "chat.send", "chat.typing", "call.answer", "call.set", "call.end", "settings.set", "link.set", "link.revoke", "perm.allow", "connect", "device.connect", "device.favorite", "device.forget", "device.rename", "recent.connect", "recent.remove", "recent.clear", "remote.close", "remote.password", "remote.chat", "remote.typing", "remote.tool", "remote.keys", "remote.typeClipboard", "files.list", "files.download", "files.upload", "files.reveal", "update.check"} {
		if _, r := e.ask(m, nil); r == "unknown" {
			t.Errorf("%s isn't answered", m)
		}
	}
	if _, r := e.ask("nonsense", nil); r != "unknown" {
		t.Fatal(r)
	}
}
