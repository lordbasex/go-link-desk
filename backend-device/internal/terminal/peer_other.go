// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !linux && !windows

package terminal

import "net"

// sameUser: elsewhere (a developer's Mac) the socket's 0600 and its
// folder's 0700 keep other users out.
func sameUser(*net.UnixConn) bool { return true }
