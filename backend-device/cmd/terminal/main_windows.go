// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command go-link-desk-terminal is for Linux (and macOS): Windows has the
// desktop app.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "go-link-desk-terminal runs on Linux; on Windows use go-link Desk")
	os.Exit(1)
}
