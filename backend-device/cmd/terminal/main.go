// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

// Command go-link-desk-terminal is go-link Desk for a computer without a
// desktop (a Raspberry Pi, a server): instead of the screen, each session
// gets a shell of its own on a new pseudo-terminal, in the browser. It is
// pure Go (CGO_ENABLED=0, -tags headless), with signalhub, the codes, the
// relay and unattended access of the app.
//
//	go-link-desk-terminal [serve]   run (systemd starts it)
//	go-link-desk-terminal password  set the password linked browsers give
//	go-link-desk-terminal link      show the pairing code, and accept a browser
//	go-link-desk-terminal status    what it is doing, and the linked browsers
//	go-link-desk-terminal unlink ID forget a linked browser
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/controllers"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/terminal"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// version is set at build time (-ldflags "-X main.version=v1.4.0").
var version = "dev"

// folder is where it keeps device.json, apart from the desktop app's:
// an identity, links and a password of its own.
const folder = "go-link-desk-terminal"

const usage = `go-link-desk-terminal: a remote terminal with go-link Desk, for a computer without a desktop.

Usage:
  go-link-desk-terminal [serve] [flags]  run the service (systemd starts it)
  go-link-desk-terminal password         set the password linked browsers must give (--stdin: from a script)
  go-link-desk-terminal link             show a pairing code, and accept the browser that uses it
  go-link-desk-terminal status           show the service and its linked browsers
  go-link-desk-terminal unlink ID        forget a linked browser (--all: every one)
  go-link-desk-terminal reset            back to zero: every linked browser and the password forgotten
  go-link-desk-terminal version

Flags (serve):
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 {
		switch a := args[0]; {
		case !strings.HasPrefix(a, "-"), a == "-v", a == "-version", a == "--version", a == "-h", a == "--help":
			cmd, args = a, args[1:]
		}
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "password", "link", "status", "unlink", "reset":
		err = command(cmd, args)
	case "version", "-v", "-version", "--version":
		fmt.Println("go-link-desk-terminal", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
		flags(os.Stdout).PrintDefaults()
	default:
		err = fmt.Errorf("unknown command %q (go-link-desk-terminal help)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-link-desk-terminal:", err)
		os.Exit(1)
	}
}

type serveFlags struct {
	config, signal, name, shell, user string
	debug, allowRoot                  bool
}

var sf serveFlags

func flags(out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&sf.config, "config", "", "device.json (default ~/.config/"+folder+"/device.json)")
	fs.StringVar(&sf.signal, "server-signaling", "", "the signaling server (default: device.json's, else go-link Desk's)")
	fs.StringVar(&sf.name, "name", "", "the name viewers see (default: the host name)")
	fs.StringVar(&sf.shell, "shell", "", "the shell sessions get (default: the user's login shell)")
	fs.StringVar(&sf.user, "user", "", "commands run as root: the user whose service to use (when several run it)")
	fs.BoolVar(&sf.debug, "debug", false, "log more")
	fs.BoolVar(&sf.allowRoot, "allow-root", false, "let it run as root: every session would be a root shell")
	return fs
}

func configPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	return repositories.ConfigPathFor(folder)
}

// socketPath is the commands' socket, next to device.json.
func socketPath(config string) string { return filepath.Join(filepath.Dir(config), "control.sock") }

func serve(args []string) error {
	if err := flags(os.Stderr).Parse(args); err != nil {
		return err
	}
	if os.Geteuid() == 0 && !sf.allowRoot {
		return errors.New("refusing to run as root: every session would be a root shell; run it as a user (or pass --allow-root)")
	}
	level := slog.LevelInfo
	if sf.debug {
		level = slog.LevelDebug
	}
	// systemd's journal keeps the log (journalctl --user -u go-link-desk-terminal).
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	path, err := configPath(sf.config)
	if err != nil {
		return err
	}
	store := repositories.NewConfigFile(path)
	cfg, created, err := store.LoadOrCreate()
	if err != nil {
		return err
	}
	lock, err := instancelock.Acquire(filepath.Join(filepath.Dir(path), "terminal.lock"))
	if err != nil {
		return fmt.Errorf("cannot start: %w", err)
	}
	defer lock.Release()
	target := cfg.EffectiveSignalURL()
	if sf.signal != "" {
		target = sf.signal
	}
	if _, err := signalclient.CheckURL(target); err != nil {
		return fmt.Errorf("invalid signaling server: %w", err)
	}
	shell := terminal.LoginShell()
	if sf.shell != "" {
		shell.Path = sf.shell
		for i, kv := range shell.Env {
			if strings.HasPrefix(kv, "SHELL=") {
				shell.Env[i] = "SHELL=" + sf.shell
			}
		}
	}
	logger.Info("terminal host starting", "version", version, "device_id", cfg.DeviceID, "config", path, "first_run", created,
		"signaling", target, "shell", shell.Path, "user", shell.User)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	settings := services.NewSettingsService(cfg, store.Save, logger)
	status := services.NewStatusService(cfg.DeviceID, version, target)
	status.SetSavedLinks(len(cfg.Links))
	links := services.NewLinkService(services.LinkConfig{
		DeviceID: cfg.DeviceID, Links: cfg.Links, Logger: logger,
		Save: func(l []models.Link) error {
			return settings.Update(func(c *models.Config) { c.Links = l })
		},
	}, status)
	ice := services.NewICEStore()
	pairing := services.NewPairingService(services.PairingConfig{DeviceID: cfg.DeviceID, DeviceSecret: cfg.DeviceSecret, Logger: logger}, status, ice)
	opener := services.NewRoomOpener()
	meta := services.SessionMeta{Name: hostName(), OS: runtime.GOOS}
	if sf.name != "" {
		meta.Name = sf.name
	}
	api, err := services.NewWebRTCAPI(cfg.UDPPort, false)
	if err != nil {
		return err
	}
	// The fixed port, opened in the router when asked (Settings of the app,
	// or device.json's map_udp_port).
	var outside func() (netip.Addr, int)
	if cfg.UDPPort > 0 && cfg.MapUDPPort {
		ports := &services.PortMapper{Port: cfg.UDPPort, Log: logger}
		outside = ports.Outside
		go func() { _ = ports.Run(ctx) }()
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, UDPPort: cfg.UDPPort, AnnounceIPs: cfg.AnnounceIPs, Outside: outside,
		Welcome: meta, Source: "terminal", DataOnly: true, Channels: []string{terminal.Channel}, Features: []string{"terminal-tabs"}, Logger: logger,
	}, ice)
	if err != nil {
		return err
	}
	session := services.NewSessionService(opener, status, meta, stream, logger)
	ctl := &terminal.Control{
		Path: socketPath(path), Log: logger,
		Status: func() terminal.Status {
			st := status.Snapshot()
			out := terminal.Status{Version: version, DeviceID: cfg.DeviceID, Signal: st.Signal.State, Name: meta.Name, User: shell.User, WebURL: cfg.EffectiveWebURL(), Unattended: settings.UnattendedOn()}
			if st.Session != nil && st.Session.Active != nil {
				out.Session = st.Session.Active.Name
			}
			for _, l := range links.Links() {
				out.Links = append(out.Links, terminal.LinkInfo{ID: l.ID, Name: l.Name, Created: l.CreatedAt, LastSeen: l.LastSeen})
			}
			return out
		},
		Code: func() string { return strings.ReplaceAll(status.Snapshot().Pairing.Code, " ", "") },
		SetPassword: func(pw string) error {
			hash, err := services.HashPassword(pw)
			if err != nil {
				return err
			}
			return settings.Update(func(c *models.Config) { c.Unattended = models.Unattended{Enabled: true, PasswordHash: hash} })
		},
		Unlink: func(id string) bool {
			if id == "*" {
				links.UnlinkAll()
				return true
			}
			for _, l := range links.Links() {
				if l.ID == id {
					links.Revoke(id)
					return true
				}
			}
			return false
		},
		Reset: func() error {
			links.UnlinkAll()
			session.End(models.EndRevoked)
			return settings.Update(func(c *models.Config) { c.Unattended = models.Unattended{} })
		},
	}
	session.SetConsent(ctl)
	session.SetUnattended(settings)
	unattended := services.NewUnattendedService(services.UnattendedConfig{
		Links: links, Session: session, Settings: settings, Send: stream.SendControl, Logger: logger,
	})
	links.SetTransport(stream.SendControl, func(peer string) { session.Drop(peer, models.EndRevoked) })
	pairing.SetLinks(session)
	pairing.SetAuth(unattended)
	stream.OnWaiting(unattended.HandleWaiting)

	idle := services.NewIdleWatch(session.End, nil)
	idle.SetMinutes(cfg.IdleMinutes)
	settings.OnChange(func(c models.Config) { idle.SetMinutes(c.IdleMinutes) })
	go idle.Run(ctx)
	// The terminals live as long as the service, like screen's: a viewer
	// leaving does not close them.
	term := terminal.NewService(shell, stream.SendControl, logger)
	defer term.Close()
	term.OnTyped(idle.Touch)
	stream.OnChannel(term.Channel)
	record := services.NewRecordService(stream.SendControl, status, logger)
	session.OnPermissions(func(peer string, p models.Permissions) {
		if p.View {
			idle.Session(peer)
		} else {
			idle.Session("")
		}
		term.Allow(peer, p)
		record.Allow(peer, p.View, p.Record)
	})
	// Links made before 1.5 could only see and type.
	for _, l := range links.Links() {
		if l.Permissions != terminal.LinkPermissions {
			links.SetLink(l.ID, func(x *models.Link) { x.Permissions = terminal.LinkPermissions })
		}
	}
	control := controllers.NewControl(logger)
	controllers.SessionRoutes(control, session)
	for _, kind := range []string{"terminal_size", "terminal_open", "terminal_close", "terminal_rename"} {
		control.Handle(kind, func(peer string, data []byte) { term.HandleControl(peer, kind, data) })
	}
	control.Handle("recording", record.Handle)
	control.Handle("screenshot", record.HandleScreenshot)
	for _, kind := range []string{"auth", "auth_challenge", "unlink"} {
		control.Handle(kind, unattended.HandleControl)
	}
	stream.OnControl(control.Dispatch)
	stream.OnClosed(session.StreamClosed)

	client := signalclient.New(signalclient.Config{URL: target, Logger: logger}, services.Dispatcher{pairing, opener, session})
	pairing.SetSender(client)
	opener.SetSender(client)
	session.SetSender(client)
	stream.SetSender(client)
	go func() {
		if err := client.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("signal client stopped", "err", err)
			cancel()
		}
	}()
	go func() { _ = stream.Run(ctx) }()
	go func() {
		if err := ctl.Serve(ctx); err != nil {
			logger.Error("cannot open the commands' socket", "path", ctl.Path, "err", err)
		}
	}()
	if !settings.UnattendedOn() {
		logger.Info("nobody can come in yet: set a password (go-link-desk-terminal password), then link a browser (go-link-desk-terminal link)")
	}
	<-ctx.Done()
	session.End(models.EndHost)
	time.Sleep(300 * time.Millisecond) // the last message goes out
	return nil
}

// hostName is the short machine name shown to viewers.
func hostName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "computer"
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}
