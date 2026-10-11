// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command desk runs the go-link Desk host: it registers with signalhub,
// keeps an invite-only session room open and shows its code, asks the
// person at the computer about every viewer that knocks, and streams to
// the one accepted over WebRTC.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/lordbasex/go-link-desk/backend-device/internal/controllers"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/beacon"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/camera"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/clipboard"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// version is replaced at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "go-link-desk:", err)
		os.Exit(1)
	}
}

// flags are the command line options.
type flags struct {
	config   string
	signal   string
	webURL   string
	headless bool
	debug    bool
	source   string
	version  bool
	// autoAccept lets every viewer in without asking: end-to-end tests
	// only, and only with the test card.
	autoAccept bool
	// inputLog writes the viewers' input to the log instead of injecting
	// it, and lets them control: end-to-end tests only.
	inputLog bool
	// clipboardFile keeps the host's clipboard in a file: end-to-end
	// tests only.
	clipboardFile string
}

func parseFlags(args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("go-link-desk", flag.ContinueOnError)
	fs.StringVar(&f.config, "config", "", "path to device.json (default: user config dir)")
	fs.StringVar(&f.signal, "server-signaling", "", "signaling server URL for this run, e.g. wss://signal.example.com/ws (default: signal_url in device.json, else "+models.DefaultSignalURL+")")
	fs.StringVar(&f.webURL, "web-url", "", "website shown with the code, for this run (default: web_url in device.json, else "+models.DefaultWebURL+")")
	fs.BoolVar(&f.headless, "headless", false, "run without window or tray icon; the session code goes to the log")
	fs.BoolVar(&f.debug, "debug", false, "verbose logs")
	fs.StringVar(&f.source, "source", "screen", "what sessions show: screen, or testpattern (diagnostics, CI)")
	fs.BoolVar(&f.version, "version", false, "print the version and exit")
	fs.BoolVar(&f.autoAccept, "test-auto-accept", false, "accept every viewer without asking (automated tests only; needs --source testpattern)")
	fs.BoolVar(&f.inputLog, "test-input-log", false, "let accepted viewers control, but write their input to the log instead of injecting it (automated tests only; needs --test-auto-accept)")
	fs.StringVar(&f.clipboardFile, "test-clipboard-file", "", "keep the host's clipboard in this file instead of the computer's (automated tests only; needs --test-input-log)")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if f.source != "screen" && f.source != "testpattern" {
		return f, fmt.Errorf("--source must be screen or testpattern, not %q", f.source)
	}
	// Sessions need consent: skipping it is only for tests, which never
	// show anyone a real screen.
	if f.autoAccept && f.source != "testpattern" && !allowAutoAcceptScreen {
		return f, errors.New("--test-auto-accept needs --source testpattern")
	}
	if f.inputLog && !f.autoAccept {
		return f, errors.New("--test-input-log needs --test-auto-accept")
	}
	if f.clipboardFile != "" && !f.inputLog {
		return f, errors.New("--test-clipboard-file needs --test-input-log")
	}
	return f, nil
}

func run(args []string) error {
	f, err := parseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.version {
		fmt.Println("go-link Desk", version)
		return nil
	}

	level := slog.LevelInfo
	if f.debug {
		level = slog.LevelDebug
	}
	logOut, closeLog := logOutput()
	defer closeLog()
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	store, cfg, created, err := loadConfig(f.config)
	if err != nil {
		return err
	}
	// One host per user: a locked file next to device.json.
	lock, err := instancelock.Acquire(filepath.Join(filepath.Dir(store.Path()), "desk.lock"))
	if err != nil {
		return fmt.Errorf("cannot start: %w", err)
	}
	defer lock.Release()
	target, err := resolveSignalURL(f.signal, cfg)
	if err != nil {
		return err
	}
	logger.Info("host starting", "version", version, "device_id", cfg.DeviceID, "config", store.Path(), "first_run", created, "signaling", target, "source", f.source)
	if encrypted, _ := signalclient.CheckURL(target); !encrypted && !isLoopback(target) {
		logger.Warn("signaling URL is not encrypted (ws://); use wss:// outside local development", "url", target)
	}
	if !guiBuilt && !f.headless {
		f.headless = true // a build without a window
	}
	if wantsHeadless() && !f.headless {
		logger.Info("no desktop found, running headless")
		f.headless = true
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	status := services.NewStatusService(cfg.DeviceID, version, target)
	status.SetSavedLinks(len(cfg.Links))
	ice := services.NewICEStore()
	pairing := services.NewPairingService(services.PairingConfig{DeviceID: cfg.DeviceID, DeviceSecret: cfg.DeviceSecret, Logger: logger}, status, ice)
	opener := services.NewRoomOpener()
	meta := services.SessionMeta{Name: hostName(), OS: runtime.GOOS}
	source := "test-pattern"
	if f.source == "screen" {
		if capture.Supported {
			source = "screen"
		} else {
			logger.Warn("screen capture is not available on this system yet: sessions show the test card")
		}
	}
	api, err := services.NewWebRTCAPI(cfg.UDPPort, false)
	if err != nil {
		return err
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, UDPPort: cfg.UDPPort, AnnounceIPs: cfg.AnnounceIPs,
		Welcome: meta, Source: source, Logger: logger,
	}, ice)
	if err != nil {
		return err
	}
	if source == "screen" {
		stream.SetSource(&services.ScreenSource{Fallback: &services.TestCardSource{Width: 960, Height: 720, FPS: 30}, Log: logger})
		stream.OnBeacon(func(on bool, d capture.Display) { beacon.Set(on, d.X, d.Y) })
		if !capture.HasAccess() {
			logger.Warn("this app may not record the screen yet: allow it in System Settings › Privacy & Security › Screen Recording")
		}
	}
	session := services.NewSessionService(opener, status, meta, stream, logger)
	if f.autoAccept {
		logger.Warn("every viewer is accepted without asking (--test-auto-accept): for automated tests only")
		session.SetConsent(services.AutoAccept)
		// With control: the tests that log the input, and measurement
		// builds on a test machine.
		if f.inputLog || (allowAutoAcceptScreen && f.source == "screen") {
			session.SetConsent(services.AutoAcceptAll)
		}
	}
	var injector inject.Injector = inject.New()
	if f.inputLog {
		injector = services.LogInjector{Log: logger}
	}
	input := services.NewInputService(injector, stream.Displays, logger)
	stream.OnInput(func(peer, channel string, data []byte) {
		if channel == "pointer" {
			input.HandlePointer(peer, data)
		} else {
			input.HandleInput(peer, data)
		}
	})
	// A viewer whose connection stops answering may never send its
	// releases: let go of everything it held.
	stream.OnStall(input.Release)
	board, shareClipboard := clipboard.System(), clipboard.Supported
	if f.clipboardFile != "" {
		board, shareClipboard = &services.FileBoard{Path: f.clipboardFile}, true
	}
	clip := services.NewClipboardService(board, stream.SendControl, input.Paste, logger)
	chat := services.NewChatService(stream.SendControl, logger)
	// The video call: the computer's camera and microphone with the real
	// screen, a test card and a tone with the test card.
	var callMedia services.CallMedia = services.SystemCallMedia{}
	if source != "screen" || !camera.Supported {
		callMedia = services.TestCallMedia{Log: logger}
	}
	call := services.NewCallService(stream, callMedia, logger)
	if f.autoAccept {
		call.SetConsent(services.AutoAcceptCall)
	}
	session.OnPermissions(func(peer string, p models.Permissions) {
		input.Allow(peer, p.Control)
		stream.SetAudio(peer, p.Audio)
		clip.Allow(peer, p.Clipboard && shareClipboard)
		if p.View {
			chat.SetPeer(peer)
			name := ""
			if st := status.Snapshot(); st.Session != nil && st.Session.Active != nil {
				name = st.Session.Active.Name
			}
			call.SetPeer(peer, name)
		} else {
			chat.SetPeer("")
			call.SetPeer("", "")
		}
		if p.Control && !input.Trusted() {
			logger.Warn("the viewer may control, but this app may not move the mouse or type yet: allow it in System Settings › Privacy & Security › Accessibility")
			stream.Notice(services.NoticeNoAccessibility)
		}
	})
	control := controllers.NewControl(logger)
	controllers.SessionRoutes(control, session)
	control.Handle("clipboard", clip.Handle)
	control.Handle("chat", chat.Handle)
	for _, kind := range []string{"call_request", "call_media", "call_end"} {
		control.Handle(kind, func(peer string, data []byte) { call.HandleControl(peer, kind, data) })
	}
	stream.OnControl(control.Dispatch)
	stream.OnClosed(session.StreamClosed)
	stream.OnStats(status.SetStream)
	// The pairing service registers on connect, so it goes first; the
	// opener must see room_opened before the session does.
	handlers := services.Dispatcher{pairing, opener, session}

	client := signalclient.New(signalclient.Config{URL: target, Logger: logger}, handlers)
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
	go func() {
		if err := stream.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("video source stopped", "err", err)
		}
	}()
	go services.NewMetricsService(ctx, status, 5*time.Second, nil).Run(ctx)

	web := cfg.EffectiveWebURL()
	if f.webURL != "" {
		web = f.webURL
	}
	var setConsent func(services.Consent)
	if !f.autoAccept {
		setConsent = session.SetConsent // the window's dialog asks
	}
	runUI(uiOptions{
		setConsent: setConsent,
		ctx:        ctx,
		quit:       cancel,
		headless:   f.headless,
		status:     status,
		newCode:    session.NewCode,
		endSession: func() {
			session.End(models.EndHost)
		},
		setPermissions: session.SetPermissions,
		chat:           chat,
		call:           call,
		webURL:         web,
		language:       cfg.Language,
		logger:         logger,
	})
	logger.Info("shutting down")
	cancel()
	return nil
}

// loadConfig opens device.json (created on first run).
func loadConfig(path string) (*repositories.ConfigFile, models.Config, bool, error) {
	if path == "" {
		var err error
		if path, err = repositories.DefaultConfigPath(); err != nil {
			return nil, models.Config{}, false, err
		}
	}
	store := repositories.NewConfigFile(path)
	cfg, created, err := store.LoadOrCreate()
	return store, cfg, created, err
}

// resolveSignalURL picks the signaling server, in order of precedence:
// the --server-signaling flag (this run only), signal_url in device.json
// (permanent), and finally the project's public server.
func resolveSignalURL(flagValue string, cfg models.Config) (string, error) {
	target := cfg.EffectiveSignalURL()
	source := "signal_url in device.json"
	if flagValue != "" {
		target = flagValue
		source = "--server-signaling"
	}
	if _, err := signalclient.CheckURL(target); err != nil {
		return "", fmt.Errorf("invalid %s: %w", source, err)
	}
	return target, nil
}

// isLoopback reports whether the URL points at this machine.
func isLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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

// maxLogSize is when the log file starts again (the old one is kept as .1).
const maxLogSize = 5 << 20

// logOutput is where the log goes: the terminal when there is one, else
// (the app opened from the Finder or the Start menu)
// ~/go-link-desk/logs/desk.log, so the code and errors are not lost.
func logOutput() (io.Writer, func()) {
	// A real terminal, not /dev/null (where macOS sends an app's output).
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr, func() {}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Stderr, func() {}
	}
	dir := filepath.Join(home, "go-link-desk", "logs")
	if os.MkdirAll(dir, 0o700) != nil {
		return os.Stderr, func() {}
	}
	path := filepath.Join(dir, "desk.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return os.Stderr, func() {}
	}
	return io.MultiWriter(f, os.Stderr), func() { _ = f.Close() }
}

// wantsHeadless reports whether there is no desktop to show a window on.
func wantsHeadless() bool {
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}
