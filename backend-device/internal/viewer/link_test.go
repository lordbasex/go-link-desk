// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"encoding/json"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

func mustKey() linkauth.Key {
	k, err := linkauth.NewKey()
	if err != nil {
		panic(err)
	}
	return k
}

// hostKey stands for the computer's device key; viewerKey for this app's
// link key. The pinned link points at both.
var hostKey = mustKey()
var viewerKey = mustKey()
var linked = &models.RemoteDevice{DeviceID: "dev", LinkID: "link1", Key: viewerKey.Private(), HostKey: hostKey.Public()}

func hostProof(nonce, host, viewer string) string {
	s, err := hostKey.Sign(linkauth.HostProofMessage(nonce, host, viewer))
	if err != nil {
		panic(err)
	}
	return s
}

// linkSession is a session at its control messages, without WebRTC: what
// it sends waits in pendingControl.
func linkSession(t *testing.T, door Door) (*Session, *recorder) {
	t.Helper()
	rec := &recorder{}
	s := New(Options{Door: door, Logger: quiet, Events: rec.events()})
	s.Start(nopTransport{})
	s.Connected(nil)
	s.Handle(signalclient.Envelope{Type: signalclient.TypePaired, Remote: hostID})
	return s, rec
}

func sentControl(s *Session) []map[string]any {
	var out []map[string]any
	for _, b := range s.takePending() {
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out
}

func control(s *Session, v any) {
	b, _ := json.Marshal(v)
	s.onControl(b)
}

func signalFromHost(s *Session, v any) {
	b, _ := json.Marshal(v)
	s.Handle(signalclient.Envelope{Type: signalclient.TypeSignal, From: hostID, Payload: b})
}

func TestAReachingSessionTrustsTheComputerOnlyAfterItsProof(t *testing.T) {
	s, rec := linkSession(t, Door{Kind: DoorReach, Device: linked})
	out := sentControl(s)
	if len(out) != 1 || out[0]["type"] != "auth_challenge" || out[0]["link_id"] != "link1" {
		t.Fatalf("sent %v", out)
	}
	nonce := out[0]["nonce"].(string)

	// Nothing the computer says counts before its proof and auth_ok.
	control(s, map[string]any{"type": "password_required"})
	signalFromHost(s, map[string]any{"kind": "accepted", "permissions": models.Permissions{Control: true}})
	if k := s.Status().Kind; k != StatusWaiting {
		t.Fatalf("status %s before the proof", k)
	}

	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "sig": hostProof(nonce, "", "")})
	out = sentControl(s)
	if len(out) != 1 || out[0]["type"] != "auth" || out[0]["sig"] == nil || out[0]["sig"] == "" {
		t.Fatalf("sent %v", out)
	}
	// The signature the viewer sent proves its own link key over the same
	// nonce and fingerprints.
	vpub, _ := linkauth.ParsePublic(viewerKey.Public())
	if !vpub.Verify(linkauth.LinkProofMessage(nonce, "", ""), out[0]["sig"].(string)) {
		t.Fatal("the viewer's proof did not verify")
	}
	// The acceptance that came early is applied with auth_ok.
	control(s, map[string]any{"type": "auth_ok", "link_id": "link1"})
	if k := s.Status().Kind; k != StatusConnecting {
		t.Fatalf("status %s after auth_ok", k)
	}
	if !s.Permissions().Control {
		t.Fatal("the early permissions were lost")
	}
	_ = rec
}

func TestAWrongProofIsUntrusted(t *testing.T) {
	s, _ := linkSession(t, Door{Kind: DoorReach, Device: linked})
	sentControl(s)
	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "sig": "forged"})
	if k := s.Status().Kind; k != StatusUntrusted {
		t.Fatalf("status %s", k)
	}
	// Nothing left for an untrusted computer.
	if out := sentControl(s); len(out) != 0 {
		t.Fatalf("sent to an untrusted computer: %v", out)
	}
}

// The proof is bound to the connection's fingerprints: a proof the real
// computer signed for its own connection does not verify on a connection a
// signaling server sits in the middle of, because the viewer sees that
// server's certificate instead.
func TestTheProofBoundToTheConnection(t *testing.T) {
	for _, c := range []struct {
		name     string
		certHost string // the computer's certificate as this viewer sees it
		signHost string // the fingerprint the real computer signed over
		trusted  bool
	}{
		{"bound to this connection", "sha-256 HO:ST", "sha-256 HO:ST", true},
		{"relayed from another connection", "sha-256 MI:TM", "sha-256 HO:ST", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := linkSession(t, Door{Kind: DoorReach, Device: linked})
			s.certs = func() (string, string) { return c.certHost, "sha-256 VI:EW" }
			nonce := sentControl(s)[0]["nonce"].(string)
			control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "sig": hostProof(nonce, c.signHost, "sha-256 VI:EW")})
			out := sentControl(s)
			if trusted := len(out) == 1 && out[0]["type"] == "auth"; trusted != c.trusted {
				t.Fatalf("sent %v, status %s", out, s.Status().Kind)
			}
			if !c.trusted && s.Status().Kind != StatusUntrusted {
				t.Fatalf("status %s", s.Status().Kind)
			}
		})
	}
}

func TestThePasswordAfterTheToken(t *testing.T) {
	s, _ := linkSession(t, Door{Kind: DoorReach, Device: linked})
	nonce := sentControl(s)[0]["nonce"].(string)
	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "sig": hostProof(nonce, "", "")})
	control(s, map[string]any{"type": "auth_ok", "link_id": "link1"})
	sentControl(s)

	control(s, map[string]any{"type": "password_required"})
	if k := s.Status().Kind; k != StatusPassword {
		t.Fatalf("status %s", k)
	}
	s.SendPassword("wrong one")
	if !s.Status().Checking {
		t.Fatal("not checking")
	}
	if out := sentControl(s); out[0]["type"] != "password" || out[0]["password"] != "wrong one" {
		t.Fatalf("sent %v", out)
	}
	control(s, map[string]any{"type": "password_result", "ok": false, "reason": "wrong", "left": 4})
	if st := s.Status(); st.Kind != StatusPassword || st.PasswordError != "wrong" || st.Left != 4 || st.Checking {
		t.Fatalf("%+v", st)
	}
	control(s, map[string]any{"type": "password_result", "ok": false, "reason": "locked", "retry_after": 600})
	if st := s.Status(); st.Kind != StatusPasswordRefused || st.RetryAfter != 600 {
		t.Fatalf("%+v", st)
	}
}

func TestAClaimKeepsItsLink(t *testing.T) {
	s, rec := linkSession(t, Door{Kind: DoorClaim, Code: "123456789"})
	signalFromHost(s, map[string]any{"kind": "accepted", "permissions": models.Permissions{}})
	out := sentControl(s)
	if len(out) != 1 || out[0]["type"] != "auth" || out[0]["pub"] == nil {
		t.Fatalf("sent %v", out)
	}
	const devID = "9c712c21-d03d-47f0-a148-5ce933f04bb8"
	control(s, map[string]any{"type": "auth_ok", "device_id": devID, "link_id": "l9", "host_key": hostKey.Public()})
	if len(rec.linked) != 1 {
		t.Fatalf("linked %+v", rec.linked)
	}
	got := rec.linked[0]
	if got.DeviceID != devID || got.LinkID != "l9" || got.HostKey != hostKey.Public() {
		t.Fatalf("linked %+v", got)
	}
	// The viewer kept its own private key, matching the public one it sent.
	key, err := linkauth.ParseKey(got.Key)
	if err != nil || key.Public() != out[0]["pub"].(string) {
		t.Fatalf("link key: %v", err)
	}
}

// A remote that returns a device_id outside the canonical UUID form (e.g.
// one crafted to look like a command-line option), or no host key, is not
// linked, so nothing bad reaches the keyring or the pin.
func TestAClaimRefusesAMalformedDeviceID(t *testing.T) {
	for _, bad := range []string{"--help", "-x", "dev", "9c712c21-d03d-47f0-a148-5ce933f04bb8 extra"} {
		s, rec := linkSession(t, Door{Kind: DoorClaim, Code: "123456789"})
		signalFromHost(s, map[string]any{"kind": "accepted", "permissions": models.Permissions{}})
		control(s, map[string]any{"type": "auth_ok", "device_id": bad, "link_id": "l9", "host_key": hostKey.Public()})
		if len(rec.linked) != 0 {
			t.Fatalf("%q linked: %+v", bad, rec.linked)
		}
	}
	// A valid id but no host key is also refused: the viewer has nothing to pin.
	s, rec := linkSession(t, Door{Kind: DoorClaim, Code: "123456789"})
	signalFromHost(s, map[string]any{"kind": "accepted", "permissions": models.Permissions{}})
	control(s, map[string]any{"type": "auth_ok", "device_id": "9c712c21-d03d-47f0-a148-5ce933f04bb8", "link_id": "l9"})
	if len(rec.linked) != 0 {
		t.Fatalf("linked with no host key: %+v", rec.linked)
	}
}

func TestARevokedLinkIsForgottenOnlyWhenSigned(t *testing.T) {
	// Before any proof, an unsigned refusal is ignored: a signaling server
	// could forge it to wipe a working link.
	s, rec := linkSession(t, Door{Kind: DoorReach, Device: linked})
	nonce := sentControl(s)[0]["nonce"].(string)
	control(s, map[string]any{"type": "auth_failed"})
	if s.Status().Kind == StatusUnlinked || rec.unlinked {
		t.Fatal("an unsigned refusal wiped the link")
	}
	// A refusal the computer signed with its device key is honored: this is
	// how a genuinely revoked link comes back.
	sig, _ := hostKey.Sign(linkauth.HostUnlinkMessage(nonce, "", ""))
	control(s, map[string]any{"type": "auth_failed", "sig": sig})
	if s.Status().Kind != StatusUnlinked || !rec.unlinked {
		t.Fatalf("%+v %v", s.Status(), rec.unlinked)
	}
}

func TestAJoinIgnoresLinkMessages(t *testing.T) {
	s, rec := linkSession(t, Door{Kind: DoorJoin, Code: "1"})
	control(s, map[string]any{"type": "unlinked"})
	control(s, map[string]any{"type": "password_required"})
	if s.Status().Kind != StatusWaiting || rec.unlinked {
		t.Fatalf("%+v", s.Status())
	}
}

func TestClipboardPiecesComeBackWhole(t *testing.T) {
	var c clipboardAssembler
	if _, ok := c.add(1, 0, 2, "ho"); ok {
		t.Fatal("whole after one piece")
	}
	if text, ok := c.add(1, 1, 2, "la"); !ok || text != "hola" {
		t.Fatalf("%q %v", text, ok)
	}
	c.add(2, 0, 3, "a")
	if _, ok := c.add(2, 2, 3, "c"); ok {
		t.Fatal("a piece out of order was taken")
	}
}
