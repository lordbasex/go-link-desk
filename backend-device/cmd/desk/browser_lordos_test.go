// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// cdp is just enough of the browser's debugging protocol.
type cdp struct {
	c  *websocket.Conn
	id atomic.Int64
}

func (b *cdp) eval(t *testing.T, js string) string {
	t.Helper()
	id := b.id.Add(1)
	if err := b.c.WriteJSON(map[string]any{"id": id, "method": "Runtime.evaluate", "params": map[string]any{"expression": js, "returnByValue": true}}); err != nil {
		t.Fatal(err)
	}
	for {
		var m struct {
			ID     int64 `json:"id"`
			Result struct {
				Result struct {
					Value any `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		_ = b.c.SetReadDeadline(time.Now().Add(10 * time.Second))
		if err := b.c.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m.ID == id {
			return fmt.Sprint(m.Result.Result.Value)
		}
	}
}

// A real browser at the website, on a real session's API: the visitor's
// consent, picture and pointer. DESK_E2E_BROWSER is the browser's path.
func TestBrowser(t *testing.T) {
	hub(t)
	browser := os.Getenv("DESK_E2E_BROWSER")
	if browser == "" || os.Getenv("DESK_E2E_RUNTIME") == "" {
		t.Skip("DESK_E2E_BROWSER or DESK_E2E_RUNTIME isn't set")
	}
	host := start(t, realShareSignal, nil)
	host.code()
	var s struct {
		Invite string `json:"invite"`
	}
	_ = json.Unmarshal([]byte(host.field("session")), &s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--no-sandbox", "--disable-gpu", "--remote-debugging-port=9333",
		"--user-data-dir="+filepath.Join(t.TempDir(), "profile"), "--window-size=1280,900", "--autoplay-policy=no-user-gesture-required", "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	var ws string
	host.wait("the browser", func() bool {
		r, err := http.Get("http://127.0.0.1:9333/json/list")
		if err != nil {
			return false
		}
		defer r.Body.Close()
		var l []struct {
			Type string `json:"type"`
			URL  string `json:"webSocketDebuggerUrl"`
		}
		_ = json.NewDecoder(r.Body).Decode(&l)
		for _, p := range l {
			if p.Type == "page" {
				ws = p.URL
			}
		}
		return ws != ""
	})
	c, _, err := websocket.DefaultDialer.Dial(ws, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b := &cdp{c: c}
	web := strings.TrimRight(strings.Trim(host.field("webURL"), `"`), "/")
	b.eval(t, fmt.Sprintf("location.href = %q", web+"/c/"+s.Invite))
	host.wait("the page", func() bool {
		return b.eval(t, `(() => { const l=[...document.querySelectorAll('label')].find(x=>/Your name/.test(x.textContent)); const i=l&&(l.control||l.querySelector('input')); if(!i) return 'no';
			Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(i,'Browser test'); i.dispatchEvent(new Event('input',{bubbles:true}));
			const c=[...document.querySelectorAll('button')].find(x=>x.textContent.trim()==='Connect'); if(!c||c.disabled) return 'no'; c.click(); return 'ok'; })()`) == "ok"
	})
	host.wait("the request", func() bool { return strings.Contains(host.field("consent"), `"Browser test"`) })
	host.ask("consent.answer", map[string]any{"consent": host.consentID(), "accept": true, "permissions": everything})
	host.wait("the picture", func() bool {
		return b.eval(t, `String(document.querySelector('video')?.videoWidth ?? 0)`) == "1280"
	})
	frames := b.eval(t, `String(document.querySelector('video').getVideoPlaybackQuality().totalVideoFrames)`)
	t.Log("browser decoded", frames, "frames")
	stats := b.eval(t, `navigator.userAgent`)
	t.Log(stats)
	shot := filepath.Join(os.Getenv("HOME"), "go-link-engine", "browser.txt")
	_ = os.WriteFile(shot, []byte(frames+"\n"+host.field("active")+"\n"), 0o644)
	host.send("session.end", nil)
}
