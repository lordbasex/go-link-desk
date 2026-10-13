// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && !darwin

package gui

import (
	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/fyneview"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// The main window and Settings drawn with Fyne as macOS draws them
// (fyneview): on Windows and Linux.

const hasMain = true

func mainOpen(labels map[string]string, h remoteview.MainHandler) error {
	return fyneview.OpenMain(labels, h)
}
func mainSetHome(h remoteview.Home)              { fyneview.SetHome(h) }
func mainShow()                                  { fyneview.ShowMain() }
func mainSetPrefs(p remoteview.Prefs, show bool) { fyneview.SetPrefs(p, show) }
func mainSetTheme(mode int)                      { fyneview.SetTheme(mode) }
func mainSetIcon(png []byte)                     { fyneview.SetIcon(png) }
func mainAbout(version, text string)             { fyneview.ShowAbout(version, text) }
func mainSetMenu(map[string]string)              {}
func useAppTheme(a fyne.App)                     { fyneview.UseTheme(a) }
