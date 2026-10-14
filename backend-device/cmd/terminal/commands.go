// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/terminal"
)

// command talks to the running service over its socket.
func command(cmd string, args []string) error {
	fs := flags(io.Discard)
	stdin := fs.Bool("stdin", false, "password: read it from the standard input (one line), for a setup script")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := servicePath()
	if err != nil {
		return err
	}
	// The service first: nothing is asked of the person if it is not there.
	conn, err := terminal.Dial(socketPath(path))
	if err != nil {
		return notRunning()
	}
	defer conn.Close()
	req := terminal.Request{Cmd: cmd}
	switch cmd {
	case "unlink":
		if fs.NArg() != 1 {
			return errors.New("which one? go-link-desk-terminal unlink ID (the IDs are in: go-link-desk-terminal status)")
		}
		req.ID = fs.Arg(0)
	case "password":
		if *stdin {
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return errors.New("no password on the standard input")
			}
			req.Password = strings.TrimRight(line, "\r\n")
		} else if req.Password, err = askPassword(); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	r := bufio.NewReader(conn)
	if cmd == "link" {
		return link(conn, r, shellUser(path))
	}
	var rep terminal.Reply
	if err := readReply(r, &rep); err != nil {
		return err
	}
	switch cmd {
	case "status":
		printStatus(*rep.Status)
	case "password":
		fmt.Println("Password set. Link a browser with: go-link-desk-terminal link")
	case "unlink":
		fmt.Println("That browser is no longer linked.")
	}
	return nil
}

func readReply(r *bufio.Reader, rep *terminal.Reply) error {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return errors.New("the service closed the connection")
	}
	*rep = terminal.Reply{}
	if err := json.Unmarshal(line, rep); err != nil {
		return err
	}
	if rep.Error != "" {
		return errors.New(rep.Error)
	}
	return nil
}

// askPassword reads the new password twice, without showing it.
func askPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("run it in a terminal: it asks for the password")
	}
	fmt.Printf("A password for linked browsers (%d to %d characters): ", services.MinPassword, services.MaxPassword)
	a, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Again: ")
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("the two passwords differ")
	}
	return string(a), nil
}

// link shows the pairing code until a browser uses it, and asks here.
func link(conn net.Conn, r *bufio.Reader, shellUser string) error {
	in := bufio.NewReader(os.Stdin)
	intro := false
	for {
		var rep terminal.Reply
		if err := readReply(r, &rep); err != nil {
			return err
		}
		switch {
		case rep.Code != "":
			if !intro {
				intro = true
				fmt.Println("Open desk.go-link.org, choose Link a computer, and type this code (Ctrl+C to stop):")
			}
			fmt.Printf("\n  %s\n\n(it changes every few minutes)\n", services.FormatCode(rep.Code))
		case rep.Ask != nil:
			who := rep.Ask.Browser
			if rep.Ask.Name != "" {
				who = fmt.Sprintf("%s (%s)", rep.Ask.Name, rep.Ask.Browser)
			}
			fmt.Printf("%s wants to be linked: it will open a shell as %s whenever it comes back with the password.\n", who, shellUser)
			fmt.Print("Name and browser are what it says; accept only if it is you. Accept? [y/N] ")
			line, _ := in.ReadString('\n')
			yes := strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes") ||
				strings.EqualFold(strings.TrimSpace(line), "s") || strings.EqualFold(strings.TrimSpace(line), "si") || strings.EqualFold(strings.TrimSpace(line), "sí")
			if err := json.NewEncoder(conn).Encode(terminal.Answer{Answer: yes}); err != nil {
				return err
			}
			if !yes {
				fmt.Println("Turned down. Waiting for another browser…")
			}
		case rep.Linked != "":
			fmt.Printf("Linked: %s. It can come back from its list of computers, with the password.\n", rep.Linked)
			return nil
		}
	}
}

func printStatus(st terminal.Status) {
	fmt.Printf("go-link-desk-terminal %s on %s\n", st.Version, st.Name)
	fmt.Printf("  signaling: %s\n", st.Signal)
	if st.User != "" {
		fmt.Printf("  sessions:  a shell as %s\n", st.User)
	}
	if st.Unattended {
		fmt.Println("  password:  set")
	} else {
		fmt.Println("  password:  not set (go-link-desk-terminal password): nobody can come in")
	}
	if st.Session != "" {
		fmt.Printf("  in session: %s\n", st.Session)
	}
	if len(st.Links) == 0 {
		fmt.Println("  linked browsers: none (go-link-desk-terminal link)")
		return
	}
	fmt.Println("  linked browsers:")
	for _, l := range st.Links {
		fmt.Printf("    %s  %s  (linked %s, last %s)\n", l.ID, l.Name, l.Created.Local().Format("2006-01-02"), l.LastSeen.Local().Format(time.DateTime))
	}
}

// servicePath is the device.json of the service to talk to: --config, this
// user's, or, run as root, the one user's whose service is running (so
// "sudo go-link-desk-terminal link" just works).
func servicePath() (string, error) {
	if sf.config != "" || os.Geteuid() != 0 {
		return configPath(sf.config)
	}
	socks, _ := filepath.Glob("/home/*/.config/" + folder + "/control.sock")
	var found []string
	for _, s := range socks {
		if conn, err := terminal.Dial(s); err == nil {
			conn.Close()
			found = append(found, s)
		}
	}
	if sf.user != "" {
		for _, s := range found {
			if strings.Split(s, "/")[2] == sf.user {
				return filepath.Join(filepath.Dir(s), "device.json"), nil
			}
		}
		return "", fmt.Errorf("go-link-desk-terminal is not running for %s", sf.user)
	}
	switch len(found) {
	case 0:
		return "", errors.New("go-link-desk-terminal runs as a user, never root, and it is not running for anyone here: as the user whose shell sessions get, run: systemctl --user enable --now go-link-desk-terminal")
	case 1:
		return filepath.Join(filepath.Dir(found[0]), "device.json"), nil
	}
	var users []string
	for _, s := range found {
		users = append(users, strings.Split(s, "/")[2])
	}
	return "", fmt.Errorf("it runs for %s: choose one with --user", strings.Join(users, ", "))
}

// shellUser is whose shell sessions get, as the service says.
func shellUser(path string) string {
	conn, err := terminal.Dial(socketPath(path))
	if err == nil {
		defer conn.Close()
		var rep terminal.Reply
		if json.NewEncoder(conn).Encode(terminal.Request{Cmd: "status"}) == nil && readReply(bufio.NewReader(conn), &rep) == nil && rep.Status != nil && rep.Status.User != "" {
			return rep.Status.User
		}
	}
	return os.Getenv("USER")
}

func notRunning() error {
	if os.Geteuid() == 0 {
		return errors.New("go-link-desk-terminal runs as a user, never root: run this as that user (su - USER)")
	}
	return errors.New("go-link-desk-terminal is not running: systemctl --user enable --now go-link-desk-terminal")
}
