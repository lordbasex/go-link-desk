// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin

package cursor

import "errors"

// Position is not read here yet.
func Position() (x, y float64) { return 0, 0 }

// Current is not read here yet.
func Current() (Shape, error) { return Shape{}, errors.New("cursor: not supported on this system") }

// Supported reports whether this build reads the pointer.
const Supported = false
