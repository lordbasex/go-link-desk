// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build headless

package main

// runUI waits until the host stops: this build has no window, so the code
// is read from the log.
func runUI(o uiOptions) {
	<-o.ctx.Done()
}

// guiBuilt: this build has no window, so it always runs headless.
const guiBuilt = false
