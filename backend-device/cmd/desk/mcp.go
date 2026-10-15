// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lordbasex/go-link-desk/backend-device/internal/mcpserver"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/repositories"
)

// runMCP answers an AI assistant on standard input and output
// ("go-link-desk mcp", which the assistant runs once Settings › Assistants
// added go-link Desk to it). It runs beside the app, never as the host:
// it takes no lock and writes nothing to standard output but MCP.
func runMCP(args []string) error {
	fs := flag.NewFlagSet("go-link-desk mcp", flag.ContinueOnError)
	config := fs.String("config", "", "path to device.json (default: user config dir)")
	signalURL := fs.String("server-signaling", "", "signaling server URL for computers linked without one (default: signal_url in device.json)")
	debug := fs.Bool("debug", false, "verbose logs")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	out, closeLog := mcpLog()
	defer closeLog()
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	store, cfg, _, err := loadConfig(*config)
	if err != nil {
		return err
	}
	target, err := resolveSignalURL(*signalURL, cfg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(store.Path())
	remotes := repositories.NewRemotesFile(dir)
	logger.Info("assistant server starting", "version", version, "config", store.Path(), "assistants", cfg.Assistants.Enabled, "act", cfg.Assistants.Act)
	srv := mcpserver.New(mcpserver.Options{
		Version: version, SignalURL: target, OwnDeviceID: cfg.DeviceID, Logger: logger,
		Config: func() (models.Config, error) {
			c, _, err := store.LoadOrCreate()
			return c, err
		},
		Remotes: remotes.Load,
		Audit:   mcpserver.NewAudit(filepath.Join(dir, mcpserver.AuditFile)),
	})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = srv.Run(ctx, &mcp.StdioTransport{})
	logger.Info("assistant server done", "err", err)
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// mcpLog is the assistant server's log: a file of its own beside the
// app's (standard output carries MCP, and the assistant may show standard
// error).
func mcpLog() (io.Writer, func()) {
	dir, err := logDir()
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		return io.Discard, func() {}
	}
	path := filepath.Join(dir, "mcp.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return io.Discard, func() {}
	}
	return f, func() { _ = f.Close() }
}
