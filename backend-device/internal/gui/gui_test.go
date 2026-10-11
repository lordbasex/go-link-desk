// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/permission"
)

func TestTheWindowShowsTheSessionCode(t *testing.T) {
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	asked := 0
	u := newUI(a, Options{Status: st, NewCode: func() { asked++ }, WebURL: "https://desk.example/", Language: "en"})

	u.render(st.Snapshot())
	if !u.copyBtn.Disabled() || !u.newBtn.Disabled() {
		t.Fatal("buttons work without a code")
	}

	st.SetSession(&models.SessionStatus{RoomID: "R", Code: "915355636", Invite: "abcdefghijklmnopqrstuv"})
	u.render(st.Snapshot())
	if u.code.Text != "915 355 636" {
		t.Fatalf("code %q", u.code.Text)
	}
	if u.link.Text != "Or open https://desk.example/c/abcdefghijklmnopqrstuv" {
		t.Fatalf("link %q", u.link.Text)
	}
	u.copyCode()
	u.copied.Stop() // the test ends before the button would change back
	if got := a.Clipboard().Content(); got != "915 355 636" {
		t.Fatalf("clipboard %q", got)
	}
	test.Tap(u.newBtn)
	if asked != 1 {
		t.Fatal("New code did not ask for one")
	}
}

func TestEveryTextIsTranslated(t *testing.T) {
	es, pt := catalogs["es"], catalogs["pt"]
	for k := range es {
		if _, ok := pt[k]; !ok {
			t.Errorf("pt lacks %q", k)
		}
	}
	for k := range pt {
		if _, ok := es[k]; !ok {
			t.Errorf("es lacks %q", k)
		}
	}
}

func TestEveryWindowTextHasATranslation(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\bLf?\("((?:[^"\\]|\\.)*)"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "i18n") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			text, err := strconv.Unquote(`"` + m[1] + `"`)
			if err != nil {
				t.Fatalf("%s: %q: %v", f, m[1], err)
			}
			if _, ok := catalogs["es"][text]; !ok {
				t.Errorf("%s: no Spanish for %q", f, text)
			}
		}
	}
}

func newTestUI(t *testing.T) (*ui, *services.StatusService, *atomic.Int32) {
	t.Helper()
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	ended := &atomic.Int32{}
	u := newUI(a, Options{Status: st, EndSession: func() { ended.Add(1) }, Language: "en"})
	return u, st, ended
}

func TestTheRequestWindowAnswers(t *testing.T) {
	u, _, _ := newTestUI(t)
	req := services.ConsentRequest{PeerID: "P", Knock: models.Knock{Name: "Ana", Browser: "Chrome 141 on macOS"}}
	for _, tc := range []struct {
		tap  func(r *request)
		want error
	}{
		{func(r *request) { test.Tap(r.accept) }, nil},
		{func(r *request) { test.Tap(r.reject) }, services.ErrDeclined},
	} {
		var got []error
		r := u.newRequest(req, time.Now().Add(time.Minute), func(err error) { got = append(got, err) })
		if r.who.Text != "Ana wants to see your screen." || r.browser.Text != "Browser: Chrome 141 on macOS" {
			t.Fatalf("shows %q / %q", r.who.Text, r.browser.Text)
		}
		tc.tap(r)
		tc.tap(r) // a second tap answers nothing more
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("answers %v, want [%v]", got, tc.want)
		}
	}
}

func TestTheScamWarningShowsTheFirstTimeOnly(t *testing.T) {
	u, _, _ := newTestUI(t)
	req := services.ConsentRequest{PeerID: "P"}
	r := u.newRequest(req, time.Now().Add(time.Minute), func(error) {})
	if r.warning == nil {
		t.Fatal("no warning the first time")
	}
	if r.who.Text != "Someone wants to see your screen." {
		t.Fatalf("who %q", r.who.Text)
	}
	test.Tap(r.reject)
	if again := u.newRequest(req, time.Now().Add(time.Minute), func(error) {}); again.warning != nil {
		t.Fatal("warning shown again")
	}
}

func TestTheRequestCountsDown(t *testing.T) {
	u, _, _ := newTestUI(t)
	now := time.Now()
	r := u.newRequest(services.ConsentRequest{}, now.Add(10*time.Second), func(error) {})
	r.tick(now.Add(2500 * time.Millisecond))
	if r.countdown.Text != "Rejected on its own in 8 s." {
		t.Fatalf("countdown %q", r.countdown.Text)
	}
}

func TestAskEndsWithTheViewerOrTheTime(t *testing.T) {
	u, _, _ := newTestUI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := u.Ask(ctx, services.ConsentRequest{PeerID: "P"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestTheSessionShowsItselfUntilItEnds(t *testing.T) {
	u, st, ended := newTestUI(t)
	st.SetSession(&models.SessionStatus{RoomID: "R", Code: "915355636"})
	st.SetActive(&models.ActiveSession{PeerID: "P", Name: "Ana", Browser: "Firefox 140 on Windows", Since: time.Now()})
	u.render(st.Snapshot())
	if !u.panel.shown || u.panel.who.Text != "Ana is viewing your screen" {
		t.Fatalf("panel shown %v: %q", u.panel.shown, u.panel.who.Text)
	}
	if u.state.Text != "In session with Ana" {
		t.Fatalf("state %q", u.state.Text)
	}
	test.Tap(u.panel.disconnect)
	deadline := time.Now().Add(time.Second)
	for ended.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ended.Load() != 1 {
		t.Fatal("Disconnect did not end the session")
	}
	st.SetActive(nil)
	u.render(st.Snapshot())
	if u.panel.shown {
		t.Fatal("the panel outlived the session")
	}
}

func TestThePermissionsShowAndAsk(t *testing.T) {
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	allowed := map[permission.Kind]bool{permission.Accessibility: true}
	var asked []permission.Kind
	u := newUI(a, Options{Status: st, Language: "en", Permissions: &PermissionChecks{
		Granted: func(k permission.Kind) bool { return allowed[k] },
		Request: func(k permission.Kind) bool { asked = append(asked, k); return false },
	}})
	screen, access := u.perms.rows[0], u.perms.rows[1]
	if screen.state.Text != "Not allowed" || screen.button.Hidden || access.state.Text != "Allowed" || !access.button.Hidden {
		t.Fatalf("screen %q (button hidden %v), accessibility %q", screen.state.Text, screen.button.Hidden, access.state.Text)
	}
	test.Tap(screen.button)
	if len(asked) != 1 || asked[0] != permission.ScreenRecording || u.perms.restart.Hidden {
		t.Fatalf("asked %v, restart hint hidden %v", asked, u.perms.restart.Hidden)
	}
	allowed[permission.ScreenRecording] = true
	u.perms.refresh()
	if screen.state.Text != "Allowed" || !u.perms.restart.Hidden {
		t.Fatal("the grant did not show")
	}
}

func TestPermissionsAreAskedForAndChangedLive(t *testing.T) {
	a := test.NewTempApp(t)
	st := services.NewStatusService("d", "1.0.0", "ws://x")
	var set []models.Permissions
	u := newUI(a, Options{Status: st, Language: "en", SetPermissions: func(p models.Permissions) { set = append(set, p) }})
	r := u.newRequest(services.ConsentRequest{PeerID: "P"}, time.Now().Add(time.Minute), func(error) {})
	// Everything is ticked: whoever asks for support accepts at once, and
	// unticks what they do not want.
	if r.perms.value() != fullControl {
		t.Fatalf("the request starts with %+v", r.perms.value())
	}
	test.Tap(r.perms.audio)
	if p := r.perms.value(); p.Audio || !p.Control || !p.Clipboard || !p.Files {
		t.Fatalf("unticking the sound gives %+v", p)
	}
	st.SetSession(&models.SessionStatus{RoomID: "R"})
	st.SetActive(&models.ActiveSession{PeerID: "P", Name: "Ana", Permissions: models.Permissions{View: true, Control: true}})
	u.render(st.Snapshot())
	c := u.panel.checks
	if !c.control.Checked || c.audio.Checked || len(set) != 0 {
		t.Fatalf("panel %+v, set %v", c.value(), set)
	}
	test.Tap(c.control)
	test.Tap(c.clipboard)
	if len(set) != 2 || set[0].Control || !set[0].View || !set[1].Clipboard || set[1].Control {
		t.Fatalf("set %v", set)
	}
}
