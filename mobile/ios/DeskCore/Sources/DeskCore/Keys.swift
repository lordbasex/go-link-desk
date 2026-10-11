// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Keys by position: a hardware keyboard's key (its USB HID usage, what
// UIKit's UIKey.keyCode is) is sent as the index of its KeyboardEvent.code
// in protocol/keycodes.json, so the computer presses the key at the same
// place on its own keyboard, whatever the layouts. Typed text from the
// on-screen keyboard goes as text instead (InputPacket.textEvent).

public enum Keys {
    private static let index: [String: UInt16] = {
        var m: [String: UInt16] = [:]
        for (i, c) in KeyCodes.codes.enumerated() { m[c] = UInt16(i) }
        return m
    }()

    /** The wire keycode of a KeyboardEvent.code, or nil when the table has no such key. */
    public static func keycode(_ code: String) -> UInt16? {
        guard let i = index[code], i != 0 else { return nil }
        return i
    }

    /** The KeyboardEvent.code of a USB HID usage (keyboard page), or nil. */
    public static func code(hidUsage u: Int) -> String? {
        switch u {
        case 0x04...0x1D: return "Key" + String(UnicodeScalar(UInt8(65 + u - 0x04)))
        case 0x1E...0x26: return "Digit\(u - 0x1D)"
        case 0x27: return "Digit0"
        case 0x3A...0x45: return "F\(u - 0x39)"
        case 0x68...0x73: return "F\(u - 0x68 + 13)"
        case 0x59...0x61: return "Numpad\(u - 0x58)"
        default: return hid[u]
        }
    }

    private static let hid: [Int: String] = [
        0x28: "Enter", 0x29: "Escape", 0x2A: "Backspace", 0x2B: "Tab", 0x2C: "Space",
        0x2D: "Minus", 0x2E: "Equal", 0x2F: "BracketLeft", 0x30: "BracketRight", 0x31: "Backslash",
        0x32: "Backslash", // non-US #, at the same place
        0x33: "Semicolon", 0x34: "Quote", 0x35: "Backquote", 0x36: "Comma", 0x37: "Period", 0x38: "Slash",
        0x39: "CapsLock", 0x46: "PrintScreen", 0x47: "ScrollLock", 0x48: "Pause", 0x49: "Insert",
        0x4A: "Home", 0x4B: "PageUp", 0x4C: "Delete", 0x4D: "End", 0x4E: "PageDown",
        0x4F: "ArrowRight", 0x50: "ArrowLeft", 0x51: "ArrowDown", 0x52: "ArrowUp",
        0x53: "NumLock", 0x54: "NumpadDivide", 0x55: "NumpadMultiply", 0x56: "NumpadSubtract",
        0x57: "NumpadAdd", 0x58: "NumpadEnter", 0x62: "Numpad0", 0x63: "NumpadDecimal",
        0x64: "IntlBackslash", 0x65: "ContextMenu", 0x67: "NumpadEqual", 0x75: "Help",
        0x7F: "AudioVolumeMute", 0x80: "AudioVolumeUp", 0x81: "AudioVolumeDown", 0x85: "NumpadComma",
        0x87: "IntlRo", 0x89: "IntlYen", 0x90: "Lang1", 0x91: "Lang2",
        0xE0: "ControlLeft", 0xE1: "ShiftLeft", 0xE2: "AltLeft", 0xE3: "MetaLeft",
        0xE4: "ControlRight", 0xE5: "ShiftRight", 0xE6: "AltRight", 0xE7: "MetaRight",
    ]

    /** The modifier bit a key holds, or 0. */
    public static func modifier(_ code: String) -> UInt8 {
        switch code {
        case "ShiftLeft", "ShiftRight": return InputPacket.modShift
        case "ControlLeft", "ControlRight": return InputPacket.modCtrl
        case "AltLeft", "AltRight": return InputPacket.modAlt
        case "MetaLeft", "MetaRight": return InputPacket.modMeta
        default: return 0
        }
    }
}

/**
 * The bar's special keys and shortcuts. A shortcut presses its keys in
 * order and lets go of them in reverse, as a person would. The
 * computer's system decides what the shortcut is: Cmd on a Mac, Ctrl
 * elsewhere.
 */
public struct Shortcut: Equatable, Sendable {
    public let id: String
    public let codes: [String]

    public init(id: String, codes: [String]) {
        self.id = id
        self.codes = codes
    }

    /** The packets: every key down (with the modifiers held so far), then up in reverse. */
    public func packets() -> [[UInt8]] {
        var mods: UInt8 = 0
        var out: [[UInt8]] = []
        var pressed: [(UInt16, String)] = []
        for c in codes {
            guard let k = Keys.keycode(c) else { continue }
            mods |= Keys.modifier(c)
            out.append(InputPacket.keyEvent(k, down: true, mods: mods))
            pressed.append((k, c))
        }
        for (k, c) in pressed.reversed() {
            mods &= ~Keys.modifier(c)
            out.append(InputPacket.keyEvent(k, down: false, mods: mods))
        }
        return out
    }

    /** The shortcuts for a computer running os ("darwin", "windows", "linux"). */
    public static func forSystem(_ os: String) -> [Shortcut] {
        let mac = os == "darwin"
        let cmd = mac ? "MetaLeft" : "ControlLeft"
        var out = [
            Shortcut(id: "copy", codes: [cmd, "KeyC"]),
            Shortcut(id: "paste", codes: [cmd, "KeyV"]),
            Shortcut(id: "undo", codes: [cmd, "KeyZ"]),
            Shortcut(id: "select_all", codes: [cmd, "KeyA"]),
        ]
        if mac {
            out += [
                Shortcut(id: "switch_apps", codes: ["MetaLeft", "Tab"]),
                Shortcut(id: "spotlight", codes: ["MetaLeft", "Space"]),
                Shortcut(id: "force_quit", codes: ["MetaLeft", "AltLeft", "Escape"]),
                Shortcut(id: "lock", codes: ["ControlLeft", "MetaLeft", "KeyQ"]),
                Shortcut(id: "screenshot", codes: ["MetaLeft", "ShiftLeft", "Digit5"]),
            ]
        } else {
            out += [
                Shortcut(id: "switch_apps", codes: ["AltLeft", "Tab"]),
                Shortcut(id: "start", codes: ["MetaLeft"]),
                Shortcut(id: "ctrl_alt_del", codes: ["ControlLeft", "AltLeft", "Delete"]),
                Shortcut(id: "task_manager", codes: ["ControlLeft", "ShiftLeft", "Escape"]),
                Shortcut(id: "lock", codes: ["MetaLeft", "KeyL"]),
            ]
        }
        return out
    }
}
