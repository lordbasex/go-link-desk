// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

// Messages on the control channel of a linked browser.
const (
	msgAuth       = "auth"
	msgAuthOK     = "auth_ok"
	msgAuthFailed = "auth_failed"
	msgUnlink     = "unlink"
	// A remembered browser first asks the device to prove itself, so its
	// token never goes to whoever answers for the device_id on signalhub.
	msgChallenge = "auth_challenge"
	msgProof     = "auth_proof"
	msgUnlinked  = "unlinked"
)

// errNotLinked is shown by the website when a remembered link is gone.
const errNotLinked = "This browser is no longer linked to the device. Link it again with a new code."

// LinkConfig configures LinkService. Zero values get defaults.
type LinkConfig struct {
	DeviceID string
	// HostKey is this host's device signing key. A linked browser pins its
	// public half and checks every proof the host signs with it.
	HostKey     linkauth.Key
	Links       []models.Link             // loaded from device.json
	Save        func([]models.Link) error // persists the links
	AuthTimeout time.Duration             // default 30 s, for peers that came with reach
	HangUpDelay time.Duration             // default 500 ms, after the last message to a dropped peer
	Now         func() time.Time
	Logger      *slog.Logger
}

// LinkService remembers the browsers linked with a pairing code, so they
// come back without one. On the first link the device gives the browser a
// random token over the encrypted WebRTC channel and keeps only its hash.
// A browser that comes back with reach proves itself with that token
// before it gets anything from the device.
type LinkService struct {
	cfg    LinkConfig
	status *StatusService

	hostKey linkauth.Key

	mu      sync.Mutex
	links   []models.Link
	trusted map[string]string // peer_id -> link id ("" until it is linked)
	timers  map[string]*time.Timer
	// nonces keeps the nonce a reaching browser sent in auth_challenge, so
	// auth can rebuild the message its proof signs over.
	nonces map[string]string
	send   func(peerID string, msg []byte) bool
	drop   func(peerID string)
	// fingerprints: the DTLS fingerprints of a peer's connection.
	fingerprints func(peerID string) (host, viewer string)
	// onChange runs (outside the lock) when a browser becomes trusted or
	// is forgotten.
	onChange func()
	// onAuth runs (outside the lock) when a browser proved its token, or
	// got one: created is true for a link made just now.
	onAuth func(peerID string, link models.Link, created bool)
}

// OnAuthenticated registers who is told that a browser proved its link,
// or was just linked.
func (l *LinkService) OnAuthenticated(fn func(peerID string, link models.Link, created bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onAuth = fn
}

func (l *LinkService) authenticated(peerID string, link models.Link, created bool) {
	l.mu.Lock()
	fn := l.onAuth
	l.mu.Unlock()
	if fn != nil {
		fn(peerID, link, created)
	}
}

// OnChange registers who is told that the trusted browsers changed.
func (l *LinkService) OnChange(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onChange = fn
}

// changed runs the OnChange callback. Never call it holding l.mu.
func (l *LinkService) changed() {
	l.mu.Lock()
	fn := l.onChange
	l.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// NewLinkService builds the service.
func NewLinkService(cfg LinkConfig, status *StatusService) *LinkService {
	if cfg.AuthTimeout <= 0 {
		cfg.AuthTimeout = 30 * time.Second
	}
	if cfg.HangUpDelay <= 0 {
		cfg.HangUpDelay = 500 * time.Millisecond
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Save == nil {
		cfg.Save = func([]models.Link) error { return nil }
	}
	l := &LinkService{
		cfg:     cfg,
		status:  status,
		hostKey: cfg.HostKey,
		links:   slices.Clone(cfg.Links),
		trusted: map[string]string{},
		timers:  map[string]*time.Timer{},
		nonces:  map[string]string{},
	}
	status.SetSavedLinks(len(l.links))
	return l
}

// SetFingerprints gives the DTLS fingerprints of a peer's connection, the
// host's and the viewer's ("" when not known), for the bound proof.
func (l *LinkService) SetFingerprints(fn func(peerID string) (host, viewer string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fingerprints = fn
}

// SetTransport wires the control channel: send a message to a linked
// browser, and drop its connection.
func (l *LinkService) SetTransport(send func(peerID string, msg []byte) bool, drop func(peerID string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.send, l.drop = send, drop
}

// Links returns the remembered browsers.
func (l *LinkService) Links() []models.Link {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.links)
}

// Linked reports whether at least one browser is remembered.
func (l *LinkService) Linked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.links) > 0
}

// Trusted reports whether a peer may use the device: it redeemed a code
// now, or proved its token.
func (l *LinkService) Trusted(peerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.trusted[peerID]
	return ok
}

// ByCode reports whether peerID redeemed the pairing code in this visit
// (it has no link yet, or got it just now).
func (l *LinkService) ByCode(peerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	id, ok := l.trusted[peerID]
	return ok && id == ""
}

// LinkOf returns the link a peer proved, if any.
func (l *LinkService) LinkOf(peerID string) (models.Link, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := l.trusted[peerID]
	i := slices.IndexFunc(l.links, func(r models.Link) bool { return id != "" && r.ID == id })
	if i < 0 {
		return models.Link{}, false
	}
	return l.links[i], true
}

// SetLink changes what a linked browser may do and how it is named.
func (l *LinkService) SetLink(id string, change func(*models.Link)) bool {
	l.mu.Lock()
	i := slices.IndexFunc(l.links, func(r models.Link) bool { return r.ID == id })
	if i < 0 {
		l.mu.Unlock()
		return false
	}
	change(&l.links[i])
	l.links[i].ID = id
	links := slices.Clone(l.links)
	l.mu.Unlock()
	l.persist(links)
	return true
}

// Revoke forgets one linked browser: it can no longer come back, and if
// it is connected now it is told and dropped.
func (l *LinkService) Revoke(id string) {
	l.mu.Lock()
	n := len(l.links)
	l.links = slices.DeleteFunc(l.links, func(r models.Link) bool { return r.ID == id })
	if len(l.links) == n {
		l.mu.Unlock()
		return
	}
	links := slices.Clone(l.links)
	var peers []string
	for p, linkID := range l.trusted {
		if linkID == id {
			peers = append(peers, p)
		}
	}
	l.mu.Unlock()
	l.persist(links)
	l.cfg.Logger.Info("browser link revoked", "link_id", id)
	for _, p := range peers {
		l.reply(p, map[string]string{"type": msgUnlinked})
		l.dropLater(p)
	}
}

// PairedByCode trusts a browser that just redeemed the pairing code. It
// gets its token when it sends auth.
func (l *LinkService) PairedByCode(peerID string) {
	l.mu.Lock()
	l.trusted[peerID] = ""
	l.mu.Unlock()
	l.changed()
}

// PanelLinkID marks a browser that came through the local web panel with
// the panel token: trusted while it stays, never remembered as a link.
const PanelLinkID = "panel"

// PairedByPanel trusts a browser of the local web panel, which proved the
// panel token over its own connection.
func (l *LinkService) PairedByPanel(peerID string) {
	l.mu.Lock()
	l.trusted[peerID] = PanelLinkID
	l.mu.Unlock()
	l.changed()
}

// Reached starts the clock for a browser that came back with reach: it
// must prove its token in time or it is dropped.
func (l *LinkService) Reached(peerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.timers[peerID]; t != nil {
		t.Stop()
	}
	l.timers[peerID] = time.AfterFunc(l.cfg.AuthTimeout, func() {
		if !l.Trusted(peerID) {
			l.cfg.Logger.Warn("browser did not authenticate", "peer_id", peerID)
			l.dropPeer(peerID)
		}
	})
}

// Forget clears a browser that left.
func (l *LinkService) Forget(peerID string) {
	l.mu.Lock()
	delete(l.trusted, peerID)
	delete(l.nonces, peerID)
	if t := l.timers[peerID]; t != nil {
		t.Stop()
		delete(l.timers, peerID)
	}
	l.mu.Unlock()
	l.changed()
}

type linkMessage struct {
	Type   string `json:"type"`
	LinkID string `json:"link_id,omitempty"`
	// Pub is the browser's public key, sent once when it links (claim).
	Pub string `json:"pub,omitempty"`
	// Sig is the browser's signature over the reach proof message.
	Sig   string `json:"sig,omitempty"`
	Nonce string `json:"nonce,omitempty"`
	// Terms: the version of the terms of use accepted in this browser.
	Terms string `json:"terms,omitempty"`
}

// validTerms accepts a terms version as the website sends it (a date).
func validTerms(v string) bool {
	if len(v) == 0 || len(v) > 20 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// HandleMessage processes auth and unlink. It reports false for other
// messages, which the caller may process only for trusted peers.
func (l *LinkService) HandleMessage(peerID string, data []byte) bool {
	var m linkMessage
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	switch m.Type {
	case msgChallenge:
		l.prove(peerID, m)
	case msgAuth:
		l.auth(peerID, m)
	case msgUnlink:
		l.unlink(peerID)
	default:
		return false
	}
	return true
}

// DTLSFingerprint is an SDP's first sha-256 certificate fingerprint, as
// "sha-256 AB:CD:…" (upper case hex), or "" when it has none.
func DTLSFingerprint(sdp string) string {
	for _, line := range strings.Split(sdp, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "a=fingerprint:")
		if !ok {
			continue
		}
		alg, hexs, ok := strings.Cut(strings.TrimSpace(v), " ")
		if ok && strings.EqualFold(alg, "sha-256") {
			return "sha-256 " + strings.ToUpper(strings.TrimSpace(hexs))
		}
	}
	return ""
}

// prove answers auth_challenge: the host signs the connection's nonce and
// DTLS fingerprints with its device key, so the browser can check it against
// the public key it pinned when it linked, before it proves its own key. A
// browser whose link is gone (revoked, or never known) gets a refusal the
// host signs the same way, so it can trust the refusal and forget the link
// — a signaling server cannot forge it to make the browser re-pair.
func (l *LinkService) prove(peerID string, m linkMessage) {
	if len(m.Nonce) < 16 || len(m.Nonce) > 128 {
		return
	}
	l.mu.Lock()
	known := slices.ContainsFunc(l.links, func(r models.Link) bool { return r.ID == m.LinkID })
	l.nonces[peerID] = m.Nonce
	fingerprints := l.fingerprints
	hostKey := l.hostKey
	l.mu.Unlock()

	host, viewer := "", ""
	if fingerprints != nil {
		host, viewer = fingerprints(peerID)
	}
	if !known {
		sig, err := hostKey.Sign(linkauth.HostUnlinkMessage(m.Nonce, host, viewer))
		if err != nil {
			l.cfg.Logger.Error("cannot sign the unlink refusal", "err", err)
		}
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked, "sig": sig})
		l.dropLater(peerID)
		return
	}
	sig, err := hostKey.Sign(linkauth.HostProofMessage(m.Nonce, host, viewer))
	if err != nil {
		l.cfg.Logger.Error("cannot sign the link proof", "err", err)
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
		l.dropLater(peerID)
		return
	}
	l.reply(peerID, map[string]string{"type": msgProof, "link_id": m.LinkID, "sig": sig})
}

func (l *LinkService) auth(peerID string, m linkMessage) {
	l.mu.Lock()
	linkID, trusted := l.trusted[peerID]
	switch {
	case trusted && linkID == "":
		// Just redeemed a code: remember this browser by the public key it
		// sends now. The private half never leaves the browser.
		if _, err := linkauth.ParsePublic(m.Pub); err != nil {
			l.mu.Unlock()
			l.cfg.Logger.Warn("a linking browser sent no valid key", "peer_id", peerID)
			l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
			l.dropLater(peerID)
			return
		}
		rec, err := newLink(l.cfg.Now(), m.Pub)
		if err != nil {
			l.mu.Unlock()
			l.cfg.Logger.Error("cannot create a link", "err", err)
			return
		}
		if validTerms(m.Terms) {
			rec.Terms, rec.TermsAt = m.Terms, l.cfg.Now()
		}
		l.links = append(l.links, rec)
		l.trusted[peerID] = rec.ID
		links := slices.Clone(l.links)
		l.mu.Unlock()
		l.persist(links)
		l.cfg.Logger.Info("browser remembered", "link_id", rec.ID, "terms", rec.Terms)
		l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": rec.ID, "host_key": l.hostKey.Public()})
		l.authenticated(peerID, rec, true)
		return
	case trusted:
		l.mu.Unlock()
		l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": linkID, "host_key": l.hostKey.Public()})
		return
	}
	i := slices.IndexFunc(l.links, func(r models.Link) bool { return r.ID == m.LinkID })
	nonce := l.nonces[peerID]
	fingerprints := l.fingerprints
	var browserKey string
	if i >= 0 {
		browserKey = l.links[i].BrowserKey
	}
	l.mu.Unlock()

	host, viewer := "", ""
	if fingerprints != nil {
		host, viewer = fingerprints(peerID)
	}
	pub, err := linkauth.ParsePublic(browserKey)
	if i < 0 || nonce == "" || err != nil || !pub.Verify(linkauth.LinkProofMessage(nonce, host, viewer), m.Sig) {
		l.cfg.Logger.Warn("browser failed to authenticate", "peer_id", peerID)
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
		l.dropLater(peerID)
		return
	}

	l.mu.Lock()
	// The link may have moved while we verified without the lock.
	i = slices.IndexFunc(l.links, func(r models.Link) bool { return r.ID == m.LinkID })
	if i < 0 {
		l.mu.Unlock()
		l.reply(peerID, map[string]string{"type": msgAuthFailed, "error": errNotLinked})
		l.dropLater(peerID)
		return
	}
	l.links[i].LastSeen = l.cfg.Now()
	if validTerms(m.Terms) && m.Terms != l.links[i].Terms {
		l.links[i].Terms, l.links[i].TermsAt = m.Terms, l.cfg.Now()
	}
	l.trusted[peerID] = m.LinkID
	delete(l.nonces, peerID)
	if t := l.timers[peerID]; t != nil {
		t.Stop()
		delete(l.timers, peerID)
	}
	links := slices.Clone(l.links)
	rec := l.links[i]
	l.mu.Unlock()
	l.persist(links)
	l.changed()
	l.status.AddPeer(peerID, "", l.cfg.Now())
	l.cfg.Logger.Info("browser back", "peer_id", peerID, "link_id", m.LinkID)
	l.reply(peerID, map[string]string{"type": msgAuthOK, "device_id": l.cfg.DeviceID, "link_id": m.LinkID, "host_key": l.hostKey.Public()})
	l.authenticated(peerID, rec, false)
}

// unlink forgets the browser that asked.
func (l *LinkService) unlink(peerID string) {
	l.mu.Lock()
	linkID, ok := l.trusted[peerID]
	if !ok {
		l.mu.Unlock()
		return
	}
	l.links = slices.DeleteFunc(l.links, func(r models.Link) bool { return r.ID == linkID })
	links := slices.Clone(l.links)
	l.mu.Unlock()
	l.persist(links)
	l.cfg.Logger.Info("browser unlinked itself", "link_id", linkID)
	l.reply(peerID, map[string]string{"type": msgUnlinked})
	l.dropLater(peerID)
}

// UnlinkAll forgets every browser and disconnects the connected ones.
// The window shows the pairing code again.
func (l *LinkService) UnlinkAll() {
	l.mu.Lock()
	peers := make([]string, 0, len(l.trusted))
	for p := range l.trusted {
		peers = append(peers, p)
	}
	l.links = nil
	l.mu.Unlock()
	l.persist(nil)
	for _, p := range peers {
		l.reply(p, map[string]string{"type": msgUnlinked})
		l.dropLater(p)
	}
	l.cfg.Logger.Info("every browser unlinked")
}

func (l *LinkService) persist(links []models.Link) {
	if err := l.cfg.Save(links); err != nil {
		l.cfg.Logger.Warn("cannot save the linked browsers", "err", err)
	}
	l.status.SetSavedLinks(len(links))
}

func (l *LinkService) reply(peerID string, msg any) {
	l.mu.Lock()
	send := l.send
	l.mu.Unlock()
	if b, err := json.Marshal(msg); err == nil && send != nil {
		send(peerID, b)
	}
}

// dropLater forgets the peer now and hangs up a moment later, so the last
// message (auth_failed, unlinked) reaches the browser before the
// connection closes.
func (l *LinkService) dropLater(peerID string) {
	l.Forget(peerID)
	time.AfterFunc(l.cfg.HangUpDelay, func() { l.dropPeer(peerID) })
}

func (l *LinkService) dropPeer(peerID string) {
	l.Forget(peerID)
	l.mu.Lock()
	drop := l.drop
	l.mu.Unlock()
	if drop != nil {
		drop(peerID)
	}
}

// newLink makes the record for a browser just linked, keeping the browser's
// public key and a fresh random id.
func newLink(now time.Time, browserKey string) (models.Link, error) {
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return models.Link{}, err
	}
	return models.Link{ID: hex.EncodeToString(id[:]), BrowserKey: browserKey, CreatedAt: now, LastSeen: now}, nil
}
