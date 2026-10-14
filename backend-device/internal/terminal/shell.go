// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

// Package terminal is go-link-desk-terminal's: a login shell on a
// pseudo-terminal of its own (/dev/pts/N, not SSH) for each viewer in
// session, carried on the session's "terminal" DataChannel.
package terminal

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ShellConfig is the shell a session gets: the user's own login shell, in
// their home folder, with a clean environment.
type ShellConfig struct {
	Path string // /bin/bash
	Home string
	User string
	// Env is the environment: HOME, USER, LOGNAME, SHELL, PATH, TERM and
	// the language, nothing else of the service's.
	Env []string
}

// LoginShell is this user's shell, from the user database (/etc/passwd),
// else $SHELL, else /bin/sh, in $HOME (else the database's home).
func LoginShell() ShellConfig {
	c := ShellConfig{Path: "/bin/sh", Home: "/"}
	if u, err := user.Current(); err == nil {
		c.User, c.Home = u.Username, u.HomeDir
	}
	if h := os.Getenv("HOME"); h != "" {
		c.Home = h // systemd's, or a test's own
	}
	if sh := passwdShell(os.Getuid()); sh != "" {
		c.Path = sh
	} else if sh := os.Getenv("SHELL"); sh != "" {
		c.Path = sh
	}
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	c.Env = []string{
		"HOME=" + c.Home, "USER=" + c.User, "LOGNAME=" + c.User, "SHELL=" + c.Path,
		"PATH=" + path, "TERM=xterm-256color", "COLORTERM=truecolor",
	}
	for _, k := range []string{"LANG", "LC_ALL", "TZ"} {
		if v := os.Getenv(k); v != "" {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	if os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" {
		c.Env = append(c.Env, "LANG=C.UTF-8") // UTF-8 for the browser's terminal
	}
	return c
}

// passwdShell reads uid's shell from /etc/passwd ("" if it is not there).
func passwdShell(uid int) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return ""
	}
	defer f.Close()
	return shellOf(bufio.NewScanner(f), uid)
}

func shellOf(sc *bufio.Scanner, uid int) string {
	want := strconv.Itoa(uid)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) == 7 && f[2] == want && strings.HasPrefix(f[6], "/") {
			return f[6]
		}
	}
	return ""
}

// Shell is a login shell running on a pseudo-terminal of its own.
type Shell struct {
	cmd  *exec.Cmd
	pty  *os.File
	done chan struct{}
	code int

	closeOnce sync.Once
	mu        sync.RWMutex // Close against Write and Resize
	closed    bool
}

// StartShell starts c's shell as a login shell ("-bash") on a new
// pseudo-terminal of cols × rows, in its own session.
func StartShell(c ShellConfig, cols, rows uint16) (*Shell, error) {
	if c.Path == "" {
		return nil, errors.New("terminal: no shell")
	}
	cmd := exec.Command(c.Path)
	cmd.Args = []string{"-" + filepath.Base(c.Path)} // a login shell, as login(1) starts it
	cmd.Env = c.Env
	cmd.Dir = c.Home
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	s := &Shell{cmd: cmd, pty: f, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		s.code = cmd.ProcessState.ExitCode()
		if err != nil && s.code == 0 {
			s.code = -1
		}
		close(s.done)
	}()
	return s, nil
}

// Read reads what the shell writes.
func (s *Shell) Read(p []byte) (int, error) { return s.pty.Read(p) }

// Write types into the shell.
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, os.ErrClosed
	}
	return s.pty.Write(p)
}

// Resize tells the shell the viewer's terminal size (it gets SIGWINCH).
func (s *Shell) Resize(cols, rows uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return os.ErrClosed
	}
	return pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

// Done is closed when the shell has ended.
func (s *Shell) Done() <-chan struct{} { return s.done }

// ExitCode is the shell's, once Done.
func (s *Shell) ExitCode() int { return s.code }

// Close hangs up: SIGHUP to the shell's whole session (what closing a
// terminal does), then SIGKILL to what is left after two seconds.
func (s *Shell) Close() {
	s.closeOnce.Do(func() {
		pgid := s.cmd.Process.Pid // Setsid: the shell leads its own group
		_ = syscall.Kill(-pgid, syscall.SIGHUP)
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-s.done
		}
		s.mu.Lock()
		s.closed = true
		_ = s.pty.Close()
		s.mu.Unlock()
	})
}
