// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package signalclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// recorder is a Handler that forwards events to channels.
type recorder struct {
	client   *Client
	connects chan Envelope
	messages chan Envelope
	drops    chan error
}

func newRecorder() *recorder {
	return &recorder{
		connects: make(chan Envelope, 10),
		messages: make(chan Envelope, 10),
		drops:    make(chan error, 10),
	}
}

func (r *recorder) OnConnect(h Envelope)   { r.connects <- h }
func (r *recorder) OnMessage(e Envelope)   { r.messages <- e }
func (r *recorder) OnDisconnect(err error) { r.drops <- err }

func wait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
		var zero T
		return zero
	}
}

// fakeServer echoes a code for register and can drop connections.
type fakeServer struct {
	*httptest.Server
	mu    sync.Mutex
	conns []*websocket.Conn
	query chan string
	// turnTTL, when set, puts TURN credentials lasting that long in hello.
	turnTTL time.Duration
}

func newFakeServer(t *testing.T) *fakeServer {
	fs := &fakeServer{query: make(chan string, 10)}
	up := websocket.Upgrader{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.query <- r.URL.RawQuery
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		fs.mu.Lock()
		fs.conns = append(fs.conns, conn)
		n := len(fs.conns)
		fs.mu.Unlock()
		hello := Envelope{Type: TypeHello, PeerID: strings.Repeat("a", 31) + string(rune('0'+n))}
		if fs.turnTTL > 0 {
			exp := time.Now().Add(fs.turnTTL).Unix()
			hello.ICEServers = []ICEServer{{URLs: []string{"stun:x:3478"}}, {URLs: []string{"turn:x:3478"}, Username: strconv.FormatInt(exp, 10) + ":" + hello.PeerID, Credential: "c"}}
		}
		_ = conn.WriteJSON(hello)
		for {
			var env Envelope
			if err := conn.ReadJSON(&env); err != nil {
				return
			}
			if env.Type == TypeRegister {
				_ = conn.WriteJSON(Envelope{Type: TypeCode, Code: "113 134 323"})
			}
		}
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fakeServer) dropAll() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, c := range fs.conns {
		_ = c.Close()
	}
}

func (fs *fakeServer) wsURL() string {
	return "ws" + strings.TrimPrefix(fs.URL, "http") + "/ws"
}

func TestConnectSendAndReconnect(t *testing.T) {
	fs := newFakeServer(t)
	rec := newRecorder()
	c := New(Config{URL: fs.wsURL(), MinBackoff: 20 * time.Millisecond, MaxBackoff: 50 * time.Millisecond}, rec)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	if q := wait(t, fs.query); q != "v=1" {
		t.Fatalf("query = %q", q)
	}
	hello := wait(t, rec.connects)
	if !strings.HasSuffix(hello.PeerID, "1") {
		t.Fatalf("hello %+v", hello)
	}
	if err := c.Send(Envelope{Type: TypeRegister, App: "x"}); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, rec.messages); got.Code != "113 134 323" {
		t.Fatalf("message %+v", got)
	}

	// The server drops the link: the client reports it and reconnects.
	fs.dropAll()
	wait(t, rec.drops)
	if err := c.Send(Envelope{Type: TypeRegister}); err != ErrNotConnected {
		t.Fatalf("send while down: %v", err)
	}
	if hello := wait(t, rec.connects); !strings.HasSuffix(hello.PeerID, "2") {
		t.Fatalf("second hello %+v", hello)
	}

	cancel()
	if err := wait(t, done); err != context.Canceled {
		t.Fatalf("run returned %v", err)
	}
}

func TestUnreachableServerKeepsRetrying(t *testing.T) {
	rec := newRecorder()
	c := New(Config{URL: "ws://127.0.0.1:1/ws", MinBackoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}, rec)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = c.Run(ctx) }()
	wait(t, rec.drops)
	wait(t, rec.drops)
}

func TestCheckURL(t *testing.T) {
	cases := []struct {
		in        string
		encrypted bool
		ok        bool
	}{
		{"wss://signal.example/ws", true, true},
		{"ws://127.0.0.1:8090/ws", false, true},
		{"http://signal.example/ws", false, false},
		{"wss:///ws", false, false},
		{"signal.example", false, false},
		{"::bad", false, false},
	}
	for _, c := range cases {
		enc, err := CheckURL(c.in)
		if (err == nil) != c.ok || enc != c.encrypted {
			t.Errorf("CheckURL(%q) = %v, %v", c.in, enc, err)
		}
	}
}

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"ws://h/ws":      "ws://h/ws?v=1",
		"wss://h/ws?v=1": "wss://h/ws?v=1",
		"wss://h/ws?x=2": "wss://h/ws?v=1&x=2",
	}
	for in, want := range cases {
		if got, err := endpoint(in); err != nil || got != want {
			t.Errorf("endpoint(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := endpoint("http://h/ws"); err == nil {
		t.Error("http scheme accepted")
	}
}

// TURN credentials come only with hello: before they expire the client
// connects again for new ones, but not while someone is in session.
func TestRenewsForFreshTURNCredentialsWhenIdle(t *testing.T) {
	renewRetry = 50 * time.Millisecond
	defer func() { renewRetry = time.Minute }()
	fs := newFakeServer(t)
	fs.turnTTL = 2 * time.Second
	rec := newRecorder()
	var busy atomic.Bool
	busy.Store(true)
	c := New(Config{URL: fs.wsURL(), MinBackoff: 20 * time.Millisecond, Renewable: func() bool { return !busy.Load() }}, rec)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = c.Run(ctx) }()

	wait(t, rec.connects)
	// Due (about 1.5 s in), but someone is in session.
	select {
	case h := <-rec.connects:
		t.Fatalf("reconnected during a session: %+v", h)
	case <-time.After(2500 * time.Millisecond):
	}
	busy.Store(false)
	if err := wait(t, rec.drops); !errors.Is(err, errRenew) {
		t.Fatalf("drop = %v", err)
	}
	if h := wait(t, rec.connects); !strings.HasSuffix(h.PeerID, "2") || len(h.ICEServers) != 2 {
		t.Fatalf("second hello %+v", h)
	}
}

func TestRenewAt(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	exp := func(d time.Duration) string { return strconv.FormatInt(now.Add(d).Unix(), 10) + ":peer" }
	for _, c := range []struct {
		name    string
		servers []ICEServer
		want    time.Time
	}{
		{"none", nil, time.Time{}},
		{"stun only", []ICEServer{{URLs: []string{"stun:x"}}}, time.Time{}},
		{"not the REST format", []ICEServer{{Username: "user", Credential: "x"}}, time.Time{}},
		{"12 h: an hour before", []ICEServer{{Username: exp(12 * time.Hour)}}, now.Add(11 * time.Hour)},
		{"the first to expire", []ICEServer{{Username: exp(12 * time.Hour)}, {Username: exp(4 * time.Hour)}}, now.Add(3 * time.Hour)},
		{"short: a quarter before", []ICEServer{{Username: exp(time.Hour)}}, now.Add(45 * time.Minute)},
		{"already expired: now", []ICEServer{{Username: exp(-time.Minute)}}, now.Add(-time.Minute)},
	} {
		if got := renewAt(c.servers, now); !got.Equal(c.want) {
			t.Errorf("%s: renewAt = %v, want %v", c.name, got, c.want)
		}
	}
}
