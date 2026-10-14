// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package terminal

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// sameUser checks the caller runs as this service's user, or as root, who
// can do anything on the computer already (SO_PEERCRED).
func sameUser(conn *net.UnixConn) bool {
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	ok := false
	_ = raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		ok = err == nil && (int(cred.Uid) == os.Getuid() || cred.Uid == 0)
	})
	return ok
}
