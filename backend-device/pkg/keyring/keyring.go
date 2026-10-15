// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package keyring keeps secrets in the system's own store: the Keychain on
// macOS, the Credential Manager on Windows, the Secret Service (secret-tool)
// on Linux. A secret is found by its service and account.
package keyring

import "errors"

var (
	// ErrNotFound: nothing is kept for that service and account.
	ErrNotFound = errors.New("keyring: not found")
	// ErrUnavailable: this system has no store this package can use.
	ErrUnavailable = errors.New("keyring: no secret store on this system")
)

// Set keeps secret for service and account, in place of what was there.
func Set(service, account, secret string) error { return set(service, account, secret) }

// Get is what is kept for service and account (ErrNotFound if nothing).
func Get(service, account string) (string, error) { return get(service, account) }

// Delete forgets it; nothing kept is not an error.
func Delete(service, account string) error {
	if err := del(service, account); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}
