// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package main

import (
	"github.com/lordbasex/go-link-desk/backend-device/internal/gui"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/mainloop"
)

// runUI shows the window and tray icon, unless the host runs headless. It
// blocks until the host stops.
func runUI(o uiOptions) {
	if o.headless {
		mainloop.Run(o.ctx)
		return
	}
	gui.Run(gui.Options{
		Ctx:            o.ctx,
		Quit:           o.quit,
		Status:         o.status,
		NewCode:        o.newCode,
		EndSession:     o.endSession,
		SetPermissions: o.setPermissions,
		Chat:           o.chat,
		Call:           o.call,
		Files:          o.files,
		Settings:       o.settings,
		Links:          o.links,
		Remotes:        o.remotes,
		SignalURL:      o.signalURL,
		Connect:        o.connect,
		AskCalls:       o.setConsent != nil,
		SetConsent:     o.setConsent,
		Woken:          o.woken,
		WebURL:         o.webURL,
		Language:       o.language,
		Version:        version,
		Logger:         o.logger,
	})
}

// guiBuilt: this build has the window; --headless still turns it off.
const guiBuilt = true
