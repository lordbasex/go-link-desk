// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package terminal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestTheShellComesFromTheUserDatabase(t *testing.T) {
	passwd := "root:x:0:0:root:/root:/bin/bash\npi:x:1000:1000:,,,:/home/pi:/usr/bin/zsh\nbad:x:1001:1001::/home/bad:nologin\n"
	if got := shellOf(bufio.NewScanner(strings.NewReader(passwd)), 1000); got != "/usr/bin/zsh" {
		t.Fatalf("got %q", got)
	}
	if got := shellOf(bufio.NewScanner(strings.NewReader(passwd)), 1001); got != "" {
		t.Fatalf("a shell that is not a path: %q", got)
	}
	c := LoginShell()
	if c.Path == "" || !strings.HasPrefix(c.Path, "/") {
		t.Fatalf("shell %q", c.Path)
	}
	for _, kv := range c.Env {
		if strings.HasPrefix(kv, "TERM=") && kv != "TERM=xterm-256color" {
			t.Fatalf("TERM %q", kv)
		}
	}
}

// browser plays the web with Pion: it answers offers and keeps what the
// terminal and control channels bring.
type browser struct {
	t      *testing.T
	id     string
	pc     *webrtc.PeerConnection
	stream *services.StreamService

	mu       sync.Mutex
	remote   bool
	queue    []webrtc.ICECandidateInit
	out      bytes.Buffer
	control  []string
	terminal *webrtc.DataChannel
	tabOut   map[uint32]*bytes.Buffer
}

type relay struct{ b *browser }

func (r relay) Send(env signalclient.Envelope) error {
	go r.b.signal(env.Payload)
	return nil
}

func (b *browser) signal(payload json.RawMessage) {
	var sig services.RTCSignal
	if json.Unmarshal(payload, &sig) != nil {
		return
	}
	switch sig.Kind {
	case "offer":
		_ = b.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sig.SDP})
		answer, _ := b.pc.CreateAnswer(nil)
		_ = b.pc.SetLocalDescription(answer)
		out, _ := json.Marshal(services.RTCSignal{Kind: "answer", SDP: answer.SDP})
		_ = b.stream.HandleSignal(b.id, out)
		b.mu.Lock()
		b.remote = true
		q := b.queue
		b.queue = nil
		b.mu.Unlock()
		for _, c := range q {
			_ = b.pc.AddICECandidate(c)
		}
	case "candidate":
		b.mu.Lock()
		if !b.remote {
			b.queue = append(b.queue, *sig.Candidate)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		_ = b.pc.AddICECandidate(*sig.Candidate)
	}
}

func newBrowser(t *testing.T, id string, stream *services.StreamService) *browser {
	se := webrtc.SettingEngine{}
	se.SetIncludeLoopbackCandidate(true)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	b := &browser{t: t, id: id, pc: pc, stream: stream}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		out, _ := json.Marshal(services.RTCSignal{Kind: "candidate", Candidate: &init})
		_ = stream.HandleSignal(id, out)
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		switch dc.Label() {
		case Channel:
			b.mu.Lock()
			b.terminal = dc
			b.mu.Unlock()
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				if len(m.Data) < 4 {
					return
				}
				id := binary.BigEndian.Uint32(m.Data)
				b.mu.Lock()
				b.out.Write(m.Data[4:])
				if b.tabOut == nil {
					b.tabOut = map[uint32]*bytes.Buffer{}
				}
				if b.tabOut[id] == nil {
					b.tabOut[id] = &bytes.Buffer{}
				}
				b.tabOut[id].Write(m.Data[4:])
				b.mu.Unlock()
			})
		case "control":
			dc.OnMessage(func(m webrtc.DataChannelMessage) {
				b.mu.Lock()
				b.control = append(b.control, string(m.Data))
				b.mu.Unlock()
			})
		}
	})
	return b
}

func (b *browser) wait(what string, ok func() bool) {
	b.t.Helper()
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		b.mu.Lock()
		yes := ok()
		b.mu.Unlock()
		if yes {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.t.Fatalf("no %s; terminal: %q; control: %v", what, b.out.String(), b.control)
}

// The terminals are the computer's, like screen's: a viewer opens, names
// and closes them; exit closes only its own; and a viewer that leaves
// finds them again, with what they showed meanwhile.
func TestTerminalsOutliveTheViewer(t *testing.T) {
	ice := services.NewICEStore()
	api, err := services.NewWebRTCAPI(0, true)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, IncludeLoopback: true, Logger: quiet, Source: "terminal", Features: []string{"terminal-tabs"},
		DataOnly: true, Channels: []string{Channel}, Welcome: services.SessionMeta{Name: "pi", OS: "linux"},
	}, ice)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = stream.Run(ctx) }()

	sh := ShellConfig{Path: "/bin/sh", Home: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin", "TERM=xterm-256color", "PS1=$ "}}
	term := NewService(sh, stream.SendControl, quiet)
	defer term.Close()
	stream.OnChannel(term.Channel)
	control := func(peer string, v map[string]any) {
		b, _ := json.Marshal(v)
		term.HandleControl(peer, v["type"].(string), b)
	}
	connect := func(id string) *browser {
		b := newBrowser(t, id, stream)
		stream.SetSender(relay{b})
		// The session lets the viewer in before its connection exists (a link).
		term.Allow(id, models.Permissions{View: true, Control: true})
		if err := stream.AddViewer(id, nil); err != nil {
			t.Fatal(err)
		}
		b.wait("the terminal channel", func() bool { return b.terminal != nil && b.terminal.ReadyState() == webrtc.DataChannelStateOpen })
		return b
	}
	typeIn := func(b *browser, id uint32, s string) { _ = b.terminal.Send(frame(id, []byte(s))) }
	lastTabs := func(b *browser) string {
		for i := len(b.control) - 1; i >= 0; i-- {
			if strings.Contains(b.control[i], `"type":"terminal_tabs"`) {
				return b.control[i]
			}
		}
		return ""
	}
	tabOut := func(b *browser, id uint32) string {
		if b.tabOut[id] == nil {
			return ""
		}
		return b.tabOut[id].String()
	}

	b := connect("V")
	b.wait("the welcome with tabs", func() bool { return strings.Contains(strings.Join(b.control, " "), `"features":["terminal-tabs"]`) })
	b.wait("a first terminal", func() bool { return strings.Contains(lastTabs(b), `"name":"shell"`) })
	if strings.Contains(strings.Join(b.control, " "), `"resumed"`) {
		t.Fatal("a first terminal was taken as resumed")
	}
	typeIn(b, 1, "echo hola-$((40+2))\n")
	b.wait("its output", func() bool { return strings.Contains(tabOut(b, 1), "hola-42") })

	control("V", map[string]any{"type": "terminal_open", "name": "logs"})
	b.wait("a second terminal", func() bool { return strings.Contains(lastTabs(b), `"id":2,"name":"logs"`) })
	control("V", map[string]any{"type": "terminal_rename", "id": 2, "name": "build\u0007 "})
	b.wait("the new name", func() bool { return strings.Contains(lastTabs(b), `"id":2,"name":"build"`) })
	typeIn(b, 2, "exit 4\n")
	b.wait("the second one's end", func() bool {
		for _, m := range b.control {
			if strings.Contains(m, `"type":"terminal_exit","id":2,"code":4`) {
				return true
			}
		}
		return false
	})
	b.wait("one terminal left", func() bool {
		return !strings.Contains(lastTabs(b), `"id":2`) && strings.Contains(lastTabs(b), `"id":1`)
	})

	// The viewer leaves while a command runs; it goes on.
	typeIn(b, 1, "sleep 1; echo despues-$((1+1))\n")
	time.Sleep(200 * time.Millisecond)
	term.Allow("V", models.Permissions{})
	stream.RemoveViewer("V")
	time.Sleep(1500 * time.Millisecond)
	if tabs := term.Tabs(); len(tabs) != 1 || tabs[0].ID != 1 {
		t.Fatalf("the terminals did not outlive the viewer: %+v", tabs)
	}

	// Back: the same terminal, and what it showed meanwhile.
	w := connect("W")
	w.wait("the terminal again", func() bool {
		return strings.Contains(lastTabs(w), `"id":1,"name":"shell"`) && strings.Contains(lastTabs(w), `"resumed":true`)
	})
	w.wait("what happened meanwhile", func() bool {
		return strings.Contains(tabOut(w, 1), "despues-2") && strings.Contains(tabOut(w, 1), "hola-42")
	})
	typeIn(w, 1, "echo otra-vez\n")
	w.wait("it still answers", func() bool { return strings.Contains(tabOut(w, 1), "otra-vez") })

	// Watch only: nothing typed reaches a shell.
	term.Allow("W", models.Permissions{View: true})
	typeIn(w, 1, "echo sin-permiso-$((2+2))\n")
	time.Sleep(400 * time.Millisecond)
	w.mu.Lock()
	leaked := strings.Contains(tabOut(w, 1), "sin-permiso-4")
	w.mu.Unlock()
	if leaked {
		t.Fatal("a viewer that may only watch typed")
	}
}

// Linking is asked in the terminal that runs "link", and nowhere else.
func TestLinkingIsAskedOfTheLinkCommand(t *testing.T) {
	dir, err := os.MkdirTemp("", "glt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir) // a short path: a Unix socket's must fit 104 bytes
	unattended := true
	c := &Control{
		Path: filepath.Join(dir, "c.sock"), Log: quiet,
		Status:      func() Status { return Status{Unattended: unattended} },
		Code:        func() string { return "915355636" },
		SetPassword: func(string) error { return nil },
		Unlink:      func(string) bool { return false },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = c.Serve(ctx) }()
	ask := services.ConsentRequest{PeerID: "B", Knock: models.Knock{Name: "Ana", Browser: "Chrome 141 on macOS"}, Link: true}

	// Nobody runs link: nobody to ask; and a session code is never asked.
	if _, err := c.Ask(ctx, ask); err != services.ErrUnavailable {
		t.Fatalf("got %v", err)
	}
	var conn interface {
		io.ReadWriteCloser
	}
	for i := 0; ; i++ {
		if conn, err = Dial(c.Path); err == nil {
			break
		}
		if i > 50 {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer conn.Close()
	_ = json.NewEncoder(conn).Encode(Request{Cmd: "link"})
	r := bufio.NewReader(conn)
	next := func() Reply {
		line, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var rep Reply
		_ = json.Unmarshal(line, &rep)
		return rep
	}
	if rep := next(); rep.Code != "915355636" {
		t.Fatalf("first %+v", rep)
	}
	room := ask
	room.Link = false
	if _, err := c.Ask(ctx, room); err != services.ErrUnavailable {
		t.Fatalf("a session code was asked: %v", err)
	}

	got := make(chan error, 1)
	var perm models.Permissions
	go func() {
		var err error
		perm, err = c.Ask(ctx, ask)
		got <- err
	}()
	if rep := next(); rep.Ask == nil || rep.Ask.Name != "Ana" {
		t.Fatalf("asked %+v", rep)
	}
	_ = json.NewEncoder(conn).Encode(Answer{Answer: true})
	if err := <-got; err != nil || perm != LinkPermissions {
		t.Fatalf("%v %+v", err, perm)
	}
	if rep := next(); !strings.Contains(rep.Linked, "Ana") {
		t.Fatalf("end %+v", rep)
	}

	// Without a password nobody could come back: link says so first.
	unattended = false
	conn2, err := Dial(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	_ = json.NewEncoder(conn2).Encode(Request{Cmd: "link"})
	line, _ := bufio.NewReader(conn2).ReadBytes('\n')
	if !strings.Contains(string(line), "password first") {
		t.Fatalf("got %s", line)
	}
}
