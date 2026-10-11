// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package mainloop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AppKit
void ml_run(void);
void ml_stop(void);
*/
import "C"

import (
	"context"
	"runtime"
)

func init() {
	// The main goroutine stays on the main thread, where Run must serve.
	runtime.LockOSThread()
}

// Run serves the main thread's event loop until ctx ends. Call it from the
// main goroutine.
func Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		C.ml_stop()
	}()
	C.ml_run()
}
