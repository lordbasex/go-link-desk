// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"log/slog"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// uiOptions is what the user interface needs from the host.
type uiOptions struct {
	ctx      context.Context // canceled when the host stops
	quit     func()          // stops the host
	headless bool
	status   *services.StatusService
	newCode  func()
	webURL   string // where viewers type the code
	language string
	logger   *slog.Logger
}
