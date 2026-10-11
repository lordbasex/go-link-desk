// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// The input channel's binary packets (docs/protocol.md, "Input"): a port of
// backend-device/pkg/inputproto's Encode. protocol/vectors/input.json holds
// the bytes every implementation tests against.

public enum InputPacket {
    public static let abs: UInt8 = 0x01
    public static let rel: UInt8 = 0x02
    public static let button: UInt8 = 0x10
    public static let wheel: UInt8 = 0x20
    public static let key: UInt8 = 0x30
    public static let text: UInt8 = 0x31
    public static let releaseAll: UInt8 = 0x3F

    /** Mouse buttons, as the protocol numbers them. */
    public enum Button: UInt8, Sendable { case left = 0, middle = 1, right = 2, back = 3, forward = 4 }

    /** Modifier bits. */
    public static let modShift: UInt8 = 1 << 0
    public static let modCtrl: UInt8 = 1 << 1
    public static let modAlt: UInt8 = 1 << 2
    public static let modMeta: UInt8 = 1 << 3
    public static let modCaps: UInt8 = 1 << 4

    /** Text packets carry at most this many bytes of UTF-8. */
    public static let maxText = 1024

    /** The pointer at x, y (0..65535 over the display), with a sequence number. */
    public static func pointer(seq: UInt16, display: UInt8, x: UInt16, y: UInt16) -> [UInt8] {
        var b: [UInt8] = [abs]
        put(&b, seq)
        b.append(display)
        put(&b, x)
        put(&b, y)
        return b
    }

    /** A pointer move by dx, dy (pointer lock). */
    public static func relative(seq: UInt16, dx: Int16, dy: Int16) -> [UInt8] {
        var b: [UInt8] = [rel]
        put(&b, seq)
        put(&b, UInt16(bitPattern: dx))
        put(&b, UInt16(bitPattern: dy))
        return b
    }

    public static func buttonEvent(_ button: Button, down: Bool, display: UInt8, x: UInt16, y: UInt16) -> [UInt8] {
        var b: [UInt8] = [InputPacket.button, button.rawValue, down ? 1 : 0, display]
        put(&b, x)
        put(&b, y)
        return b
    }

    /** The wheel: the browser's sign (dy > 0 scrolls the content down), in pixels or in lines. */
    public static func wheelEvent(dx: Int16, dy: Int16, lines: Bool) -> [UInt8] {
        var b: [UInt8] = [wheel]
        put(&b, UInt16(bitPattern: dx))
        put(&b, UInt16(bitPattern: dy))
        b.append(lines ? 1 : 0)
        return b
    }

    /** A key by its index in protocol/keycodes.json. */
    public static func keyEvent(_ keycode: UInt16, down: Bool, repeating: Bool = false, mods: UInt8) -> [UInt8] {
        var b: [UInt8] = [key]
        put(&b, keycode)
        b.append(down ? (repeating ? 2 : 1) : 0)
        b.append(mods)
        return b
    }

    /** Text to type as it is, cut to maxText bytes without splitting a character. */
    public static func textEvent(_ s: String) -> [UInt8] {
        var bytes = Array(s.utf8)
        if bytes.count > maxText {
            var cut = maxText
            while cut > 0 && (bytes[cut] & 0xC0) == 0x80 { cut -= 1 }
            bytes = Array(bytes[0..<cut])
        }
        return [text] + bytes
    }

    public static func releaseAllEvent() -> [UInt8] { [releaseAll] }

    private static func put(_ b: inout [UInt8], _ v: UInt16) {
        b.append(UInt8(v >> 8))
        b.append(UInt8(v & 0xFF))
    }

    /** 0..1 over the display, as 0..65535. */
    public static func coord(_ v: Double) -> UInt16 {
        UInt16((v.clamped(0, 1) * 65535).rounded())
    }
}
