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
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/terminal"
)

// command talks to the running service over its socket.
func command(cmd string, args []string) error {
	fs := flags(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := configPath(sf.config)
	if err != nil {
		return err
	}
	req := terminal.Request{Cmd: cmd}
	switch cmd {
	case "unlink":
		if fs.NArg() != 1 {
			return errors.New("which one? go-link-desk-terminal unlink ID (the IDs are in: go-link-desk-terminal status)")
		}
		req.ID = fs.Arg(0)
	case "password":
		if req.Password, err = askPassword(); err != nil {
			return err
		}
	}
	conn, err := terminal.Dial(socketPath(path))
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	r := bufio.NewReader(conn)
	if cmd == "link" {
		return link(conn, r)
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
func link(conn net.Conn, r *bufio.Reader) error {
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
			fmt.Printf("%s wants to be linked: it will open a shell as %s whenever it comes back with the password.\n", who, os.Getenv("USER"))
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
