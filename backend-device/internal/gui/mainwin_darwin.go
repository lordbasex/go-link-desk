// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless && darwin

package gui

import (
	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/fyneview"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

// The main window and Settings: macOS draws them natively (remoteview).

var hasMain = remoteview.Supported

func mainOpen(labels map[string]string, h remoteview.MainHandler) error {
	return remoteview.OpenMain(labels, h)
}
func mainSetHome(h remoteview.Home)              { remoteview.SetHome(h) }
func mainShow()                                  { remoteview.ShowMain() }
func mainSetPrefs(p remoteview.Prefs, show bool) { remoteview.SetPrefs(p, show) }
func mainSetTheme(mode int)                      { remoteview.SetTheme(mode) }
func mainSetIcon(png []byte)                     { remoteview.SetDockIcon(png) }
func mainAbout(version, text string)             { remoteview.ShowAbout(version, text) }
func mainSetMenu(labels map[string]string)       { remoteview.SetAppMenu(labels) }

// useAppTheme: the Mac keeps Fyne's theme for its few Fyne windows, with
// faint text that reads.
func useAppTheme(a fyne.App) { fyneview.UseReadableTheme(a) }

// styleWindow: macOS draws the windows' frames in its appearance.
func styleWindow(fyne.Window) {}
