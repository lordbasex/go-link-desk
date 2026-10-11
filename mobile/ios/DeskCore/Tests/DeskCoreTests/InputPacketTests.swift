// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** The input packets against protocol/vectors/input.json, the bytes the host and the website test too. */
final class InputPacketTests: XCTestCase {
    func testVectors() throws {
        let data = try Data(contentsOf: repoRoot().appendingPathComponent("protocol/vectors/input.json"))
        let root = try XCTUnwrap(JSONText.parseObject(String(decoding: data, as: UTF8.self)))
        let packets = root["packets"].arr
        XCTAssertGreaterThan(packets.count, 10)
        for v in packets {
            let name = v["name"].str()
            let p = v["packet"]
            let hex = v["hex"].str().replacingOccurrences(of: " ", with: "")
            func n(_ k: String) -> Double { p?[k].num ?? 0 }
            let bytes: [UInt8]
            switch p?["type"].str() {
            case "abs":
                bytes = InputPacket.pointer(seq: UInt16(n("seq")), display: UInt8(n("display")), x: UInt16(n("x")), y: UInt16(n("y")))
            case "rel":
                bytes = InputPacket.relative(seq: UInt16(n("seq")), dx: Int16(n("dx")), dy: Int16(n("dy")))
            case "button":
                bytes = InputPacket.buttonEvent(InputPacket.Button(rawValue: UInt8(n("button")))!, down: p?["down"].isTrue ?? false,
                                                display: UInt8(n("display")), x: UInt16(n("x")), y: UInt16(n("y")))
            case "wheel":
                bytes = InputPacket.wheelEvent(dx: Int16(n("dx")), dy: Int16(n("dy")), lines: n("mode") == 1)
            case "key":
                bytes = InputPacket.keyEvent(UInt16(n("keycode")), down: p?["down"].isTrue ?? false, repeating: p?["repeat"].isTrue ?? false, mods: UInt8(n("mods")))
            case "text":
                bytes = InputPacket.textEvent(p?["text"].str() ?? "")
            case "release_all":
                bytes = InputPacket.releaseAllEvent()
            default:
                XCTFail("unknown packet \(name)")
                continue
            }
            XCTAssertEqual(bytes.map { String(format: "%02x", $0) }.joined(), hex, name)
        }
    }

    func testTextIsCutWithoutSplittingACharacter() {
        let s = String(repeating: "a", count: 1023) + "ñ"
        let b = InputPacket.textEvent(s)
        XCTAssertEqual(b.count, 1 + 1023)
        XCTAssertEqual(InputPacket.coord(1), 65535)
        XCTAssertEqual(InputPacket.coord(-2), 0)
    }

    func testKeysByPosition() {
        XCTAssertEqual(Keys.code(hidUsage: 0x04), "KeyA")
        XCTAssertEqual(Keys.code(hidUsage: 0x1D), "KeyZ")
        XCTAssertEqual(Keys.code(hidUsage: 0x1E), "Digit1")
        XCTAssertEqual(Keys.code(hidUsage: 0x27), "Digit0")
        XCTAssertEqual(Keys.code(hidUsage: 0x3A), "F1")
        XCTAssertEqual(Keys.code(hidUsage: 0x45), "F12")
        XCTAssertEqual(Keys.code(hidUsage: 0x68), "F13")
        XCTAssertEqual(Keys.code(hidUsage: 0x59), "Numpad1")
        XCTAssertEqual(Keys.code(hidUsage: 0xE3), "MetaLeft")
        XCTAssertEqual(Keys.keycode("KeyA"), 1)
        XCTAssertNil(Keys.keycode("Unidentified"))
        XCTAssertNil(Keys.keycode("NoSuchKey"))
        // Every HID usage the map names is in the table.
        for u in 0...0xE7 {
            if let c = Keys.code(hidUsage: u) { XCTAssertNotNil(Keys.keycode(c), "\(u) -> \(c)") }
        }
    }

    func testShortcutPressesInOrderAndReleasesInReverse() {
        let s = Shortcut(id: "x", codes: ["MetaLeft", "ShiftLeft", "Digit5"])
        let meta = Keys.keycode("MetaLeft")!, shift = Keys.keycode("ShiftLeft")!, five = Keys.keycode("Digit5")!
        XCTAssertEqual(s.packets(), [
            InputPacket.keyEvent(meta, down: true, mods: InputPacket.modMeta),
            InputPacket.keyEvent(shift, down: true, mods: InputPacket.modMeta | InputPacket.modShift),
            InputPacket.keyEvent(five, down: true, mods: InputPacket.modMeta | InputPacket.modShift),
            InputPacket.keyEvent(five, down: false, mods: InputPacket.modMeta | InputPacket.modShift),
            InputPacket.keyEvent(shift, down: false, mods: InputPacket.modMeta),
            InputPacket.keyEvent(meta, down: false, mods: 0),
        ])
        XCTAssertTrue(Shortcut.forSystem("darwin").contains { $0.id == "spotlight" })
        XCTAssertTrue(Shortcut.forSystem("windows").contains { $0.id == "ctrl_alt_del" })
    }
}

final class PointerTests: XCTestCase {
    func testSlowSlidesArePreciseAndFastOnesGoFurther() {
        var p = Pointer()
        p.slide(dx: 10, dy: 0, dt: 0.1, pictureWidth: 1000, pictureHeight: 500)
        XCTAssertEqual(p.x, 0.51, accuracy: 1e-9)
        var q = Pointer()
        q.slide(dx: 100, dy: 0, dt: 0.05, pictureWidth: 1000, pictureHeight: 500) // 2000 points a second
        XCTAssertEqual(q.x, 0.8, accuracy: 1e-9)
        q.slide(dx: 1000, dy: 1000, dt: 0.01, pictureWidth: 1000, pictureHeight: 500)
        XCTAssertEqual(q.x, 1)
        XCTAssertEqual(q.y, 1)
    }

    func testPlaceAndFit() {
        let view = Rect(x: 0, y: 0, width: 1000, height: 1000)
        let fit = view.fit(1920, 1080)
        XCTAssertEqual(fit.width, 1000, accuracy: 1e-9)
        XCTAssertEqual(fit.height, 562.5, accuracy: 1e-9)
        XCTAssertEqual(fit.y, 218.75, accuracy: 1e-9)
        var p = Pointer()
        p.place(viewX: 500, viewY: fit.y, picture: fit)
        XCTAssertEqual(p.x, 0.5, accuracy: 1e-9)
        XCTAssertEqual(p.y, 0, accuracy: 1e-9)
    }

    func testZoomKeepsThePointUnderTheFingers() {
        let fit = Rect(x: 0, y: 0, width: 1000, height: 500)
        var z = Zoom()
        z.pinch(by: 2, aroundX: 250, aroundY: 125, fit: fit)
        let r = z.apply(fit)
        XCTAssertEqual(r.width, 2000)
        // The point at 250,125 was the picture's (0.25, 0.25); it still is.
        XCTAssertEqual((250 - r.x) / r.width, 0.25, accuracy: 1e-9)
        XCTAssertEqual((125 - r.y) / r.height, 0.25, accuracy: 1e-9)
        z.pinch(by: 100, aroundX: 0, aroundY: 0, fit: fit)
        XCTAssertEqual(z.scale, Zoom.maxScale)
        z.pan(dx: 1e6, dy: 1e6, fit: fit)
        XCTAssertEqual(z.offsetX, 0)
        z.pinch(by: 0.01, aroundX: 0, aroundY: 0, fit: fit)
        XCTAssertEqual(z.apply(fit), fit)
    }
}
