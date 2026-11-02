// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

// Command desk, on LordOS, is the headless engine of the Desk go-link
// app: everything go-link Desk does but its window, which is a React app at
// the other end of a socket (lordos/app/ENGINE.md). LordOS's lord-server
// starts it as the app's companion, with the app on fd 3 and the share
// connection on fd 4 (docs/GO-LINK.md 3.2).
//
// There is no --test-* flag here, no auto-accept and no updater: every
// visitor is asked through the app, and without the app nobody comes in.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/appsock"
	"github.com/lordbasex/go-link-desk/backend-device/internal/controllers"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/awake"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/clipboard"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/instancelock"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/netprobe"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// version is replaced at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

// appFD is the inherited socket to the app.
const appFD = 3

// goneGrace is how long the engine stays after the app leaves, for the
// refusals of whoever was being asked to reach them.
const goneGrace = 500 * time.Millisecond

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "go-link-desk:", err)
		os.Exit(1)
	}
}

// dataDir is the app's data folder, the one place the sandbox lets this
// program write besides ~/Downloads.
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "lordos", "appdata", "go-link-desk"), nil
}

// appConn is the inherited socket. Without it nobody could accept anyone,
// so the engine doesn't start.
func appConn() (net.Conn, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(appFD, &st); err != nil {
		return nil, errors.New("no app on fd 3")
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return nil, errors.New("fd 3 is not a socket")
	}
	f := os.NewFile(appFD, "app")
	defer f.Close()
	return net.FileConn(f)
}

func logTo(dir string) (io.Writer, func()) {
	if os.MkdirAll(dir, 0o700) != nil {
		return os.Stderr, func() {}
	}
	path := filepath.Join(dir, "desk.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return os.Stderr, func() {}
	}
	return io.MultiWriter(f, os.Stderr), func() { _ = f.Close() }
}

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

func localIP() string {
	if a := netprobe.LocalIP(); a.IsValid() {
		return a.String()
	}
	return ""
}

// connectShare is the API, or a connection already gone: the engine still
// runs, the app can show why, and every visitor is turned down.
func connectShare(log *slog.Logger) *lordshare.Client {
	c, err := lordshare.Connect()
	if err == nil {
		return c
	}
	log.Warn("no screen-sharing API", "err", err)
	a, b := net.Pipe()
	_ = b.Close()
	c = lordshare.New(a)
	lordshare.SetShared(c)
	return c
}

// shareCheck is what the app's consent runs before bothering anyone.
func shareCheck(c *lordshare.Client) error {
	select {
	case <-c.Done():
		return services.ErrUnavailable
	default:
	}
	st, ok := c.Check(time.Second)
	if !ok || st.Blocked != lordshare.OK {
		return services.ErrUnavailable
	}
	return nil
}

// warningFile marks that the person saw the scam warning once.
func warningFile(dir string) (seen func() bool, set func()) {
	p := filepath.Join(dir, "scam_warning_seen")
	return func() bool {
			_, err := os.Stat(p)
			return err == nil
		}, func() {
			_ = os.WriteFile(p, nil, 0o600)
		}
}

func run(args []string) error {
	fs := flag.NewFlagSet("go-link-desk", flag.ContinueOnError)
	config := fs.String("config", "", "path to device.json (default: the app's data folder)")
	debug := fs.Bool("debug", false, "verbose logs")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println("go-link Desk", version)
		return nil
	}
	// The inherited descriptors first: nothing else may take their numbers.
	conn, err := appConn()
	if err != nil {
		return err
	}
	dir, err := dataDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "device.json")
	if *config != "" {
		cfgPath = *config
		dir = filepath.Dir(cfgPath)
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	out, closeLog := logTo(dir)
	defer closeLog()
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	store := repositories.NewConfigFile(cfgPath)
	cfg, created, err := store.LoadOrCreate()
	if err != nil {
		return err
	}
	lock, err := instancelock.Acquire(filepath.Join(dir, "desk.lock"))
	if err != nil {
		return fmt.Errorf("cannot start: %w", err)
	}
	defer lock.Release()
	target := cfg.EffectiveSignalURL()
	encrypted, err := signalclient.CheckURL(target)
	if err != nil {
		return fmt.Errorf("invalid signal_url: %w", err)
	}
	if !encrypted && !signalclient.IsLoopback(target) {
		return fmt.Errorf("refusing to use the unencrypted signaling URL %q", target)
	}
	logger.Info("engine starting", "version", version, "device_id", cfg.DeviceID, "first_run", created, "signaling", target)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	share := connectShare(logger)
	defer share.Close()
	settings := services.NewSettingsService(cfg, store.Save, logger)
	status := services.NewStatusService(cfg.DeviceID, version, target)
	status.SetSavedLinks(len(cfg.Links))
	hostKey, err := linkauth.ParseKey(cfg.HostKey)
	if err != nil {
		return fmt.Errorf("the device key is invalid: %w", err)
	}
	links := services.NewLinkService(services.LinkConfig{
		DeviceID: cfg.DeviceID, HostKey: hostKey, Links: cfg.Links, Logger: logger,
		Save: func(l []models.Link) error {
			return settings.Update(func(c *models.Config) { c.Links = l })
		},
	}, status)
	ice := services.NewICEStore()
	pairing := services.NewPairingService(services.PairingConfig{DeviceID: cfg.DeviceID, DeviceSecret: cfg.DeviceSecret, Logger: logger}, status, ice)
	opener := services.NewRoomOpener()
	meta := services.SessionMeta{Name: hostName(), OS: runtime.GOOS}
	api, err := services.NewWebRTCAPI(cfg.UDPPort, false)
	if err != nil {
		return err
	}
	var ports *services.PortMapper
	var outside func() (netip.Addr, int)
	if cfg.UDPPort > 0 && cfg.MapUDPPort {
		ports = &services.PortMapper{Port: cfg.UDPPort, Log: logger}
		outside = ports.Outside
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, UDPPort: cfg.UDPPort, AnnounceIPs: cfg.AnnounceIPs, Outside: outside,
		Welcome: meta, Source: "screen", Logger: logger, PickCodec: services.EncodedCodec,
	}, ice)
	if err != nil {
		return err
	}
	session := services.NewSessionService(opener, status, meta, stream, logger)
	stream.SetSource(&services.EncodedSource{Share: share, Stream: stream, Log: logger, End: func(string) { session.End(models.EndHost) }})

	// The consent is always the app's.
	seen, setSeen := warningFile(dir)
	consent := &services.SocketConsent{Ready: func() error { return shareCheck(share) }, Warned: seen, SetWarned: setSeen}
	callAsk := &services.SocketCallConsent{}
	session.SetConsent(consent)

	injector := inject.New()
	input := services.NewInputService(injector, stream.Displays, logger)
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
	stream.OnStall(input.Release)
	board, shareClipboard := clipboard.System(), clipboard.Supported
	clip := services.NewClipboardService(board, stream.SendControl, input.Paste, logger)
	chat := services.NewChatService(stream.SendControl, logger)
	record := services.NewRecordService(stream.SendControl, status, logger)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	downloads := filepath.Join(home, "Downloads")
	guard := &services.FilesGuard{Home: home, Downloads: downloads, Send: stream.SendFiles, Log: logger}
	files := services.NewFilesService(home, downloads, guard.SendFiles, logger)
	guard.Next = files.Handle
	stream.OnFiles(guard.HandleFiles)
	call := services.NewCallService(stream, services.SystemCallMedia{}, logger)
	call.SetConsent(callAsk)
	session.SetUnattended(settings)
	unattended := services.NewUnattendedService(services.UnattendedConfig{
		Links: links, Session: session, Settings: settings, Send: stream.SendControl, Logger: logger,
	})
	links.SetTransport(stream.SendControl, func(peer string) { session.Drop(peer, models.EndRevoked) })
	links.SetFingerprints(stream.Fingerprints)
	pairing.SetLinks(session)
	pairing.SetAuth(unattended)
	stream.OnWaiting(unattended.HandleWaiting)
	var inSession struct {
		sync.Mutex
		peer string
	}
	session.OnPermissions(func(peer string, p models.Permissions) {
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
				share.Lock()
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
		if p.Control && !input.Trusted() {
			logger.Warn("the viewer may control, but this app has no remote-input permission")
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
	handlers := services.Dispatcher{pairing, opener, session}

	client := signalclient.New(signalclient.Config{URL: target, Logger: logger, Renewable: func() bool { p, _, _ := session.Active(); return p == "" }}, handlers)
	pairing.SetSender(client)
	opener.SetSender(client)
	session.SetSender(client)
	stream.SetSender(client)

	remotesFile := repositories.NewRemotesFile(dir)
	known, err := remotesFile.Load()
	if err != nil {
		logger.Warn("cannot read the remote computers", "err", err)
	}
	remotes := services.NewRemotesService(known, remotesFile.Save, logger)

	// The socket is bound before anything can knock.
	sock := appsock.New(conn, logger)
	engine := appsock.Bind(appsock.Deps{
		Sock: sock, Ctx: ctx, Quit: cancel, Status: status, NewCode: session.NewCode,
		EndSession:     func() { session.End(models.EndHost) },
		SetPermissions: session.SetPermissions,
		Consent:        consent, CallAsk: callAsk, Chat: chat, Call: call, Files: files, Settings: settings, Links: links, Remotes: remotes, Ports: ports,
		Version: version, Hostname: hostName(), LocalIP: localIP(), WebURL: cfg.EffectiveWebURL(), SignalURL: target,
		DataDir: dir, Home: home, Downloads: downloads, Logger: logger,
		Perms: func() []appsock.Perm { return systemPerms(share) },
	})
	served := make(chan error, 1)
	go func() { served <- sock.Serve() }()
	go engine.Run(ctx)

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

	// The engine dies with the app. Whoever waits for an answer is refused.
	select {
	case err := <-served:
		logger.Info("the app is gone", "err", err)
	case <-ctx.Done():
	}
	engine.AppGone()
	session.End(models.EndHost)
	time.Sleep(goneGrace)
	cancel()
	sock.Close()
	select {
	case <-portsDone:
	case <-time.After(4 * time.Second):
	}
	logger.Info("engine stopped")
	return nil
}

// systemPerms answers `perms` from the system's two permissions.
func systemPerms(share *lordshare.Client) []appsock.Perm {
	st, ok := share.State()
	if !share.Running() {
		if s, got := share.Check(500 * time.Millisecond); got {
			st, ok = s, true
		}
	}
	return []appsock.Perm{
		{ID: "screen", Granted: ok && st.Granted&lordshare.FlagVideo != 0},
		{ID: "accessibility", Granted: ok && st.Granted&lordshare.FlagInput != 0},
	}
}
