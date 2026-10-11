// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** SignalClient.test: the check made before saving a custom server in Settings. */
@MainActor
final class SignalTestTests: XCTestCase {
    /** A socket that answers (or not) as soon as it opens. */
    private final class Answering: SignalSocketFactory {
        final class Sock: SignalSocket {
            var closed = false
            func send(_ text: String) -> Bool { true }
            func close() { closed = true }
        }

        let reply: String?
        let closeInstead: Bool
        var url = ""
        let sock = Sock()

        init(_ reply: String?, closeInstead: Bool = false) {
            self.reply = reply
            self.closeInstead = closeInstead
        }

        func open(url: String, listener: SignalSocketListener) -> SignalSocket? {
            self.url = url
            if closeInstead { listener.onClosed() } else if let reply { listener.onMessage(reply) }
            return sock
        }
    }

    private func run(_ url: String, _ f: Answering, clock: ManualScheduler, timeoutMs: Int = 5000) -> String?? {
        var out: String??
        SignalClient.test(url: url, factory: f, scheduler: clock, timeoutMs: timeoutMs) { out = .some($0) }
        clock.advance(by: timeoutMs + 1)
        return out
    }

    func testAServerThatSaysHelloPasses() {
        let clock = ManualScheduler()
        let f = Answering(#"{"type":"hello","peer_id":"p","ice_servers":[]}"#)
        XCTAssertEqual(.some(nil), run("ws://127.0.0.1:8191/ws", f, clock: clock))
        XCTAssertEqual("ws://127.0.0.1:8191/ws?v=1", f.url)
        XCTAssertTrue(f.sock.closed)
    }

    func testSilenceOrAClosedSocketFails() {
        let clock = ManualScheduler()
        XCTAssertEqual(.some("no answer from the server"), run("wss://a.example/ws", Answering(nil), clock: clock, timeoutMs: 100))
        XCTAssertEqual(.some("no answer from the server"), run("wss://a.example/ws", Answering(#"{"type":"error"}"#), clock: clock, timeoutMs: 100))
        XCTAssertEqual(.some("could not connect"), run("wss://a.example/ws", Answering(nil, closeInstead: true), clock: clock))
    }

    func testReconnectBackoffAndRequests() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let signal = SignalClient(url: "wss://a.example/ws", factory: sockets, scheduler: clock, random: { 0 })
        var states: [ConnectionState] = []
        signal.addStateListener { states.append($0) }
        signal.connect()
        sockets.last.listener.onClosed()
        clock.advance(by: 499)
        XCTAssertEqual(1, sockets.conns.count)
        clock.advance(by: 1) // min backoff / 2 with no jitter
        XCTAssertEqual(2, sockets.conns.count)
        sockets.last.receive(#"{"type":"hello","peer_id":"p","ice_servers":[{"urls":"stun:a.example:3478"}]}"#)
        XCTAssertEqual(.open, signal.state)
        XCTAssertEqual("p", signal.peerId)
        XCTAssertEqual(1, signal.iceServers.count)

        var replies: [String] = []
        signal.request(.obj(("type", "a")), expect: ["a_ok"]) { r in replies.append((try? r.get().type) ?? "fail") }
        signal.request(.obj(("type", "b")), expect: ["b_ok"]) { r in
            if case let .failure(e) = r { replies.append("error:\(e.message):\(e.fromServer)") }
        }
        sockets.last.receive(#"{"type":"a_ok"}"#)
        sockets.last.receive(#"{"type":"error","error":"room not found"}"#)
        XCTAssertEqual(["a_ok", "error:room not found:true"], replies)

        // A drop forgets the ICE servers (TURN credentials expire).
        sockets.last.listener.onClosed()
        XCTAssertEqual([], signal.iceServers)
        XCTAssertEqual("", signal.peerId)
        signal.close()
        XCTAssertEqual(.closed, signal.state)
        XCTAssertEqual([.connecting, .open, .connecting, .closed], states)
    }
}
