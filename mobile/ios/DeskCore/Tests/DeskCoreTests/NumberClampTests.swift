// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** Untrusted numbers are narrowed without trapping (docs/protocol.md, control channel). */
final class NumberClampTests: XCTestCase {
    func testDoubleNarrowingNeverTraps() {
        XCTAssertEqual((1e300).clampedToInt(), Int.max)
        XCTAssertEqual((-1e300).clampedToInt(), Int.min)
        XCTAssertEqual((1e300).clampedToInt64(), Int64.max)
        XCTAssertEqual((-1e300).clampedToInt64(), Int64.min)
        XCTAssertEqual((42.6).clampedToInt(), 43)
        XCTAssertEqual((Double.nan).clampedToInt(), 0)        // non-finite -> safe 0
        XCTAssertEqual((Double.infinity).clampedToInt64(), 0) // non-finite -> safe 0
    }

    func testJSONIntReadersClampOutOfRangeNumbers() {
        let m = JSONText.parseObject(#"{"big":1e300,"neg":-1e300,"small":5,"huge":1e300}"#)!
        let o = JSON.object(m)
        XCTAssertEqual(o["big"].int, Int.max)
        XCTAssertEqual(o["neg"].int, Int.min)
        XCTAssertEqual(o["small"].int, 5)
        XCTAssertEqual(o["big"].int64, Int64.max)
        XCTAssertEqual(o["huge"].uint32, UInt32.max)
        XCTAssertEqual(o["neg"].uint32, 0)
    }

    // A control-channel message with an out-of-range number does not crash
    // the session; it is read as a clamped value.
    func testAStreamStatsWithAHugeNumberDoesNotCrash() {
        let m = JSONText.parseObject(#"{"type":"stream_stats","width":1e300,"height":0,"kbps":0,"fps":0}"#)!
        let o = JSON.object(m)
        XCTAssertEqual(o["width"].int, Int.max) // no trap narrowing it
    }

    // An ICE candidate relayed with an out-of-range sdpMLineIndex is clamped
    // to 0...255, so narrowing it to Int32 for WebRTC never traps.
    func testACandidateWithAHugeLineIndexIsClamped() {
        let m = JSONText.parseObject(#"{"kind":"candidate","candidate":{"candidate":"c","sdpMid":"0","sdpMLineIndex":3000000000}}"#)!
        guard case let .candidate(c)? = RtcSignals.parse(.object(m)) else {
            return XCTFail("not a candidate")
        }
        XCTAssertEqual(c.sdpMLineIndex, 255)
        XCTAssertNotNil(Int32(exactly: c.sdpMLineIndex))
    }
}
