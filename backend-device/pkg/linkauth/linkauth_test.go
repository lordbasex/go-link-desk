// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package linkauth

import "testing"

const (
	nonce  = "nonce-0123456789abcdef"
	hostFp = "sha-256 EF:A2:35:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:EE"
	viewFp = "sha-256 10:20:30:40:50:60:70:80:90:A0:B0:C0:D0:E0:F0:01:02:03:04:05:06:07:08:09:0A:0B:0C:0D:0E:0F:10:11"
)

func TestAHostKeyRoundTripsThroughItsStoredForm(t *testing.T) {
	h, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseKey(h.Private())
	if err != nil {
		t.Fatal(err)
	}
	if got.Private() != h.Private() || got.Public() != h.Public() {
		t.Fatal("a device key did not survive its stored form")
	}
}

func TestEachSideVerifiesTheOtherSignature(t *testing.T) {
	host, _ := NewKey()
	link, _ := NewKey() // a browser's link key is the same kind of key

	hostPub, err := ParsePublic(host.Public())
	if err != nil {
		t.Fatal(err)
	}
	linkPub, err := ParsePublic(link.Public())
	if err != nil {
		t.Fatal(err)
	}

	hostSig, err := host.Sign(HostProofMessage(nonce, hostFp, viewFp))
	if err != nil {
		t.Fatal(err)
	}
	if !hostPub.Verify(HostProofMessage(nonce, hostFp, viewFp), hostSig) {
		t.Fatal("the host proof did not verify")
	}
	linkSig, err := link.Sign(LinkProofMessage(nonce, hostFp, viewFp))
	if err != nil {
		t.Fatal(err)
	}
	if !linkPub.Verify(LinkProofMessage(nonce, hostFp, viewFp), linkSig) {
		t.Fatal("the link proof did not verify")
	}
	// A host proof must never read as a link proof, or verify for the wrong
	// nonce/fingerprints, or against the other key.
	if hostPub.Verify(LinkProofMessage(nonce, hostFp, viewFp), hostSig) {
		t.Fatal("a host proof read as a link proof")
	}
	if hostPub.Verify(HostProofMessage("other", hostFp, viewFp), hostSig) {
		t.Fatal("a host proof verified for the wrong nonce")
	}
	if hostPub.Verify(HostProofMessage(nonce, viewFp, hostFp), hostSig) {
		t.Fatal("a host proof verified with the fingerprints swapped")
	}
	if linkPub.Verify(HostProofMessage(nonce, hostFp, viewFp), hostSig) {
		t.Fatal("the host proof verified under the link key")
	}
}

func TestVerifyRefusesMalformedInput(t *testing.T) {
	host, _ := NewKey()
	pub, _ := ParsePublic(host.Public())
	msg := HostProofMessage(nonce, hostFp, viewFp)
	for _, bad := range []string{"", "not base64url !!!", "AAAA", host.Public()} {
		if pub.Verify(msg, bad) {
			t.Fatalf("a malformed signature verified: %q", bad)
		}
	}
	if (PublicKey{}).Verify(msg, "AAAA") {
		t.Fatal("the zero key verified something")
	}
	for _, bad := range []string{"", "nope !!!", "BBBB"} {
		if _, err := ParsePublic(bad); err == nil {
			t.Fatalf("a malformed public key parsed: %q", bad)
		}
	}
	for _, bad := range []string{"", "nope !!!", "CCCC"} {
		if _, err := ParseKey(bad); err == nil {
			t.Fatalf("a malformed device key parsed: %q", bad)
		}
	}
}
