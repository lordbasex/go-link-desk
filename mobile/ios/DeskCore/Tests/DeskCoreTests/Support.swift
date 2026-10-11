// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
@testable import DeskCore

/** A clock the tests move by hand, so timers fire exactly when expected. */
@MainActor
final class ManualScheduler: Scheduler {
    private final class Item: CancelHandle {
        let due: Int
        let seq: Int
        let fn: @MainActor () -> Void
        var cancelled = false

        init(due: Int, seq: Int, fn: @escaping @MainActor () -> Void) {
            self.due = due
            self.seq = seq
            self.fn = fn
        }

        func cancel() { cancelled = true }
    }

    private(set) var now = 0
    private var items: [Item] = []
    private var seq = 0

    @discardableResult
    func after(ms: Int, _ fn: @escaping @MainActor () -> Void) -> CancelHandle {
        seq += 1
        let item = Item(due: now + max(0, ms), seq: seq, fn: fn)
        items.append(item)
        return item
    }

    /** Moves the clock forward, running every timer that falls due, in order. */
    func advance(by ms: Int) {
        let end = now + ms
        while true {
            items.removeAll { $0.cancelled }
            guard let next = items.filter({ $0.due <= end }).min(by: { ($0.due, $0.seq) < ($1.due, $1.seq) }) else { break }
            items.removeAll { $0 === next }
            now = next.due
            next.fn()
        }
        now = end
    }
}

/** A signalhub stand-in: records what the client sends and lets tests answer. */
@MainActor
final class FakeSockets: SignalSocketFactory {
    final class Conn: SignalSocket {
        let url: String
        let listener: SignalSocketListener
        var sent: [JSONObject] = []
        var closed = false

        init(url: String, listener: SignalSocketListener) {
            self.url = url
            self.listener = listener
        }

        func send(_ text: String) -> Bool {
            sent.append(JSONText.parseObject(text)!)
            sentText.append(text)
            return !closed
        }

        var sentText: [String] = []

        func close() { closed = true }

        func receive(_ text: String) { listener.onMessage(text) }
    }

    var conns: [Conn] = []
    var last: Conn { conns.last! }

    func open(url: String, listener: SignalSocketListener) -> SignalSocket? {
        let c = Conn(url: url, listener: listener)
        conns.append(c)
        return c
    }
}

@MainActor
final class FakePeer: RtcPeer {
    var control: [String] = []
    var pointer: [[UInt8]] = []
    var input: [[UInt8]] = []
    var candidates: [IceCandidateInit] = []
    var offers: [String] = []
    var closed = false
    var events: RtcPeerEvents!

    func answer(offerSdp: String, done: @escaping (Result<String, Error>) -> Void) {
        offers.append(offerSdp)
        done(.success("answer-for-\(offerSdp)"))
    }

    func addCandidate(_ candidate: IceCandidateInit) { candidates.append(candidate) }

    func sendControl(_ text: String) -> Bool {
        control.append(text)
        return true
    }

    func sendPointer(_ packet: [UInt8]) -> Bool {
        pointer.append(packet)
        return true
    }

    func sendInput(_ packet: [UInt8]) -> Bool {
        input.append(packet)
        return true
    }

    func close() { closed = true }
}

@MainActor
final class FakePeers: RtcPeerFactory {
    var created: [FakePeer] = []
    var servers: [[IceServer]] = []

    func create(iceServers: [IceServer], events: RtcPeerEvents) -> RtcPeer {
        let p = FakePeer()
        p.events = events
        created.append(p)
        servers.append(iceServers)
        return p
    }
}

func obj(_ text: String) -> JSONObject { JSONText.parseObject(text)! }

/** The repository's root, from this file's place in it. */
func repoRoot() -> URL {
    URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("../../../../..").standardized
}
