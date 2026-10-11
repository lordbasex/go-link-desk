// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// unattendedRig is a host with an open room, its links, its settings and
// the unattended service, with the stream played by fakeViewers.
type unattendedRig struct {
	session  *SessionService
	status   *StatusService
	snd      *fakeSender
	viewers  *fakeViewers
	links    *LinkService
	tr       *fakeLinkTransport
	settings *SettingsService
	u        *UnattendedService
	saved    *[]models.Link
	// autoPass: the rig gave the host rigPassword (unattended access needs
	// one), and reach types it.
	autoPass bool
}

const rigPassword = "rig password"

func newUnattendedRig(t *testing.T, saved *[]models.Link, cfg models.Config) *unattendedRig {
	t.Helper()
	s, st, snd, v := openRoom(t)
	links, tr, _ := newTestLinks(t, saved, time.Minute)
	autoPass := false
	if cfg.Unattended.Enabled && cfg.Unattended.PasswordHash == "" {
		hash, err := HashPassword(rigPassword)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Unattended.PasswordHash, autoPass = hash, true
	}
	settings := NewSettingsService(cfg, nil, nil)
	s.SetUnattended(settings)
	u := NewUnattendedService(UnattendedConfig{
		Links: links, Session: s, Settings: settings, Send: tr.send,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	links.SetTransport(tr.send, func(peer string) { s.Drop(peer, models.EndRevoked) })
	return &unattendedRig{session: s, status: st, snd: snd, viewers: v, links: links, tr: tr, settings: settings, u: u, saved: saved, autoPass: autoPass}
}

// claim plays a browser that types the pairing code, is accepted by the
// person at the host and gets its token. It returns the link and token.
func (r *unattendedRig) claim(t *testing.T, peer string) (linkID, token string) {
	t.Helper()
	c := newAskedConsent()
	r.session.SetConsent(c)
	r.u.PairedByCode(peer)
	r.session.AddLink(peer, true)
	signalFrom(r.session, peer, `{"kind":"knock","name":"Ana","ua":"Firefox 140 on Linux"}`)
	if req := c.request(t); !req.Link || req.PeerID != peer {
		t.Fatalf("the person must be asked to link: %+v", req)
	}
	c.answers <- nil
	if got := reply(t, r.snd, peer); got["kind"] != "accepted" {
		t.Fatalf("reply %v", got)
	}
	r.u.HandleControl(peer, []byte(`{"type":"auth"}`))
	ok := r.tr.last(peer)
	if ok["type"] != "auth_ok" || ok["token"] == "" {
		t.Fatalf("auth_ok %+v", ok)
	}
	return ok["link_id"], ok["token"]
}

// reach plays a linked browser that comes back: it knocks, is connected
// gated, checks the device's proof and shows its token.
func (r *unattendedRig) reach(t *testing.T, peer, linkID, token string) {
	t.Helper()
	r.u.Reached(peer)
	r.session.AddLink(peer, false)
	signalFrom(r.session, peer, `{"kind":"knock","name":"Ana","ua":"Firefox 140 on Linux"}`)
	if got := r.viewers.snapshot(); !slices.Contains(got.waiting, peer) || slices.Contains(got.added, peer) {
		t.Fatalf("a linked browser is connected gated: %+v", got)
	}
	nonce := strings.Repeat("n", 32)
	r.u.HandleWaiting(peer, []byte(`{"type":"auth_challenge","link_id":"`+linkID+`","nonce":"`+nonce+`"}`))
	switch p := r.tr.last(peer); p["type"] {
	case "auth_failed":
		return // a link the device no longer has: it cannot prove itself
	case "auth_proof":
	default:
		t.Fatalf("proof %+v", p)
	}
	r.u.HandleWaiting(peer, []byte(`{"type":"auth","link_id":"`+linkID+`","token":"`+token+`"}`))
	if r.autoPass && r.tr.last(peer)["type"] == "password_required" {
		r.u.HandleWaiting(peer, []byte(`{"type":"password","password":"`+rigPassword+`"}`))
	}
}

func TestUnattendedAccessNeedsAPassword(t *testing.T) {
	set := NewSettingsService(models.Config{Unattended: models.Unattended{Enabled: true}}, nil, nil)
	if set.UnattendedOn() {
		t.Fatal("unattended access without a password let linked devices in")
	}
	hash, _ := HashPassword("correct horse")
	_ = set.Update(func(c *models.Config) { c.Unattended.PasswordHash = hash })
	if !set.UnattendedOn() {
		t.Fatal("with a password it is on")
	}
}

func TestALinkKeepsWhatThePersonAllowedAndComesBackWithoutAnyone(t *testing.T) {
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	linkID, token := r.claim(t, "B1")
	if len(saved) != 1 || saved[0].Name != "Firefox 140 on Linux" || saved[0].Permissions != (models.Permissions{View: true, Control: true}) {
		t.Fatalf("link %+v", saved)
	}
	r.session.End(models.EndViewer)

	// The host restarts: the browser comes back with reach, nobody is asked.
	r2 := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	r2.session.SetConsent(NobodyToAsk)
	r2.reach(t, "B2", linkID, token)
	if got := reply(t, r2.snd, "B2"); got["kind"] != "accepted" || got["permissions"].(map[string]any)["control"] != true {
		t.Fatalf("reply %v", got)
	}
	if got := r2.viewers.snapshot(); !slices.Equal(got.admitted, []string{"B2"}) {
		t.Fatalf("not admitted: %+v", got)
	}
	if a := r2.status.Snapshot().Session.Active; a == nil || !a.Unattended || !a.Permissions.Control {
		t.Fatalf("active %+v", a)
	}
}

func TestARevokedLinkIsTurnedAway(t *testing.T) {
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	linkID, token := r.claim(t, "B1")
	r.links.Revoke(linkID)
	if len(saved) != 0 {
		t.Fatal("revoked link kept")
	}
	// Connected right now: told and dropped, its session over.
	if r.tr.last("B1")["type"] != "unlinked" {
		t.Fatalf("not told: %+v", r.tr.last("B1"))
	}
	waitUntil(t, func() bool { return r.status.Snapshot().Session.Active == nil })

	// And when it comes back, its token no longer opens anything.
	r.reach(t, "B2", linkID, token)
	if got := r.tr.last("B2"); got["type"] != "auth_failed" {
		t.Fatalf("revoked token: %+v", got)
	}
	waitUntil(t, func() bool { return slices.Contains(r.viewers.snapshot().removed, "B2") })
	if got := r.viewers.snapshot(); slices.Contains(got.admitted, "B2") {
		t.Fatal("a revoked browser got in")
	}
}

func TestAWrongTokenNeverGetsIn(t *testing.T) {
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	linkID, _ := r.claim(t, "B1")
	r.session.End(models.EndViewer)
	r.reach(t, "X", linkID, strings.Repeat("A", 43))
	if got := r.tr.last("X"); got["type"] != "auth_failed" {
		t.Fatalf("got %+v", got)
	}
	// Whatever it sends after, it is not a session.
	r.u.HandleWaiting("X", []byte(`{"type":"password","password":"whatever1"}`))
	if got := r.viewers.snapshot(); len(got.admitted) != 0 {
		t.Fatalf("admitted %+v", got)
	}
}

func TestUnattendedOffTurnsLinkedBrowsersDown(t *testing.T) {
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	linkID, token := r.claim(t, "B1")
	r.session.End(models.EndViewer)
	_ = r.settings.Update(func(c *models.Config) { c.Unattended.Enabled = false })

	r.u.Reached("B2")
	r.session.AddLink("B2", false)
	signalFrom(r.session, "B2", `{"kind":"knock","name":"Ana"}`)
	if got := reply(t, r.snd, "B2"); got["kind"] != "rejected" || got["reason"] != models.RejectOff {
		t.Fatalf("reply %v", got)
	}
	// Nor can anyone link with the code.
	r.u.PairedByCode("B3")
	r.session.AddLink("B3", true)
	signalFrom(r.session, "B3", `{"kind":"knock","name":"Ana"}`)
	if got := reply(t, r.snd, "B3"); got["kind"] != "rejected" || got["reason"] != models.RejectOff {
		t.Fatalf("reply %v", got)
	}
	if got := r.viewers.snapshot(); len(got.waiting) != 0 {
		t.Fatalf("connected while off: %+v", got)
	}
	_, _ = linkID, token
}

func TestThePasswordComesAfterTheTokenWithLimitedTries(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true, PasswordHash: hash}})
	linkID, token := r.claim(t, "B1")
	r.session.End(models.EndViewer)

	// A password before the token counts for nothing.
	r.session.AddLink("B2", false)
	r.u.Reached("B2")
	r.u.HandleWaiting("B2", []byte(`{"type":"password","password":"correct horse"}`))
	if got := r.viewers.snapshot(); len(got.admitted) != 0 {
		t.Fatal("a password alone let it in")
	}

	r.reach(t, "B2", linkID, token)
	if got := r.tr.last("B2"); got["type"] != "password_required" {
		t.Fatalf("got %+v", got)
	}
	r.u.HandleWaiting("B2", []byte(`{"type":"password","password":"wrong one"}`))
	if got := r.tr.last("B2"); got["type"] != "password_result" || got["reason"] != "wrong" {
		t.Fatalf("got %+v", got)
	}
	if got := r.viewers.snapshot(); len(got.admitted) != 0 {
		t.Fatal("a wrong password let it in")
	}
	r.u.HandleWaiting("B2", []byte(`{"type":"password","password":"correct horse"}`))
	if got := reply(t, r.snd, "B2"); got["kind"] != "accepted" {
		t.Fatalf("reply %v", got)
	}

	// Five wrong ones and the browser is turned away.
	r.session.End(models.EndViewer)
	r.reach(t, "B3", linkID, token)
	for range pinTriesPerPeer {
		r.u.HandleWaiting("B3", []byte(`{"type":"password","password":"nope nope"}`))
	}
	waitUntil(t, func() bool { return slices.Contains(r.viewers.snapshot().removed, "B3") })
	r.u.HandleWaiting("B3", []byte(`{"type":"password","password":"correct horse"}`))
	if got := r.viewers.snapshot(); slices.Contains(got.admitted, "B3") {
		t.Fatal("got in after being turned away")
	}
}

func TestALinkedBrowserWaitsForTheSessionOn(t *testing.T) {
	var saved []models.Link
	r := newUnattendedRig(t, &saved, models.Config{Unattended: models.Unattended{Enabled: true}})
	linkID, token := r.claim(t, "B1") // B1 is in session
	r.u.Reached("B2")
	r.session.AddLink("B2", false)
	signalFrom(r.session, "B2", `{"kind":"knock","name":"Ana"}`)
	if got := reply(t, r.snd, "B2"); got["kind"] != "rejected" || got["reason"] != models.RejectBusy {
		t.Fatalf("reply %v", got)
	}
	_, _ = linkID, token
}

func TestPasswords(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("a short password was taken")
	}
	h, err := HashPassword("ñandú ñandú")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash %q %v", h, err)
	}
	h2, _ := HashPassword("ñandú ñandú")
	if h == h2 {
		t.Fatal("no salt")
	}
	if !CheckPassword(h, "ñandú ñandú") || CheckPassword(h, "ñandú ñandu") || CheckPassword("", "x") || CheckPassword("$argon2id$v=19$m=999999999,t=1,p=1$AAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAA", "x") {
		t.Fatal("check")
	}
}

func TestThePasswordGateLocksAfterTooManyWrongTries(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	g := NewPasswordGate(func() time.Time { return now })
	wrong := func() bool { return false }
	for i := range pinRoomFails - 1 {
		if r := g.Check(string(rune('a'+i)), wrong); r.Reason != "wrong" || r.Left != pinTriesPerPeer-1 {
			t.Fatalf("try %d: %+v", i, r)
		}
	}
	if r := g.Check("z", wrong); r.Reason != "locked" {
		t.Fatalf("not locked: %+v", r)
	}
	if r := g.Check("y", func() bool { t.Fatal("hashed while locked"); return true }); r.Reason != "locked" {
		t.Fatalf("%+v", r)
	}
	now = now.Add(pinLock + time.Second)
	if r := g.Check("y", func() bool { return true }); !r.OK {
		t.Fatalf("still locked: %+v", r)
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(2 * time.Second); !ok(); {
		if time.Now().After(end) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
