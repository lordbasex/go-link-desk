// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin && (!linux || headless)

package inject

// New returns an injector that does nothing: input injection on Windows
// and Linux comes in phase 7.
func New() Injector { return nop{} }

type nop struct{}

func (nop) Move(float64, float64)                    {}
func (nop) MoveBy(int, int)                          {}
func (nop) Button(int, bool, float64, float64, bool) {}
func (nop) Scroll(int, int, bool)                    {}
func (nop) Key(int, bool, bool, uint8)               {}
func (nop) Text(string)                              {}
func (nop) ReleaseAll()                              {}
func (nop) Trusted() bool                            { return false }

// Supported reports whether this build injects input.
const Supported = false
