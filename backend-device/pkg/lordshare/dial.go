// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build unix

package lordshare

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
)

// InheritedFD is the descriptor LordOS connects for a companion program
// (GO-LINK.md 3.2): the share connection, already tied to the app.
const InheritedFD = 4

var shared atomic.Pointer[Client]

// Shared is the program's connection, nil until Connect.
func Shared() *Client { return shared.Load() }

// SetShared sets the program's connection.
func SetShared(c *Client) { shared.Store(c) }

// Connect makes the program's connection: the inherited descriptor, or for
// an app the launcher started, the session's socket.
func Connect() (*Client, error) {
	conn, err := inherited()
	if err != nil {
		conn, err = dialSession()
	}
	if err != nil {
		return nil, err
	}
	c := New(conn)
	SetShared(c)
	return c, nil
}

func inherited() (net.Conn, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(InheritedFD, &st); err != nil {
		return nil, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return nil, errors.New("lordshare: fd 4 is not a socket")
	}
	f := os.NewFile(InheritedFD, "share")
	defer f.Close()
	return net.FileConn(f)
}

func dialSession() (net.Conn, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return nil, errors.New("lordshare: no XDG_RUNTIME_DIR")
	}
	return net.Dial("unix", filepath.Join(dir, "lordos", "share.sock"))
}
