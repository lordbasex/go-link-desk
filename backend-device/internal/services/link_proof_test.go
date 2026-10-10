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
