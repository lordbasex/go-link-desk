// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && lordos

package awake

import "github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"

// On LordOS the session stays awake through the share: the desktops' D-Bus
// services aren't there. The client sends it again once a share runs.
func hold() bool {
	c := lordshare.Shared()
	if c == nil {
		return false
	}
	c.Awake(true)
	return true
}

func release() {
	if c := lordshare.Shared(); c != nil {
		c.Awake(false)
	}
}
