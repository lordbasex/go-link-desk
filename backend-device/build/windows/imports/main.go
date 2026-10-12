// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command imports lists the DLLs a Windows program loads at start (its
// import table), one per line: scripts/check-deps.sh windows reads it on
// any system, with no Windows tools.
//
//	go run ./build/windows/imports go-link-desk.exe
package main

import (
	"debug/pe"
	"fmt"
	"os"
	"slices"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: imports FILE.exe")
		os.Exit(2)
	}
	f, err := pe.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	// ImportedLibraries is not implemented for PE files (it returns
	// nothing): the DLLs come from the imported symbols ("name:DLL").
	syms, err := f.ImportedSymbols()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var libs []string
	for _, sym := range syms {
		if _, lib, ok := strings.Cut(sym, ":"); ok && !slices.Contains(libs, lib) {
			libs = append(libs, lib)
		}
	}
	// A Windows program always imports something (KERNEL32 at least).
	if len(libs) == 0 {
		fmt.Fprintln(os.Stderr, "no imports found: not a Windows program, or one this tool cannot read")
		os.Exit(1)
	}
	for _, lib := range libs {
		fmt.Println(lib)
	}
}
