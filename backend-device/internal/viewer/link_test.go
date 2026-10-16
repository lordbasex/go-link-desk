// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package viewer

import (
	"encoding/json"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

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

var linked = &models.RemoteDevice{DeviceID: "dev", LinkID: "link1", Token: "secret-token"}

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

	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "proof": linkProof("secret-token", nonce)})
	out = sentControl(s)
	if len(out) != 1 || out[0]["type"] != "auth" || out[0]["token"] != "secret-token" {
		t.Fatalf("sent %v", out)
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
	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "proof": "forged"})
	if k := s.Status().Kind; k != StatusUntrusted {
		t.Fatalf("status %s", k)
	}
	// The token never left.
	for _, m := range sentControl(s) {
		if m["token"] != nil {
			t.Fatal("token sent to an untrusted computer")
		}
	}
}

func TestTheProofBoundToTheConnection(t *testing.T) {
	bound := *linked
	for _, c := range []struct {
		name      string
		device    *models.RemoteDevice
		certHost  string // the computer's certificate as this viewer sees it
		proofHost string // the one the real computer bound its proof to
		withProof bool
		trusted   bool
	}{
		{"bound to this connection", linked, "sha-256 HO:ST", "sha-256 HO:ST", true, true},
		{"relayed from another connection", linked, "sha-256 MI:TM", "sha-256 HO:ST", true, false},
		{"a computer from before proof2", linked, "sha-256 HO:ST", "", false, true},
		{"the plain proof once bound", func() *models.RemoteDevice { bound.Bound = true; return &bound }(), "sha-256 HO:ST", "", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, rec := linkSession(t, Door{Kind: DoorReach, Device: c.device})
			s.certs = func() (string, string) { return c.certHost, "sha-256 VI:EW" }
			nonce := sentControl(s)[0]["nonce"].(string)
			m := map[string]any{"type": "auth_proof", "link_id": "link1", "proof": linkProof("secret-token", nonce)}
			if c.withProof {
				m["proof2"] = linkProofBound("secret-token", nonce, c.proofHost, "sha-256 VI:EW")
			}
			control(s, m)
			out := sentControl(s)
			if trusted := len(out) == 1 && out[0]["type"] == "auth"; trusted != c.trusted {
				t.Fatalf("sent %v, status %s", out, s.Status().Kind)
			}
			if !c.trusted && s.Status().Kind != StatusUntrusted {
				t.Fatalf("status %s", s.Status().Kind)
			}
			if got := rec.bound; got != (c.trusted && c.withProof) {
				t.Fatalf("bound event %v", got)
			}
		})
	}
}

func TestThePasswordAfterTheToken(t *testing.T) {
	s, _ := linkSession(t, Door{Kind: DoorReach, Device: linked})
	nonce := sentControl(s)[0]["nonce"].(string)
	control(s, map[string]any{"type": "auth_proof", "link_id": "link1", "proof": linkProof("secret-token", nonce)})
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
	if out := sentControl(s); len(out) != 1 || out[0]["type"] != "auth" {
		t.Fatalf("sent %v", out)
	}
	const devID = "9c712c21-d03d-47f0-a148-5ce933f04bb8"
	control(s, map[string]any{"type": "auth_ok", "device_id": devID, "link_id": "l9", "token": "tok"})
	if len(rec.linked) != 1 || rec.linked[0] != (NewLink{DeviceID: devID, LinkID: "l9", Token: "tok"}) {
		t.Fatalf("linked %+v", rec.linked)
	}
}

// A remote that returns a device_id outside the canonical UUID form (e.g.
// one crafted to look like a command-line option) is not linked, so the
// value never reaches the keyring's secret-tool argv.
func TestAClaimRefusesAMalformedDeviceID(t *testing.T) {
	for _, bad := range []string{"--help", "-x", "dev", "9c712c21-d03d-47f0-a148-5ce933f04bb8 extra"} {
		s, rec := linkSession(t, Door{Kind: DoorClaim, Code: "123456789"})
		signalFromHost(s, map[string]any{"kind": "accepted", "permissions": models.Permissions{}})
		control(s, map[string]any{"type": "auth_ok", "device_id": bad, "link_id": "l9", "token": "tok"})
		if len(rec.linked) != 0 {
			t.Fatalf("%q linked: %+v", bad, rec.linked)
		}
	}
}

func TestARevokedLinkIsForgotten(t *testing.T) {
	s, rec := linkSession(t, Door{Kind: DoorReach, Device: linked})
	control(s, map[string]any{"type": "auth_failed"})
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
