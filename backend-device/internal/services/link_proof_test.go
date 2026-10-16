// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

// linkVector is the shared cross-language vector. The web, iOS and Android
// clients verify the same signatures against the same keys
// (frontend/packages/shared/test, mobile/.../LinkTests): all sides must agree.
type linkVector struct {
	HostKey           string                              `json:"host_key"`
	HostPub           string                              `json:"host_pub"`
	LinkKey           string                              `json:"link_key"`
	LinkPub           string                              `json:"link_pub"`
	Nonce             string                              `json:"nonce"`
	HostFingerprint   string                              `json:"host_fingerprint"`
	ViewerFingerprint string                              `json:"viewer_fingerprint"`
	HostProofSig      string                              `json:"host_proof_sig"`
	LinkProofSig      string                              `json:"link_proof_sig"`
	HostUnlinkSig     string                              `json:"host_unlink_sig"`
	FingerprintCases  []struct{ SDP, Fingerprint string } `json:"fingerprint_cases"`
}

func readLinkVector(t *testing.T) linkVector {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "vectors", "link-proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v linkVector
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("vectors: %v", err)
	}
	return v
}

// The clients verify the host's and the browser's signatures against the
// public keys in the vector; ECDSA is randomized, so this verifies rather
// than reproduces bytes.
func TestLinkProofVectorsVerify(t *testing.T) {
	v := readLinkVector(t)
	hostPub, err := linkauth.ParsePublic(v.HostPub)
	if err != nil {
		t.Fatal(err)
	}
	linkPub, err := linkauth.ParsePublic(v.LinkPub)
	if err != nil {
		t.Fatal(err)
	}
	if !hostPub.Verify(linkauth.HostProofMessage(v.Nonce, v.HostFingerprint, v.ViewerFingerprint), v.HostProofSig) {
		t.Error("the host proof in the vector did not verify")
	}
	if !linkPub.Verify(linkauth.LinkProofMessage(v.Nonce, v.HostFingerprint, v.ViewerFingerprint), v.LinkProofSig) {
		t.Error("the link proof in the vector did not verify")
	}
	if !hostPub.Verify(linkauth.HostUnlinkMessage(v.Nonce, v.HostFingerprint, v.ViewerFingerprint), v.HostUnlinkSig) {
		t.Error("the host unlink refusal in the vector did not verify")
	}
	// The stored private keys reproduce the public keys in the vector.
	host, err := linkauth.ParseKey(v.HostKey)
	if err != nil || host.Public() != v.HostPub {
		t.Fatalf("host key: %v", err)
	}
	link, err := linkauth.ParseKey(v.LinkKey)
	if err != nil || link.Public() != v.LinkPub {
		t.Fatalf("link key: %v", err)
	}
}

// How a fingerprint is read from an SDP, from the same vectors as the viewers.
func TestFingerprintVectors(t *testing.T) {
	v := readLinkVector(t)
	if len(v.FingerprintCases) == 0 {
		t.Fatal("no fingerprint cases")
	}
	for _, c := range v.FingerprintCases {
		if got := DTLSFingerprint(c.SDP); got != c.Fingerprint {
			t.Errorf("fingerprint %q, want %q", got, c.Fingerprint)
		}
	}
}
