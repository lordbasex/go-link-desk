// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package signalclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		_ = conn.WriteJSON(Envelope{Type: TypeHello, PeerID: strings.Repeat("a", 31) + string(rune('0'+n))})
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
