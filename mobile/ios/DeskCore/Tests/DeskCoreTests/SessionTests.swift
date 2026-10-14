// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** A session from the code to its end, against a fake signalhub and a fake peer. */
@MainActor
final class SessionTests: XCTestCase {
    private var sockets: FakeSockets!
    private var clock: ManualScheduler!
    private var peers: FakePeers!
    private var statuses: [SessionStatus] = []

    private func session() -> DeskSession {
        sockets = FakeSockets()
        clock = ManualScheduler()
        peers = FakePeers()
        let signal = SignalClient(url: "wss://signal.example/ws", factory: sockets, scheduler: clock)
        let s = DeskSession(code: "915355636", name: "  Ana  ", ua: "go-link Desk 1.0 on iOS", signal: signal, peers: peers, scheduler: clock)
        statuses = []
        s.events.status = { [unowned self] in self.statuses.append($0) }
        s.start()
        return s
    }

    private func hello() {
        sockets.last.receive(#"{"type":"hello","peer_id":"me","ice_servers":[{"urls":["stun:s.example:3478"]}]}"#)
    }

    private func fromHost(_ payload: String) {
        sockets.last.receive(#"{"type":"signal","from":"H","payload":\#(payload)}"#)
    }

    func testJoinKnockAcceptAnswerAndLive() {
        let s = session()
        XCTAssertEqual(sockets.last.url, "wss://signal.example/ws?v=1")
        hello()
        XCTAssertEqual(sockets.last.sent.last, obj(#"{"type":"join","app":"go-link-desk","code":"915355636"}"#))
        sockets.last.receive(#"{"type":"joined","remote":"H","room_id":"R"}"#)
        XCTAssertEqual(s.status, .waiting)
        let knock = sockets.last.sentText.last!
        XCTAssertEqual(knock, #"{"type":"signal","to":"H","payload":{"kind":"knock","name":"Ana","ua":"go-link Desk 1.0 on iOS","codecs":["h264-high","h264","vp8"]}}"#)

        // A candidate before the offer waits for the peer.
        fromHost(#"{"kind":"candidate","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}"#)
        fromHost(#"{"kind":"accepted","permissions":{"view":true,"control":true,"clipboard":true}}"#)
        XCTAssertEqual(s.status, .connecting)
        XCTAssertTrue(s.permissions.control)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        XCTAssertEqual(peers.servers[0].first?.urls, ["stun:s.example:3478"])
        XCTAssertEqual(peer.offers, ["v=0"])
        XCTAssertEqual(peer.candidates.map(\.candidate), ["c1"])
        XCTAssertEqual(sockets.last.sent.last?["payload"], JSON.obj(("kind", "answer"), ("sdp", "answer-for-v=0")))

        // Control waits for its channel.
        s.setVideoWant(width: 2000, height: 1000)
        XCTAssertTrue(peer.control.isEmpty)
        peer.events.onControlOpen()
        XCTAssertEqual(peer.control, [#"{"type":"video_want","width":2000,"height":1000}"#])
        peer.events.onState(.connected)
        XCTAssertEqual(s.status, .live)

        // The computer's messages.
        var displays: [Display] = []
        s.events.displays = { d, _ in displays = d }
        peer.events.onControlMessage(#"{"type":"welcome","host":{"name":"Mac","os":"darwin"}}"#)
        XCTAssertEqual(s.hostOS, "darwin")
        peer.events.onControlMessage(#"{"type":"displays","displays":[{"id":7,"name":"A","width":100,"height":50},{"id":9,"name":"B","width":10,"height":5}],"current":9}"#)
        XCTAssertEqual(displays.map(\.id), [7, 9])
        var typing: [Bool] = []
        s.events.typing = { typing.append($0) }
        peer.events.onControlMessage(#"{"type":"typing","on":true}"#)
        peer.events.onControlMessage(#"{"type":"typing","on":false}"#)
        XCTAssertEqual(typing, [true, false])
        s.sendTyping(true)
        XCTAssertEqual(peer.control.last, #"{"type":"typing","on":true}"#)
        peer.events.onControlMessage(#"{"type":"ping","id":4}"#)
        XCTAssertEqual(peer.control.last, #"{"type":"pong","id":4}"#)

        // Input goes with the current display's index.
        s.pointer(x: 0.5, y: 1)
        XCTAssertEqual(peer.pointer.last, InputPacket.pointer(seq: 1, display: 1, x: 32768, y: 65535))
        s.button(.right, down: true, x: 0, y: 0)
        XCTAssertEqual(peer.input.last, InputPacket.buttonEvent(.right, down: true, display: 1, x: 0, y: 0))

        // Without control, nothing goes.
        peer.events.onControlMessage(#"{"type":"permissions","permissions":{"view":true}}"#)
        s.pointer(x: 0, y: 0)
        XCTAssertEqual(peer.pointer.count, 1)

        s.hangUp()
        XCTAssertEqual(peer.control.last, #"{"type":"session_end"}"#)
        XCTAssertEqual(s.status, .ended("viewer"))
        XCTAssertTrue(peer.closed)
        XCTAssertTrue(sockets.last.closed)
        XCTAssertEqual(statuses, [.joining, .waiting, .connecting, .live, .ended("viewer")])
    }

    func testRejectedAndUnknownCode() {
        let s = session()
        hello()
        sockets.last.receive(#"{"type":"joined","remote":"H"}"#)
        fromHost(#"{"kind":"rejected","reason":"busy"}"#)
        XCTAssertEqual(s.status, .rejected("busy"))

        let t = session()
        hello()
        sockets.last.receive(#"{"type":"error","error":"invalid or expired code"}"#)
        XCTAssertEqual(t.status, .notFound)

        let u = session()
        hello()
        sockets.last.receive(#"{"type":"error","error":"rate limit exceeded"}"#)
        XCTAssertEqual(u.status, .rateLimited)
    }

    func testNoAnswerFromTheServer() {
        let s = session()
        clock.advance(by: DeskSession.joinTimeoutMs)
        XCTAssertEqual(s.status, .unreachable)
    }

    func testSignalsFromSomeoneElseAreIgnored() {
        let s = session()
        hello()
        sockets.last.receive(#"{"type":"joined","remote":"H"}"#)
        sockets.last.receive(#"{"type":"signal","from":"X","payload":{"kind":"rejected"}}"#)
        XCTAssertEqual(s.status, .waiting)
        sockets.last.receive(#"{"type":"peer_left","from":"H"}"#)
        XCTAssertEqual(s.status, .ended("signal"))
    }

    func testTheHostEndsTheSession() {
        let s = session()
        hello()
        sockets.last.receive(#"{"type":"joined","remote":"H"}"#)
        fromHost(#"{"kind":"accepted","permissions":{}}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        peers.created[0].events.onControlMessage(#"{"type":"session_end","reason":"idle"}"#)
        XCTAssertEqual(s.status, .ended("idle"))
    }

    func testClipboardInPiecesBothWays() {
        let s = session()
        hello()
        sockets.last.receive(#"{"type":"joined","remote":"H"}"#)
        fromHost(#"{"kind":"accepted","permissions":{"clipboard":true}}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        var got: [String] = []
        s.events.clipboard = { got.append($0) }
        peer.events.onControlMessage(#"{"type":"clipboard","id":3,"part":0,"parts":2,"text":"ho"}"#)
        peer.events.onControlMessage(#"{"type":"clipboard","id":3,"part":1,"parts":2,"text":"la"}"#)
        XCTAssertEqual(got, ["hola"])

        let big = String(repeating: "ñ", count: 10_000) // 20000 bytes: two pieces
        XCTAssertTrue(s.sendClipboard(big, paste: true))
        let sent = peer.control.compactMap { JSONText.parseObject($0) }.filter { $0["type"] == "clipboard" }
        XCTAssertEqual(sent.count, 2)
        XCTAssertNil(sent[0]["paste"])
        XCTAssertEqual(sent[1]["paste"], true)
        let joined = sent.map { $0["text"].str(1 << 20) }.joined()
        XCTAssertEqual(joined, big)
    }

    func testScreenshotsAndRecordingAskTheComputerFirst() {
        let s = session()
        hello()
        sockets.last.receive(#"{"type":"joined","remote":"H"}"#)
        fromHost(#"{"kind":"accepted","permissions":{"view":true}}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        var answers: [Bool] = []
        s.askScreenshot { answers.append($0) }
        XCTAssertEqual(answers, [false]) // no permission: nothing asked
        XCTAssertTrue(peer.control.isEmpty)

        peer.events.onControlMessage(#"{"type":"permissions","permissions":{"view":true,"record":true}}"#)
        s.askScreenshot { answers.append($0) }
        XCTAssertEqual(peer.control.last, #"{"type":"screenshot","id":1}"#)
        peer.events.onControlMessage(#"{"type":"screenshot","id":1,"ok":true}"#)
        XCTAssertEqual(answers, [false, true])

        s.startRecording { answers.append($0) }
        XCTAssertEqual(peer.control.last, #"{"type":"recording","on":true,"id":2}"#)
        XCTAssertFalse(s.recording)
        peer.events.onControlMessage(#"{"type":"recording","id":2,"on":true}"#)
        XCTAssertEqual(answers.last, true)
        XCTAssertTrue(s.recording)

        // The person at the computer unticks it: the recording stops here.
        var stopped = 0
        s.events.recordingStopped = { stopped += 1 }
        peer.events.onControlMessage(#"{"type":"permissions","permissions":{"view":true}}"#)
        XCTAssertFalse(s.recording)
        XCTAssertEqual(stopped, 1)

        // No answer in time is a no, and a late yes is turned down.
        peer.events.onControlMessage(#"{"type":"permissions","permissions":{"view":true,"record":true}}"#)
        s.startRecording { answers.append($0) }
        clock.advance(by: 5000)
        XCTAssertEqual(answers.last, false)
        peer.events.onControlMessage(#"{"type":"recording","id":3,"on":true}"#)
        XCTAssertFalse(s.recording)
        XCTAssertEqual(peer.control.last, #"{"type":"recording","id":0,"on":false}"#)
    }
}

final class RecentsTests: XCTestCase {
    func testNewestFirstWithoutRepeats() {
        let a = Recent(code: "111111111", name: "A", os: "darwin", at: Date(timeIntervalSince1970: 1))
        let b = Recent(code: "222222222", name: "B", os: "windows", at: Date(timeIntervalSince1970: 2))
        var list = Recents.add(a, to: [])
        list = Recents.add(b, to: list)
        list = Recents.add(a, to: list)
        XCTAssertEqual(list.map(\.code), ["111111111", "222222222"])
        for i in 0..<20 { list = Recents.add(Recent(code: String(format: "%09d", i), name: "", os: "", at: Date()), to: list) }
        XCTAssertEqual(list.count, Recents.max)
        XCTAssertEqual(Recents.digits("915 355 636"), "915355636")
        XCTAssertNil(Recents.digits("915 355 63"))
        XCTAssertNil(Recents.digits("https://x/915355636"))
    }

    func testStatsRates() {
        var m = StatsMeter()
        var s = StatsMeter.Sample(atMs: 0)
        s.framesDecoded = 0; s.bytesReceived = 0; s.packetsReceived = 0; s.packetsLost = 0
        s.localType = "host"; s.remoteType = "prflx"
        XCTAssertEqual(m.next(s).path, "direct")
        var t = StatsMeter.Sample(atMs: 2000)
        t.framesDecoded = 120; t.bytesReceived = 1_000_000; t.packetsReceived = 90; t.packetsLost = 10
        t.localType = "relay"; t.remoteType = "host"
        let r = m.next(t)
        XCTAssertEqual(r.fps, 60)
        XCTAssertEqual(r.kbps, 4000)
        XCTAssertEqual(r.lossPct, 10)
        XCTAssertEqual(r.path, "relay")
    }
}
