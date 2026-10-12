// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package keymap

// Evdev is the key's Linux input event code (KEY_*), which a Wayland
// desktop takes by position (its portal's keyboard keycodes); -1: none.
// Most are the Windows scan code itself; the extended ones and a few
// others are listed.
func (k Key) Evdev() int {
	if v, ok := evdevOf[k.Code]; ok {
		return v
	}
	if k.Win > 0 && k.Win < 0x59 {
		return k.Win
	}
	return -1
}

var evdevOf = map[string]int{
	"F13": 183, "F14": 184, "F15": 185, "F16": 186, "F17": 187, "F18": 188,
	"F19": 189, "F20": 190, "F21": 191, "F22": 192, "F23": 193, "F24": 194,
	"PrintScreen": 99, "Pause": 119, "Insert": 110, "Home": 102, "PageUp": 104,
	"Delete": 111, "End": 107, "PageDown": 109,
	"ArrowRight": 106, "ArrowLeft": 105, "ArrowDown": 108, "ArrowUp": 103,
	"NumpadDivide": 98, "NumpadEnter": 96, "NumpadEqual": 117, "NumpadComma": 121,
	"ContextMenu": 127, "MetaLeft": 125, "MetaRight": 126,
	"ControlRight": 97, "AltRight": 100,
	"AudioVolumeMute": 113, "AudioVolumeDown": 114, "AudioVolumeUp": 115,
	"IntlRo": 89, "IntlYen": 124, "Lang1": 122, "Lang2": 123, "Help": 138,
}

var evdevOfX11 = func() map[int]int {
	m := map[int]int{}
	for _, k := range Keys {
		if e := k.Evdev(); k.X11 >= 0 && e >= 0 {
			if _, dup := m[k.X11]; !dup {
				m[k.X11] = e
			}
		}
	}
	return m
}()

// EvdevOfX11 is the evdev code of the key whose X11 keysym (on a US
// layout, as the table gives it) is sym; -1 when no key has it.
func EvdevOfX11(sym int) int {
	if v, ok := evdevOfX11[sym]; ok {
		return v
	}
	return -1
}
