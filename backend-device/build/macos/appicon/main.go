// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command appicon writes the macOS app icon set (go-link-desk.iconset) from the
// app icon drawn by pkg/trayicon (RenderApp); iconutil turns it into go-link-desk.icns.
// With -ios it writes the iOS app's icon (RenderAppSquare) and Home's logo
// into its asset catalog; with -linux, a Linux desktop's icon theme folder
// (DIR/hicolor/<size>x<size>/apps/org.go-link.desk.png).
//
//	go run ./build/macos/appicon DIR/go-link-desk.iconset
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/trayicon"
)

func main() {
	// The iOS app's icon (1024 pixels, to the edges) and the logo Home shows.
	if len(os.Args) == 3 && os.Args[1] == "-ios" {
		dir := os.Args[2]
		files := map[string][]byte{
			"AppIcon.appiconset/AppIcon-1024.png": trayicon.RenderAppSquare(1024),
			"Logo.imageset/Logo@2x.png":           trayicon.RenderApp(88, false),
			"Logo.imageset/Logo@3x.png":           trayicon.RenderApp(132, false),
		}
		for name, b := range files {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		return
	}
	// A Linux desktop's icon theme (hicolor), named as the app's ID.
	if len(os.Args) == 3 && os.Args[1] == "-linux" {
		for _, size := range []int{48, 64, 128, 256, 512} {
			dir := filepath.Join(os.Args[2], "hicolor", fmt.Sprintf("%dx%d", size, size), "apps")
			err := os.MkdirAll(dir, 0o755)
			if err == nil {
				err = os.WriteFile(filepath.Join(dir, "org.go-link.desk.png"), trayicon.RenderApp(size, false), 0o644)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: appicon DIR/go-link-desk.iconset | appicon -ios DIR/Assets.xcassets | appicon -linux DIR/icons")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The sizes macOS asks for, each at 1x and 2x.
	for _, size := range []int{16, 32, 128, 256, 512} {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			name := fmt.Sprintf("icon_%dx%d%s.png", size, size, suffix)
			if err := os.WriteFile(filepath.Join(dir, name), trayicon.RenderApp(size*scale, false), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}
