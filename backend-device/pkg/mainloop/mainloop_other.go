// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package mainloop

import "context"

// Run waits until ctx ends: nothing else needs the main thread here.
func Run(ctx context.Context) { <-ctx.Done() }
