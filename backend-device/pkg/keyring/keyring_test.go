// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keyring

import (
	"errors"
	"os"
	"testing"
)

// The system's store, for real: only where asked (it may show a prompt).
func TestKeepsGetsAndForgetsASecret(t *testing.T) {
	if os.Getenv("KEYRING_TEST") == "" {
		t.Skip("set KEYRING_TEST=1 to use this system's secret store")
	}
	const service, account = "go-link-desk-test", "a-device"
	if err := Set(service, account, "first"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Delete(service, account) })
	if err := Set(service, account, "sécret ✓"); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(service, account); err != nil || got != "sécret ✓" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := Delete(service, account); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(service, account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := Delete(service, account); err != nil {
		t.Fatalf("delete twice: %v", err)
	}
}
