// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package instancelock

import (
	"net"
	"os"
	"time"
)

// Linux: a Unix socket next to the lock, in the user's own settings folder
// (mode 0700). The running process accepts on it; a second one connects,
// and that is the wake.

func wakePath(path string) string { return path + ".wake" }

func listen(path string, woken chan<- struct{}) func() {
	sock := wakePath(path)
	os.Remove(sock) // left by a process that ended: the lock is ours now
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		return func() {}
	}
	ln.SetUnlinkOnClose(true)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
			select {
			case woken <- struct{}{}:
			default: // one is waiting already
			}
		}
	}()
	return func() { ln.Close() }
}

// Wake asks the process holding the lock at path to show itself; false
// when none answers.
func Wake(path string) bool {
	c, err := net.DialTimeout("unix", wakePath(path), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
