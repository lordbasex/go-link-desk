// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

// Keys by position: a hardware keyboard's key (Android's KeyEvent key code,
// which the key layout sets by the key's place, not by the language) is
// sent as the index of its KeyboardEvent.code in protocol/keycodes.json, so
// the computer presses the key at the same place on its own keyboard,
// whatever the layouts. Typed text from the on-screen keyboard goes as text
// instead (InputPacket.textEvent).

object Keys {
    private val index: Map<String, Int> = KeyCodes.codes.withIndex().associate { it.value to it.index }

    /** The wire keycode of a KeyboardEvent.code, or null when the table has no such key. */
    fun keycode(code: String): Int? = index[code]?.takeIf { it != 0 }

    /** The KeyboardEvent.code of an Android key code (android.view.KeyEvent.KEYCODE_*), or null. */
    fun code(androidKey: Int): String? = when (androidKey) {
        in 29..54 -> "Key" + ('A' + (androidKey - 29))
        in 7..16 -> "Digit${androidKey - 7}"
        in 131..142 -> "F${androidKey - 130}"
        in 144..153 -> "Numpad${androidKey - 144}"
        else -> ANDROID[androidKey]
    }

    private val ANDROID: Map<Int, String> = mapOf(
        19 to "ArrowUp", 20 to "ArrowDown", 21 to "ArrowLeft", 22 to "ArrowRight",
        24 to "AudioVolumeUp", 25 to "AudioVolumeDown", 164 to "AudioVolumeMute",
        55 to "Comma", 56 to "Period", 57 to "AltLeft", 58 to "AltRight", 59 to "ShiftLeft", 60 to "ShiftRight",
        61 to "Tab", 62 to "Space", 66 to "Enter", 67 to "Backspace", 68 to "Backquote", 69 to "Minus", 70 to "Equal",
        71 to "BracketLeft", 72 to "BracketRight", 73 to "Backslash", 74 to "Semicolon", 75 to "Quote", 76 to "Slash",
        82 to "ContextMenu", 92 to "PageUp", 93 to "PageDown", 111 to "Escape", 112 to "Delete",
        113 to "ControlLeft", 114 to "ControlRight", 115 to "CapsLock", 116 to "ScrollLock",
        117 to "MetaLeft", 118 to "MetaRight", 120 to "PrintScreen", 121 to "Pause",
        122 to "Home", 123 to "End", 124 to "Insert", 143 to "NumLock",
        154 to "NumpadDivide", 155 to "NumpadMultiply", 156 to "NumpadSubtract", 157 to "NumpadAdd",
        158 to "NumpadDecimal", 159 to "NumpadComma", 160 to "NumpadEnter", 161 to "NumpadEqual",
        216 to "IntlYen", 217 to "IntlRo", 259 to "Help",
    )

    /** The modifier bits of Android's meta state (KeyEvent.getMetaState). */
    fun mods(metaState: Int): Int {
        var m = 0
        if (metaState and 0x1 != 0) m = m or InputPacket.MOD_SHIFT // META_SHIFT_ON
        if (metaState and 0x1000 != 0) m = m or InputPacket.MOD_CTRL // META_CTRL_ON
        if (metaState and 0x2 != 0) m = m or InputPacket.MOD_ALT // META_ALT_ON
        if (metaState and 0x10000 != 0) m = m or InputPacket.MOD_META // META_META_ON
        if (metaState and 0x100000 != 0) m = m or InputPacket.MOD_CAPS // META_CAPS_LOCK_ON
        return m
    }

    /** The modifier bit a key holds, or 0. */
    fun modifier(code: String): Int = when (code) {
        "ShiftLeft", "ShiftRight" -> InputPacket.MOD_SHIFT
        "ControlLeft", "ControlRight" -> InputPacket.MOD_CTRL
        "AltLeft", "AltRight" -> InputPacket.MOD_ALT
        "MetaLeft", "MetaRight" -> InputPacket.MOD_META
        else -> 0
    }

    /** The KeyboardEvent.code of a letter or digit typed ("c" is KeyC), for a shortcut by position. */
    fun codeOf(c: Char): String? = when (c.lowercaseChar()) {
        in 'a'..'z' -> "Key" + c.uppercaseChar()
        in '0'..'9' -> "Digit$c"
        else -> null
    }
}

/**
 * The bar's special keys and shortcuts. A shortcut presses its keys in
 * order and lets go of them in reverse, as a person would. The
 * computer's system decides what the shortcut is: Cmd on a Mac, Ctrl
 * elsewhere.
 */
data class Shortcut(val id: String, val codes: List<String>) {
    /** The packets: every key down (with the modifiers held so far), then up in reverse. */
    fun packets(): List<ByteArray> {
        var mods = 0
        val out = mutableListOf<ByteArray>()
        val pressed = mutableListOf<Pair<Int, String>>()
        for (c in codes) {
            val k = Keys.keycode(c) ?: continue
            mods = mods or Keys.modifier(c)
            out += InputPacket.keyEvent(k, down = true, mods = mods)
            pressed += k to c
        }
        for ((k, c) in pressed.asReversed()) {
            mods = mods and Keys.modifier(c).inv()
            out += InputPacket.keyEvent(k, down = false, mods = mods)
        }
        return out
    }

    companion object {
        /** The shortcuts for a computer running os ("darwin", "windows", "linux"). */
        fun forSystem(os: String): List<Shortcut> {
            val mac = os == "darwin"
            val cmd = if (mac) "MetaLeft" else "ControlLeft"
            val out = mutableListOf(
                Shortcut("copy", listOf(cmd, "KeyC")),
                Shortcut("paste", listOf(cmd, "KeyV")),
                Shortcut("undo", listOf(cmd, "KeyZ")),
                Shortcut("select_all", listOf(cmd, "KeyA")),
            )
            out += if (mac) {
                listOf(
                    Shortcut("switch_apps", listOf("MetaLeft", "Tab")),
                    Shortcut("spotlight", listOf("MetaLeft", "Space")),
                    Shortcut("force_quit", listOf("MetaLeft", "AltLeft", "Escape")),
                    Shortcut("lock", listOf("ControlLeft", "MetaLeft", "KeyQ")),
                    Shortcut("screenshot", listOf("MetaLeft", "ShiftLeft", "Digit5")),
                )
            } else {
                listOf(
                    Shortcut("switch_apps", listOf("AltLeft", "Tab")),
                    Shortcut("start", listOf("MetaLeft")),
                    Shortcut("ctrl_alt_del", listOf("ControlLeft", "AltLeft", "Delete")),
                    Shortcut("task_manager", listOf("ControlLeft", "ShiftLeft", "Escape")),
                    Shortcut("lock", listOf("MetaLeft", "KeyL")),
                )
            }
            return out
        }
    }
}
