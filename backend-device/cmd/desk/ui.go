// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package main

import (
	"context"
	"log/slog"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// uiOptions is what the user interface needs from the host.
type uiOptions struct {
	ctx      context.Context // canceled when the host stops
	quit     func()          // stops the host
	headless bool
	status   *services.StatusService
	newCode  func()
	// endSession disconnects the viewer in session.
	endSession func()
	// setPermissions changes what the viewer in session may do.
	setPermissions func(models.Permissions)
	// chat is the session's chat.
	chat *services.ChatService
	// call is the session's video call.
	call *services.CallService
	// files moves files with the viewer.
	files *services.FilesService
	// settings and links are the Settings window's: unattended access,
	// the linked browsers, idle time and locking.
	settings *services.SettingsService
	links    *services.LinkService
	// setConsent hands the window's consent dialog to the session (nil:
	// the tests' auto-accept stays).
	setConsent func(services.Consent)
	webURL     string // where viewers type the code
	language   string
	logger     *slog.Logger
}
