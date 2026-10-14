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
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/lordbasex/go-link-desk/backend-device/internal/controllers"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/awake"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/beacon"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/camera"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/clipboard"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/testpattern"
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
	// unattended turns unattended access on for this run, and password
	// sets its password: end-to-end tests only.
	unattended bool
	password   string
	name       string
	codeFile   string
	filesDir   string
	demoScreen bool
	relayOnly  bool
	picture    string
	// connect opens a session to another computer at start (its 9-digit
	// session code).
	connect string
	// testViewer runs only the viewer, without a window, through a door
	// (join, claim or reach): end-to-end tests only.
	testViewer         string
	testViewerCode     string
	testViewerPassword string
	testViewerFrames   int
}

// demoMaxUpload is the largest file a demonstration host takes.
const demoMaxUpload = 25 << 20

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
	fs.BoolVar(&f.unattended, "test-unattended", false, "turn unattended access on (automated tests only; needs --test-auto-accept)")
	fs.StringVar(&f.picture, "test-picture", "", "a PNG or JPEG the session shows instead of the test card, still and silent (screenshots only; needs --source testpattern)")
	fs.StringVar(&f.codeFile, "test-code-file", "", "a public demonstration host: keep the session and pairing codes (and the unattended password) in this JSON file as they change, for a web page that shows them (needs --test-clipboard-file, --test-files-folder, and --test-input-log with --source testpattern or --test-demo-screen)")
	fs.BoolVar(&f.demoScreen, "test-demo-screen", false, "a demonstration host that shows this computer's real screen and takes the viewers' mouse and keyboard: only on a computer kept for it, locked down, with nothing of anyone's on it (needs --test-code-file and --source screen)")
	fs.BoolVar(&f.relayOnly, "test-relay-only", false, "carry every session through the TURN relay, never straight to the viewer, so a firewall may let the computer reach the signaling server and the relay only (a demonstration host; needs --test-code-file)")
	fs.StringVar(&f.filesDir, "test-files-folder", "", "the only folder viewers may browse, download from and send files to (DIR/Received), with small files only (a demonstration host; needs --test-code-file)")
	fs.StringVar(&f.name, "test-name", "", "the computer's name viewers see, instead of this machine's (tests, screenshots and a demonstration computer; needs --source testpattern or --test-demo-screen)")
	fs.StringVar(&f.password, "test-unattended-password", "", "the unattended password (automated tests only; --test-unattended needs it: unattended access always has a password)")
	fs.StringVar(&f.connect, "connect", "", "connect to other computers with their 9-digit session codes at start (separated by commas)")
	fs.StringVar(&f.testViewer, "test-viewer", "", "run only the viewer, without a window, through join, claim or reach (automated tests only)")
	fs.StringVar(&f.testViewerCode, "test-viewer-code", "", "the session or pairing code (automated tests only; needs --test-viewer join or claim)")
	fs.StringVar(&f.testViewerPassword, "test-viewer-password", "", "the unattended password to give (automated tests only; needs --test-viewer reach)")
	fs.IntVar(&f.testViewerFrames, "test-viewer-frames", 30, "frames to receive before leaving (automated tests only)")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	switch f.testViewer {
	case "", "join", "claim", "reach":
	default:
		return f, fmt.Errorf("--test-viewer must be join, claim or reach, not %q", f.testViewer)
	}
	if f.source != "screen" && f.source != "testpattern" {
		return f, fmt.Errorf("--source must be screen or testpattern, not %q", f.source)
	}
	// Sessions need consent: skipping it is only for tests, which never
	// show anyone a real screen, and a demonstration computer, whose
	// screen is there to be shown.
	if f.autoAccept && f.source != "testpattern" && !allowAutoAcceptScreen && !f.demoScreen {
		return f, errors.New("--test-auto-accept needs --source testpattern")
	}
	if f.demoScreen && (f.codeFile == "" || f.source != "screen" || !f.autoAccept) {
		return f, errors.New("--test-demo-screen needs --test-code-file, --test-auto-accept and --source screen")
	}
	if f.inputLog && !f.autoAccept {
		return f, errors.New("--test-input-log needs --test-auto-accept")
	}
	if f.clipboardFile != "" && !f.inputLog && !f.demoScreen {
		return f, errors.New("--test-clipboard-file needs --test-input-log")
	}
	if f.unattended && !f.autoAccept {
		return f, errors.New("--test-unattended needs --test-auto-accept")
	}
	if f.picture != "" && f.source != "testpattern" {
		return f, errors.New("--test-picture needs --source testpattern")
	}
	// A public demonstration host: the test card, input only logged.
	// A public demonstration host: anyone connects, so nothing they reach
	// is the computer's own (the input logged, the clipboard a file, a
	// folder of its own).
	// (A demonstration computer's own screen and input are what it shows.)
	testCard := f.inputLog && f.source == "testpattern"
	if f.codeFile != "" && (f.clipboardFile == "" || f.filesDir == "" || !(testCard || f.demoScreen)) {
		return f, errors.New("--test-code-file needs --test-clipboard-file, --test-files-folder, and --test-input-log with --source testpattern or --test-demo-screen")
	}
	if f.relayOnly && f.codeFile == "" {
		return f, errors.New("--test-relay-only needs --test-code-file")
	}
	if f.filesDir != "" && f.codeFile == "" {
		return f, errors.New("--test-files-folder needs --test-code-file")
	}
	if f.name != "" && f.source != "testpattern" && !f.demoScreen {
		return f, errors.New("--test-name needs --source testpattern or --test-demo-screen")
	}
	if f.unattended && f.password == "" {
		return f, errors.New("--test-unattended needs --test-unattended-password: unattended access always has a password")
	}
	if f.password != "" && !f.unattended {
		return f, errors.New("--test-unattended-password needs --test-unattended")
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
	if f.testViewer != "" {
		target, err := resolveSignalURL(f.signal, cfg)
		if err != nil {
			return err
		}
		return runTestViewer(f, cfg, target, filepath.Dir(store.Path()), logger)
	}
	// One host per user: a locked file next to device.json.
	lockPath := filepath.Join(filepath.Dir(store.Path()), "desk.lock")
	lock, err := instancelock.Acquire(lockPath)
	if errors.Is(err, instancelock.ErrLocked) && !f.headless && instancelock.Wake(lockPath) {
		// Opened again (the Start menu, a double click): the one running
		// shows its window, and this one has nothing to do.
		logger.Info("go-link Desk is running already: its window comes forward")
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot start: %w", err)
	}
	defer lock.Release()
	target, err := resolveSignalURL(f.signal, cfg)
	if err != nil {
		return err
	}
	logger.Info("host starting", "version", version, "device_id", cfg.DeviceID, "config", store.Path(), "first_run", created, "signaling", target, "source", f.source)
	if encrypted, _ := signalclient.CheckURL(target); !encrypted && !signalclient.IsLoopback(target) {
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

	save := store.Save
	if f.unattended {
		// For this run only: what is saved keeps the file's own setting.
		saved := cfg.Unattended
		save = func(c models.Config) error {
			c.Unattended = saved
			return store.Save(c)
		}
		cfg.Unattended.Enabled = true
		if f.password != "" {
			if cfg.Unattended.PasswordHash, err = services.HashPassword(f.password); err != nil {
				return err
			}
		}
	}
	settings := services.NewSettingsService(cfg, save, logger)
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
	if f.name != "" {
		meta.Name = f.name
	}
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
	// The fixed port, opened in the router by the app when asked.
	var ports *services.PortMapper
	var outside func() (netip.Addr, int)
	if cfg.UDPPort > 0 && cfg.MapUDPPort {
		ports = &services.PortMapper{Port: cfg.UDPPort, Log: logger}
		outside = ports.Outside
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, UDPPort: cfg.UDPPort, AnnounceIPs: cfg.AnnounceIPs, Outside: outside,
		Welcome: meta, Source: source, Logger: logger, RelayOnly: f.relayOnly,
	}, ice)
	if err != nil {
		return err
	}
	if f.picture != "" {
		card, err := testPicture(f.picture)
		if err != nil {
			return err
		}
		stream.SetSource(card)
	}
	if source == "screen" {
		stream.SetSource(&services.ScreenSource{Fallback: &services.TestCardSource{Width: 960, Height: 720, FPS: 30}, Log: logger})
		stream.OnBeacon(func(on bool, d capture.Display) { beacon.Set(on, d.X, d.Y) })
		if !capture.HasAccess() {
			logger.Warn("this app may not record the screen yet: allow it in System Settings › Privacy & Security › Screen Recording")
		}
	}
	session := services.NewSessionService(opener, status, meta, stream, logger)
	web := cfg.EffectiveWebURL()
	if f.webURL != "" {
		web = f.webURL
	}
	if f.codeFile != "" {
		codes := &services.CodeFile{Path: f.codeFile, WebURL: web, Name: meta.Name, Password: f.password, Log: logger}
		status.OnChange(codes.Update)
		codes.Update(status.Snapshot())
	}
	if f.autoAccept {
		logger.Warn("every viewer is accepted without asking (--test-auto-accept): for automated tests only")
		session.SetConsent(services.AutoAccept)
		// With control: the tests that log the input, and measurement
		// builds on a test machine.
		if f.inputLog || (allowAutoAcceptScreen && f.source == "screen") {
			session.SetConsent(services.AutoAcceptAll)
		}
		if f.codeFile != "" {
			session.SetConsent(services.AutoAcceptDemo)
		}
		if f.demoScreen {
			logger.Warn("a demonstration computer: its real screen goes to every viewer (--test-demo-screen)")
		}
	}
	var injector inject.Injector = inject.New()
	if f.inputLog {
		injector = services.LogInjector{Log: logger}
	}
	input := services.NewInputService(injector, stream.Displays, logger)
	// A session nobody uses ends after the time set in Settings.
	idle := services.NewIdleWatch(session.End, nil)
	idle.SetMinutes(cfg.IdleMinutes)
	settings.OnChange(func(c models.Config) { idle.SetMinutes(c.IdleMinutes) })
	go idle.Run(ctx)
	stream.OnInput(func(peer, channel string, data []byte) {
		idle.Touch(peer)
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
	logger.Info("clipboard", "board", fmt.Sprintf("%T", board))
	clip := services.NewClipboardService(board, stream.SendControl, input.Paste, logger)
	chat := services.NewChatService(stream.SendControl, logger)
	record := services.NewRecordService(stream.SendControl, status, logger)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	files := services.NewFilesService(home, filepath.Join(home, "Downloads", services.ReceivedFolder), stream.SendFiles, logger)
	if f.filesDir != "" {
		files = services.NewFilesService(f.filesDir, filepath.Join(f.filesDir, "Received"), stream.SendFiles, logger)
		files.SetMaxUpload(demoMaxUpload)
	}
	stream.OnFiles(files.Handle)
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
	// Unattended access: browsers linked with the pairing code come back
	// without anyone accepting, with their link's permissions.
	session.SetUnattended(settings)
	unattended := services.NewUnattendedService(services.UnattendedConfig{
		Links: links, Session: session, Settings: settings, Send: stream.SendControl, Logger: logger,
	})
	links.SetTransport(stream.SendControl, func(peer string) { session.Drop(peer, models.EndRevoked) })
	pairing.SetLinks(session)
	pairing.SetAuth(unattended)
	stream.OnWaiting(unattended.HandleWaiting)
	var inSession struct {
		sync.Mutex
		peer string
	}
	session.OnPermissions(func(peer string, p models.Permissions) {
		// While a session is on the display stays awake; when it ends the
		// screen locks, if the person asked for it.
		inSession.Lock()
		started := p.View && inSession.peer != peer
		ended := !p.View && inSession.peer == peer
		if started {
			inSession.peer = peer
		} else if ended {
			inSession.peer = ""
		}
		inSession.Unlock()
		if started {
			idle.Session(peer)
			awake.Hold()
		} else if ended {
			idle.Session("")
			awake.Release()
			if settings.Get().LockOnEnd {
				if !services.LockScreen(injector) {
					logger.Warn("cannot lock the screen on this system")
				} else if !input.Trusted() {
					logger.Warn("cannot lock the screen: allow this app in System Settings › Privacy & Security › Accessibility")
				}
			}
		}
		input.Allow(peer, p.Control)
		stream.SetAudio(peer, p.Audio)
		clip.Allow(peer, p.Clipboard && shareClipboard)
		files.Allow(peer, p.Files)
		record.Allow(peer, p.View, p.Record)
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
		if p.Control && !inject.Supported {
			logger.Warn("the viewer may control, but this system cannot move the mouse or type in this version")
		} else if p.Control && !input.Trusted() {
			logger.Warn("the viewer may control, but this app may not move the mouse or type yet: allow it in System Settings › Privacy & Security › Accessibility")
			stream.Notice(services.NoticeNoAccessibility)
		}
	})
	control := controllers.NewControl(logger)
	controllers.SessionRoutes(control, session)
	control.Handle("clipboard", clip.Handle)
	control.Handle("chat", func(peer string, data []byte) {
		idle.Touch(peer)
		chat.Handle(peer, data)
	})
	control.Handle("typing", chat.HandleTyping)
	control.Handle("recording", record.Handle)
	control.Handle("screenshot", record.HandleScreenshot)
	for _, kind := range []string{"auth", "auth_challenge", "unlink"} {
		control.Handle(kind, unattended.HandleControl)
	}
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
	portsDone := make(chan struct{})
	if ports != nil {
		go func() {
			defer close(portsDone)
			_ = ports.Run(ctx)
		}()
	} else {
		close(portsDone)
	}

	// The computers this app connects to, next to device.json.
	remotesFile := repositories.NewRemotesFile(filepath.Dir(store.Path()))
	known, err := remotesFile.Load()
	if err != nil {
		logger.Warn("cannot read the remote computers", "err", err)
	}
	remotes := services.NewRemotesService(known, remotesFile.Save, logger)
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
		files:          files,
		settings:       settings,
		links:          links,
		remotes:        remotes,
		signalURL:      target,
		connect:        f.connect,
		webURL:         web,
		woken:          lock.Woken(),
		language:       cfg.Language,
		ports:          ports,
		logger:         logger,
	})
	logger.Info("shutting down")
	cancel()
	// The port comes out of the router before the app goes.
	select {
	case <-portsDone:
	case <-time.After(4 * time.Second):
	}
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
// (the app opened from the Finder or the Start menu) logDir's desk.log,
// so the code and errors are not lost.
func logOutput() (io.Writer, func()) {
	// A real terminal, not /dev/null (where macOS sends an app's output).
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr, func() {}
	}
	dir, err := logDir()
	if err != nil {
		return os.Stderr, func() {}
	}
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
	// A Windows program built without a console has no standard error
	// (its handle is 0); the console build's may be a file or a pipe.
	if h := os.Stderr.Fd(); runtime.GOOS == "windows" && (h == 0 || h == ^uintptr(0)) {
		return f, func() { _ = f.Close() }
	}
	return io.MultiWriter(f, os.Stderr), func() { _ = f.Close() }
}

// logDir is the log's folder: ~/go-link-desk/logs, and on Windows
// %LOCALAPPDATA%\go-link-desk\logs, where Windows keeps a program's own
// files (not in the user's folder, among their documents).
func logDir() (string, error) {
	if runtime.GOOS == "windows" {
		dir, err := os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "go-link-desk", "logs"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "go-link-desk", "logs"), nil
}

// wantsHeadless reports whether there is no desktop to show a window on.
func wantsHeadless() bool {
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// testPicture is the --test-picture source: the picture at up to 1920
// pixels wide, keeping its shape, with even sizes.
func testPicture(path string) (*services.TestCardSource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("--test-picture: %w", err)
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w > 1920 {
		h = h * 1920 / w
		w = 1920
	}
	w, h = w&^1, h&^1
	if w < 16 || h < 16 {
		return nil, errors.New("--test-picture: the picture is too small")
	}
	return &services.TestCardSource{Width: w, Height: h, FPS: 30, Picture: testpattern.Picture(img, w, h)}, nil
}
