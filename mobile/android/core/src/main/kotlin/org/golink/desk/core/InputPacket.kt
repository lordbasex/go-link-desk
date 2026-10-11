// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlin.math.roundToInt

// The input channel's binary packets (docs/protocol.md, "Input"): a port of
// backend-device/pkg/inputproto's Encode. protocol/vectors/input.json holds
// the bytes every implementation tests against.

object InputPacket {
    const val ABS: Int = 0x01
    const val REL: Int = 0x02
    const val BUTTON: Int = 0x10
    const val WHEEL: Int = 0x20
    const val KEY: Int = 0x30
    const val TEXT: Int = 0x31
    const val RELEASE_ALL: Int = 0x3F

    /** Mouse buttons, as the protocol numbers them. */
    enum class Button(val wire: Int) { LEFT(0), MIDDLE(1), RIGHT(2), BACK(3), FORWARD(4) }

    /** Modifier bits. */
    const val MOD_SHIFT: Int = 1 shl 0
    const val MOD_CTRL: Int = 1 shl 1
    const val MOD_ALT: Int = 1 shl 2
    const val MOD_META: Int = 1 shl 3
    const val MOD_CAPS: Int = 1 shl 4

    /** Text packets carry at most this many bytes of UTF-8. */
    const val MAX_TEXT = 1024

    /** The pointer at x, y (0..65535 over the display), with a sequence number. */
    fun pointer(seq: Int, display: Int, x: Int, y: Int): ByteArray =
        bytes(ABS) + u16(seq) + byteArrayOf(display.toByte()) + u16(x) + u16(y)

    /** A pointer move by dx, dy (pointer lock). */
    fun relative(seq: Int, dx: Int, dy: Int): ByteArray = bytes(REL) + u16(seq) + u16(dx) + u16(dy)

    fun buttonEvent(button: Button, down: Boolean, display: Int, x: Int, y: Int): ByteArray =
        bytes(BUTTON, button.wire, if (down) 1 else 0, display) + u16(x) + u16(y)

    /** The wheel: the browser's sign (dy > 0 scrolls the content down), in pixels or in lines. */
    fun wheelEvent(dx: Int, dy: Int, lines: Boolean): ByteArray = bytes(WHEEL) + u16(dx) + u16(dy) + bytes(if (lines) 1 else 0)

    /** A key by its index in protocol/keycodes.json. */
    fun keyEvent(keycode: Int, down: Boolean, repeating: Boolean = false, mods: Int): ByteArray =
        bytes(KEY) + u16(keycode) + bytes(if (down) (if (repeating) 2 else 1) else 0, mods)

    /** Text to type as it is, cut to MAX_TEXT bytes without splitting a character. */
    fun textEvent(s: String): ByteArray {
        var b = s.toByteArray(Charsets.UTF_8)
        if (b.size > MAX_TEXT) {
            var cut = MAX_TEXT
            while (cut > 0 && (b[cut].toInt() and 0xC0) == 0x80) cut--
            b = b.copyOf(cut)
        }
        return bytes(TEXT) + b
    }

    fun releaseAllEvent(): ByteArray = bytes(RELEASE_ALL)

    /** 0..1 over the display, as 0..65535. */
    fun coord(v: Double): Int = (v.coerceIn(0.0, 1.0) * 65535).roundToInt()

    private fun bytes(vararg v: Int) = ByteArray(v.size) { v[it].toByte() }

    private fun u16(v: Int) = byteArrayOf((v shr 8).toByte(), v.toByte())
}
