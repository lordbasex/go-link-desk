// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"github.com/lordbasex/go-link-desk/backend-device/pkg/keymap"
	"log/slog"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/inject"
)

// LogInjector writes to the log what it would do instead of doing it:
// the end-to-end tests (--test-input-log) check the viewer's input reaches
// the host without moving anyone's mouse.
type LogInjector struct{ Log *slog.Logger }

var _ inject.Injector = LogInjector{}

func (l LogInjector) Move(x, y float64) { l.Log.Debug("injected", "what", "move", "x", x, "y", y) }
func (l LogInjector) MoveBy(dx, dy int) {
	l.Log.Info("injected", "what", "move_by", "dx", dx, "dy", dy)
}
func (l LogInjector) Button(b int, down bool, x, y float64, at bool) {
	l.Log.Info("injected", "what", "button", "button", b, "down", down, "x", int(x), "y", int(y))
}
func (l LogInjector) Scroll(dx, dy int, lines bool) {
	l.Log.Info("injected", "what", "scroll", "dx", dx, "dy", dy, "lines", lines)
}

// Key logs the key as macOS's virtual key code on every system, so the
// tests read the same numbers wherever the host runs.
func (l LogInjector) Key(vk int, down, repeat bool, mods uint8) {
	l.Log.Info("injected", "what", "key", "vk", keymap.MacOf(vk), "down", down, "repeat", repeat, "mods", mods)
}
func (l LogInjector) Text(s string) { l.Log.Info("injected", "what", "text", "text", s) }
func (l LogInjector) ReleaseAll()   { l.Log.Info("injected", "what", "release_all") }
func (l LogInjector) Trusted() bool { return true }
