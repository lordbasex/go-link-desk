// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build desk_test_screen

package main

// allowAutoAcceptScreen: measurement builds only (-tags desk_test_screen,
// never a release) may accept viewers of the real screen without asking,
// to measure capture, latency and CPU on a test machine.
const allowAutoAcceptScreen = true
