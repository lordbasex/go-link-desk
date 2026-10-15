// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The web computes the same value from the same vectors
// (frontend/packages/shared/test/hmac.test.ts): both sides must agree byte
// for byte.
func TestLinkProofMatchesTheVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "vectors", "link-proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct{ Token, Nonce, Proof string }
	}
	if err := json.Unmarshal(data, &v); err != nil || len(v.Cases) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	for _, c := range v.Cases {
		hash := sha256.Sum256([]byte(c.Token))
		if got := LinkProof(hash[:], c.Nonce); got != c.Proof {
			t.Errorf("proof for %q = %s, want %s", c.Nonce, got, c.Proof)
		}
	}
}

// The proof bound to the connection, and how a fingerprint is read from an
// SDP, from the same vectors as the viewers.
func TestBoundLinkProofMatchesTheVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "vectors", "link-proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		BoundCases []struct {
			Token, Nonce, Proof2 string
			Host                 string `json:"host_fingerprint"`
			Viewer               string `json:"viewer_fingerprint"`
		} `json:"bound_cases"`
		FingerprintCases []struct{ SDP, Fingerprint string } `json:"fingerprint_cases"`
	}
	if err := json.Unmarshal(data, &v); err != nil || len(v.BoundCases) == 0 || len(v.FingerprintCases) == 0 {
		t.Fatalf("vectors: %v", err)
	}
	for _, c := range v.BoundCases {
		hash := sha256.Sum256([]byte(c.Token))
		if got := LinkProofBound(hash[:], c.Nonce, c.Host, c.Viewer); got != c.Proof2 {
			t.Errorf("proof2 = %s, want %s", got, c.Proof2)
		}
	}
	for _, c := range v.FingerprintCases {
		if got := DTLSFingerprint(c.SDP); got != c.Fingerprint {
			t.Errorf("fingerprint %q, want %q", got, c.Fingerprint)
		}
	}
}
