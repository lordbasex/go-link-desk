// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

type fakeLinkTransport struct {
	mu      sync.Mutex
	sent    map[string][]map[string]string
	dropped []string
}

func (f *fakeLinkTransport) send(peerID string, msg []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	var m map[string]string
	_ = json.Unmarshal(msg, &m)
	f.sent[peerID] = append(f.sent[peerID], m)
	return true
}

func (f *fakeLinkTransport) drop(peerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped = append(f.dropped, peerID)
}

func (f *fakeLinkTransport) last(peerID string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.sent[peerID]
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

func (f *fakeLinkTransport) wasDropped(peerID string) bool {
	for i := 0; i < 200; i++ {
		if f.dropped1(peerID) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

func (f *fakeLinkTransport) dropped1(peerID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.dropped {
		if p == peerID {
			return true
		}
	}
	return false
}

func newTestLinks(t *testing.T, saved *[]models.Link, timeout time.Duration) (*LinkService, *fakeLinkTransport, *StatusService) {
	t.Helper()
	hostKey, err := linkauth.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	l, tr, status := newTestLinksWith(t, saved, timeout, hostKey)
	return l, tr, status
}

// newTestLinksWith builds a service with a given device key, so a "restart"
// can keep the same key the browser pinned.
func newTestLinksWith(t *testing.T, saved *[]models.Link, timeout time.Duration, hostKey linkauth.Key) (*LinkService, *fakeLinkTransport, *StatusService) {
	t.Helper()
	status := NewStatusService(deviceID, "test", "ws://x")
	l := NewLinkService(LinkConfig{
		DeviceID:    deviceID,
		HostKey:     hostKey,
		Links:       *saved,
		Save:        func(links []models.Link) error { *saved = links; return nil },
		AuthTimeout: timeout,
		HangUpDelay: time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, status)
	tr := &fakeLinkTransport{sent: map[string][]map[string]string{}}
	l.SetTransport(tr.send, tr.drop)
	return l, tr, status
}

const testNonce = "nonce-0123456789abcdef"

// claim links a browser by its public key and returns the link id, the key
// it made, and the host key it pinned.
func claim(t *testing.T, l *LinkService, tr *fakeLinkTransport, peer, terms string) (linkID string, key linkauth.Key, hostPub linkauth.PublicKey) {
	t.Helper()
	key, err := linkauth.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	l.PairedByCode(peer)
	msg := map[string]string{"type": "auth", "pub": key.Public()}
	if terms != "" {
		msg["terms"] = terms
	}
	b, _ := json.Marshal(msg)
	if !l.Trusted(peer) || !l.HandleMessage(peer, b) {
		t.Fatal("a code-paired browser must be trusted and auth handled")
	}
	ok := tr.last(peer)
	if ok["type"] != "auth_ok" || ok["device_id"] != deviceID || ok["link_id"] == "" || ok["host_key"] == "" {
		t.Fatalf("auth_ok = %+v", ok)
	}
	hostPub, err = linkauth.ParsePublic(ok["host_key"])
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	return ok["link_id"], key, hostPub
}

// reach proves a link both ways and reports whether the browser got auth_ok.
func reach(t *testing.T, l *LinkService, tr *fakeLinkTransport, peer, linkID, terms string, key linkauth.Key, hostPub linkauth.PublicKey, host, viewer string) bool {
	t.Helper()
	l.Reached(peer)
	l.HandleMessage(peer, []byte(`{"type":"auth_challenge","link_id":"`+linkID+`","nonce":"`+testNonce+`"}`))
	proof := tr.last(peer)
	if proof["type"] != "auth_proof" {
		return false
	}
	if !hostPub.Verify(linkauth.HostProofMessage(testNonce, host, viewer), proof["sig"]) {
		t.Fatal("the host's proof did not verify against its pinned key")
	}
	sig, err := key.Sign(linkauth.LinkProofMessage(testNonce, host, viewer))
	if err != nil {
		t.Fatal(err)
	}
	msg := map[string]string{"type": "auth", "link_id": linkID, "sig": sig}
	if terms != "" {
		msg["terms"] = terms
	}
	b, _ := json.Marshal(msg)
	l.HandleMessage(peer, b)
	return tr.last(peer)["type"] == "auth_ok"
}

func TestLinkRememberedAndBack(t *testing.T) {
	var saved []models.Link
	hostKey, err := linkauth.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	l, tr, status := newTestLinksWith(t, &saved, time.Minute, hostKey)
	if l.Linked() {
		t.Fatal("no browser yet")
	}

	// First time: the pairing code; auth keeps the browser's public key, and
	// hands back no secret.
	linkID, key, hostPub := claim(t, l, tr, "b1", "")
	ok := tr.last("b1")
	if ok["token"] != "" {
		t.Fatalf("auth_ok still carries a token: %+v", ok)
	}
	if ok["host_key"] != hostKey.Public() {
		t.Fatal("auth_ok did not carry the host's device key")
	}
	if len(saved) != 1 || saved[0].ID != linkID || saved[0].BrowserKey != key.Public() {
		t.Fatalf("saved = %+v", saved)
	}
	if !l.Linked() || status.Snapshot().SavedLinks != 1 {
		t.Fatal("one browser remembered")
	}

	// The device restarts: a new service loads the saved links and keeps the
	// same device key the browser pinned.
	l2, tr2, status2 := newTestLinksWith(t, &saved, time.Minute, hostKey)
	l2.Reached("b2")
	if l2.Trusted("b2") || l2.HandleMessage("b2", []byte(`{"type":"create_room"}`)) {
		t.Fatal("a reached browser is not trusted and other messages are not auth's")
	}
	if !reach(t, l2, tr2, "b2", linkID, "", key, hostPub, "", "") || !l2.Trusted("b2") {
		t.Fatalf("back = %+v", tr2.last("b2"))
	}
	if tr2.last("b2")["token"] != "" {
		t.Fatal("auth_ok carried a token")
	}
	if len(status2.Snapshot().Peers) != 1 {
		t.Fatal("the browser shows as linked once proven")
	}

	// A wrong signature is refused and dropped.
	l2.Reached("evil")
	l2.HandleMessage("evil", []byte(`{"type":"auth_challenge","link_id":"`+linkID+`","nonce":"`+testNonce+`"}`))
	evilKey, _ := linkauth.NewKey()
	evilSig, _ := evilKey.Sign(linkauth.LinkProofMessage(testNonce, "", ""))
	l2.HandleMessage("evil", []byte(`{"type":"auth","link_id":"`+linkID+`","sig":"`+evilSig+`"}`))
	if bad := tr2.last("evil"); bad["type"] != "auth_failed" || !tr2.wasDropped("evil") || l2.Trusted("evil") {
		t.Fatalf("wrong signature: %+v", bad)
	}

	// The browser unlinks itself.
	l2.HandleMessage("b2", []byte(`{"type":"unlink"}`))
	if tr2.last("b2")["type"] != "unlinked" || !tr2.wasDropped("b2") || len(saved) != 0 || l2.Linked() {
		t.Fatalf("after unlink: %+v", saved)
	}
}

func TestTheLinkRemembersTheAcceptedTerms(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	linkID, key, hostPub := claim(t, l, tr, "b1", "2026-09-28")
	if len(saved) != 1 || saved[0].Terms != "2026-09-28" || saved[0].TermsAt.IsZero() {
		t.Fatalf("saved = %+v", saved)
	}
	// A newer version accepted later is recorded when the browser comes back;
	// anything that is not a version is ignored.
	if !reach(t, l, tr, "b2", linkID, "2027-01-01", key, hostPub, "", "") {
		t.Fatal("reach did not authenticate")
	}
	if saved[0].Terms != "2027-01-01" {
		t.Fatalf("terms = %q", saved[0].Terms)
	}
	reach(t, l, tr, "b3", linkID, "<script>", key, hostPub, "", "")
	if saved[0].Terms != "2027-01-01" {
		t.Fatalf("terms = %q", saved[0].Terms)
	}
}

func TestReachedMustAuthInTime(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, 30*time.Millisecond)
	l.Reached("silent")
	deadline := time.Now().Add(2 * time.Second)
	for !tr.wasDropped("silent") {
		if time.Now().After(deadline) {
			t.Fatal("a browser that never authenticates must be dropped")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUnlinkAll(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	claim(t, l, tr, "a", "")
	claim(t, l, tr, "b", "")
	if len(saved) != 2 {
		t.Fatalf("saved = %d", len(saved))
	}
	l.UnlinkAll()
	if len(saved) != 0 || l.Linked() || !tr.wasDropped("a") || !tr.wasDropped("b") || tr.last("a")["type"] != "unlinked" {
		t.Fatalf("after UnlinkAll: saved=%d", len(saved))
	}
}

func TestTheHostSignsItsProof(t *testing.T) {
	var saved []models.Link
	l, tr, _ := newTestLinks(t, &saved, time.Minute)
	linkID, _, hostPub := claim(t, l, tr, "b1", "")

	// A browser coming back sends a nonce; the host answers with a signature
	// over the nonce and the connection's fingerprints, which the browser
	// checks against the key it pinned.
	l.SetFingerprints(func(peer string) (string, string) {
		if peer != "b2" {
			return "", ""
		}
		return "sha-256 AA", "sha-256 BB"
	})
	l.Reached("b2")
	if !l.HandleMessage("b2", []byte(`{"type":"auth_challenge","link_id":"`+linkID+`","nonce":"`+testNonce+`"}`)) {
		t.Fatal("auth_challenge not handled")
	}
	got := tr.last("b2")
	if got["type"] != "auth_proof" || got["sig"] == "" {
		t.Fatalf("proof %v", got)
	}
	if !hostPub.Verify(linkauth.HostProofMessage(testNonce, "sha-256 AA", "sha-256 BB"), got["sig"]) {
		t.Fatal("the host proof did not verify over the connection's fingerprints")
	}
	// The same signature must not verify for different fingerprints: a
	// signaling server in the middle sees other certificates.
	if hostPub.Verify(linkauth.HostProofMessage(testNonce, "sha-256 XX", "sha-256 BB"), got["sig"]) {
		t.Fatal("the proof verified for the wrong fingerprints")
	}
	// Answering a challenge does not trust anyone: the proof still must come.
	if l.Trusted("b2") {
		t.Fatal("a challenge trusted the browser")
	}

	// An unknown link gets a signed refusal, so the browser can trust it and
	// forget the link; a signaling server could not forge it.
	l.HandleMessage("b3", []byte(`{"type":"auth_challenge","link_id":"nope","nonce":"`+testNonce+`"}`))
	ref := tr.last("b3")
	if ref["type"] != "auth_failed" || ref["sig"] == "" {
		t.Fatalf("unknown link %v", ref)
	}
	if !hostPub.Verify(linkauth.HostUnlinkMessage(testNonce, "", ""), ref["sig"]) {
		t.Fatal("the refusal for an unknown link did not verify")
	}
}
