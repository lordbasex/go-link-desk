// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package mcpserver

import (
	"strings"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inputproto"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
)

func desktopHost(perms models.Permissions) *fakeHost {
	return &fakeHost{host: viewer.HostInfo{Name: "Studio", OS: "darwin"}, perms: perms, password: "m"}
}

func (h *fakeHost) sent() []inputproto.Packet {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []inputproto.Packet
	for _, b := range h.packets {
		p, err := inputproto.Decode(b)
		if err == nil {
			out = append(out, p)
		}
	}
	h.packets = nil
	return out
}

// A desktop: clicks land where the last screenshot showed, keys and
// shortcuts by position, text as text, the clipboard both ways.
func TestAnAssistantUsesADesktop(t *testing.T) {
	mac := desktopHost(models.Permissions{View: true, Control: true, Clipboard: true, Record: true})
	r := newRig(t, on(true), map[string]string{"dev-mac": "m"}, map[string]*fakeHost{"dev-mac": mac})
	if got, isErr, _ := r.call("connect", map[string]any{"computer": "Office Mac"}); isErr || !strings.Contains(got, "click, scroll, type_text and key") {
		t.Fatalf("connect: %q", got)
	}
	if got, isErr, _ := r.call("click", map[string]any{"computer": "Office Mac", "x": 10, "y": 10}); !isErr || !strings.Contains(got, "screenshot first") {
		t.Fatalf("click before a screenshot: %q", got)
	}
	c := r.server.connected("dev-mac")
	c.mu.Lock()
	c.shotW, c.shotH = 1000, 500 // as a screenshot leaves them
	c.mu.Unlock()

	if got, isErr, _ := r.call("click", map[string]any{"computer": "Office Mac", "x": 499, "y": 249, "button": "right"}); isErr {
		t.Fatal(got)
	}
	ps := mac.sent()
	if len(ps) != 3 || ps[0].Type != inputproto.TypeAbs || ps[1].Type != inputproto.TypeButton || !ps[1].Down || ps[1].Button != inputproto.ButtonRight || ps[2].Down {
		t.Fatalf("click: %+v", ps)
	}
	if d := int(ps[0].X) - 32767; d < -100 || d > 100 || int(ps[0].Y)-32767 < -100 || int(ps[0].Y)-32767 > 100 {
		t.Fatalf("the middle of the screen: %d, %d", ps[0].X, ps[0].Y)
	}
	r.call("click", map[string]any{"computer": "Office Mac", "x": 1, "y": 1, "double": true})
	if n := len(mac.sent()); n != 5 {
		t.Fatalf("double click: %d packets", n)
	}
	if got, isErr, _ := r.call("click", map[string]any{"computer": "Office Mac", "x": 1000, "y": 1}); !isErr || !strings.Contains(got, "outside") {
		t.Fatalf("outside: %q", got)
	}

	r.call("key", map[string]any{"computer": "Office Mac", "keys": "cmd+shift+t"})
	ps = mac.sent()
	want := []string{"MetaLeft", "ShiftLeft", "KeyT"}
	if len(ps) != 6 {
		t.Fatalf("shortcut: %+v", ps)
	}
	for i, code := range want {
		if ps[i].Keycode != keymap.Index(code) || !ps[i].Down || ps[5-i].Keycode != keymap.Index(code) || ps[5-i].Down {
			t.Fatalf("shortcut order: %+v", ps)
		}
	}
	r.call("type_text", map[string]any{"computer": "Office Mac", "text": "hola ñandú"})
	if ps = mac.sent(); len(ps) != 1 || ps[0].Type != inputproto.TypeText || ps[0].Text != "hola ñandú" {
		t.Fatalf("text: %+v", ps)
	}
	r.call("scroll", map[string]any{"computer": "Office Mac", "x": 5, "y": 5, "lines": 3})
	if ps = mac.sent(); len(ps) != 2 || ps[1].Type != inputproto.TypeWheel || ps[1].DY != 3 || ps[1].Mode != inputproto.WheelLines {
		t.Fatalf("scroll: %+v", ps)
	}

	r.call("clipboard_set", map[string]any{"computer": "Office Mac", "text": "copied"})
	if mac.clipboard != "copied" {
		t.Fatalf("clipboard: %q", mac.clipboard)
	}
	mac.ev().Clipboard("from the Mac")
	if got, _, _ := r.call("clipboard_get", map[string]any{"computer": "Office Mac"}); got != "from the Mac" {
		t.Fatalf("clipboard_get: %q", got)
	}
}

// Only looking, or a link that only watches: no input.
func TestDesktopInputNeedsActAndControl(t *testing.T) {
	mac := desktopHost(models.Permissions{View: true, Record: true})
	r := newRig(t, on(false), map[string]string{"dev-mac": "m"}, map[string]*fakeHost{"dev-mac": mac})
	if got, isErr, _ := r.call("key", map[string]any{"computer": "Office Mac", "keys": "enter"}); !isErr || !strings.Contains(got, "only look") {
		t.Fatalf("look only: %q", got)
	}
	if got, isErr, _ := r.call("file_upload", map[string]any{"computer": "Office Mac", "path": "/etc/hosts"}); !isErr || !strings.Contains(got, "only look") {
		t.Fatalf("upload while looking only: %q", got)
	}
	r.cfg = on(true)
	if got, isErr, _ := r.call("key", map[string]any{"computer": "Office Mac", "keys": "enter"}); !isErr || !strings.Contains(got, "only watch") {
		t.Fatalf("no control: %q", got)
	}
	if got, isErr, _ := r.call("file_list", map[string]any{"computer": "Office Mac", "path": ""}); !isErr || !strings.Contains(got, "files") {
		t.Fatalf("no files permission: %q", got)
	}
	if len(mac.sent()) != 0 {
		t.Fatal("input went out")
	}
}

func TestParseKeys(t *testing.T) {
	for in, want := range map[string]string{
		"enter": "Enter", "ESC": "Escape", "ctrl+c": "ControlLeft KeyC", "Cmd + Space": "MetaLeft Space",
		"alt+f4": "AltLeft F4", "shift+arrowdown": "ShiftLeft ArrowDown", "ctrl+alt+delete": "ControlLeft AltLeft Delete",
		"5": "Digit5", "BracketLeft": "BracketLeft", "win": "MetaLeft",
	} {
		got, err := parseKeys(in)
		if err != nil || strings.Join(got, " ") != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ctrl+", "hyper+x", "f99"} {
		if _, err := parseKeys(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}
