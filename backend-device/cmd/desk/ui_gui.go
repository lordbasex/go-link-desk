// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package main

import "github.com/lordbasex/go-link-desk/backend-device/internal/gui"

// runUI shows the window and tray icon, unless the host runs headless. It
// blocks until the host stops.
func runUI(o uiOptions) {
	if o.headless {
		<-o.ctx.Done()
		return
	}
	gui.Run(gui.Options{
		Ctx:        o.ctx,
		Quit:       o.quit,
		Status:     o.status,
		NewCode:    o.newCode,
		EndSession: o.endSession,
		SetConsent: o.setConsent,
		WebURL:     o.webURL,
		Language:   o.language,
		Version:    version,
		Logger:     o.logger,
	})
}

// guiBuilt: this build has the window; --headless still turns it off.
const guiBuilt = true
