// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

package keyring

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Linux keeps them in the Secret Service (GNOME Keyring, KWallet) through
// secret-tool (libsecret-tools); the secret goes by its standard input,
// never its arguments.

func tool() (string, error) {
	path, err := exec.LookPath("secret-tool")
	if err != nil {
		return "", fmt.Errorf("%w: install secret-tool (libsecret-tools)", ErrUnavailable)
	}
	return path, nil
}

func run(stdin string, args ...string) (string, error) {
	path, err := tool()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.TrimSpace(errOut.String()) == "" {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keyring: secret-tool: %v %s", err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// The "--" ends secret-tool's option parsing, so an attribute value that
// begins with "-" (a remote-supplied device_id like "--help") is read as a
// value, never as an option.
func set(service, account, secret string) error {
	_, err := run(secret, "store", "--label=go-link Desk ("+account+")", "--", "service", service, "account", account)
	return err
}

func get(service, account string) (string, error) {
	return run("", "lookup", "--", "service", service, "account", account)
}

func del(service, account string) error {
	_, err := run("", "clear", "--", "service", service, "account", account)
	return err
}
