// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build headless

package main

import "github.com/lordbasex/go-link-desk/backend-device/pkg/mainloop"

// runUI serves the main thread until the host stops: this build has no
// window, so the code is read from the log.
func runUI(o uiOptions) {
	mainloop.Run(o.ctx)
}

// guiBuilt: this build has no window, so it always runs headless.
const guiBuilt = false
