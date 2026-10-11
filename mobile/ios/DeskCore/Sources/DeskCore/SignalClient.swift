// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// MARK: - Timers

/** Something that can be stopped: a timer, a listener. */
public protocol CancelHandle: AnyObject {
    func cancel()
}

/**
 * Runs code later on the main actor. The app uses MainScheduler; the tests
 * use a manual clock, so timing (reconnects, input repeats) is exact.
 */
@MainActor
public protocol Scheduler: AnyObject {
    @discardableResult
    func after(ms: Int, _ fn: @escaping @MainActor () -> Void) -> CancelHandle
}

/** The real clock: the main queue. */
@MainActor
public final class MainScheduler: Scheduler {
    public static let shared = MainScheduler()

    private final class Item: CancelHandle {
        let work: DispatchWorkItem
        init(_ work: DispatchWorkItem) { self.work = work }
        func cancel() { work.cancel() }
    }

    public init() {}

    @discardableResult
    public func after(ms: Int, _ fn: @escaping @MainActor () -> Void) -> CancelHandle {
        let work = DispatchWorkItem { MainActor.assumeIsolated { fn() } }
        DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(max(0, ms)), execute: work)
        return Item(work)
    }
}

// MARK: - Sockets

/** A WebSocket the platform opens (URLSessionWebSocketTask on iOS). */
@MainActor
public protocol SignalSocket: AnyObject {
    @discardableResult
    func send(_ text: String) -> Bool
    func close()
}

/** Socket events. The platform delivers them on the main actor. */
@MainActor
public protocol SignalSocketListener: AnyObject {
    func onMessage(_ text: String)
    func onClosed()
}

@MainActor
public protocol SignalSocketFactory: AnyObject {
    /** Opens a socket, or nil when it cannot even start. */
    func open(url: String, listener: SignalSocketListener) -> SignalSocket?
}

/** A listener made of two closures. */
@MainActor
final class ClosureSocketListener: SignalSocketListener {
    let message: (String) -> Void
    let closed: () -> Void

    init(message: @escaping (String) -> Void, closed: @escaping () -> Void) {
        self.message = message
        self.closed = closed
    }

    func onMessage(_ text: String) { message(text) }
    func onClosed() { closed() }
}

public enum ConnectionState: Sendable { case connecting, open, closed }

/** An error reply from signalhub (fromServer), or a local failure. */
public struct SignalError: Error, Equatable, Sendable {
    public let message: String
    public let fromServer: Bool

    public init(_ message: String, fromServer: Bool) {
        self.message = message
        self.fromServer = fromServer
    }
}

/**
 * Client for signalhub, a port of the web's SignalClient
 * (frontend/packages/shared/src/signal-client.ts). It reconnects with jittered exponential backoff, keeps
 * the ICE servers from hello in memory only, and offers request() for
 * messages that expect one reply. Replies arrive in the order requests were
 * sent (signalhub handles one connection's messages in order), so pending
 * requests form a FIFO queue.
 *
 * Everything runs on the main actor. Native clients send no Origin
 * header, which signalhub accepts.
 */
@MainActor
public final class SignalClient {
    private final class Pending {
        let expect: Set<String>
        var done: ((Result<Envelope, SignalError>) -> Void)?
        var timer: CancelHandle?

        init(expect: Set<String>, done: @escaping (Result<Envelope, SignalError>) -> Void) {
            self.expect = expect
            self.done = done
        }

        func finish(_ r: Result<Envelope, SignalError>) {
            timer?.cancel()
            let d = done
            done = nil
            d?(r)
        }
    }

    private final class Token: CancelHandle {
        let fn: () -> Void
        init(_ fn: @escaping () -> Void) { self.fn = fn }
        func cancel() { fn() }
    }

    public let url: String
    private let factory: SignalSocketFactory
    private let scheduler: Scheduler
    private let minBackoffMs: Int
    private let maxBackoffMs: Int
    private let requestTimeoutMs: Int
    private let random: () -> Double

    private var socket: SignalSocket?
    private var generation = 0
    private var stopped = true
    private var backoff: Int
    private var retryTimer: CancelHandle?
    private var pending: [Pending] = []
    private var listeners: [(Int, (Envelope) -> Void)] = []
    private var stateListeners: [(Int, (ConnectionState) -> Void)] = []
    private var openWaiters: [(Int, () -> Void)] = []
    private var nextId = 0

    public private(set) var state: ConnectionState = .closed {
        didSet {
            if state != oldValue {
                for (_, fn) in stateListeners { fn(state) }
            }
        }
    }

    /** peer_id assigned in hello; empty while disconnected. */
    public private(set) var peerId = ""

    /** STUN/TURN servers from hello, for the next peer connection. */
    public private(set) var iceServers: [IceServer] = []

    public init(
        url: String,
        factory: SignalSocketFactory,
        scheduler: Scheduler,
        minBackoffMs: Int = 1000,
        maxBackoffMs: Int = 30000,
        requestTimeoutMs: Int = 10000,
        random: @escaping () -> Double = { Double.random(in: 0..<1) }
    ) {
        self.url = url
        self.factory = factory
        self.scheduler = scheduler
        self.minBackoffMs = minBackoffMs
        self.maxBackoffMs = maxBackoffMs
        self.requestTimeoutMs = requestTimeoutMs
        self.random = random
        backoff = minBackoffMs
    }

    /** Starts connecting (and reconnecting) until close() is called. */
    public func connect() {
        guard stopped else { return }
        stopped = false
        open()
    }

    /** Closes the link for good and fails pending requests. */
    public func close() {
        stopped = true
        retryTimer?.cancel()
        retryTimer = nil
        let s = socket
        socket = nil
        generation += 1
        s?.close()
        handleDown()
    }

    /**
     * Drops the connection and opens a new one right away. A signalhub
     * connection belongs to at most one session, so this is how the app
     * leaves a session.
     */
    public func reset() {
        close()
        backoff = minBackoffMs
        connect()
    }

    /** Listens to every message; the result stops listening. */
    @discardableResult
    public func addListener(_ fn: @escaping (Envelope) -> Void) -> CancelHandle {
        nextId += 1
        let id = nextId
        listeners.append((id, fn))
        return Token { [weak self] in self?.listeners.removeAll { $0.0 == id } }
    }

    /** Listens to connection state changes; the result stops listening. */
    @discardableResult
    public func addStateListener(_ fn: @escaping (ConnectionState) -> Void) -> CancelHandle {
        nextId += 1
        let id = nextId
        stateListeners.append((id, fn))
        return Token { [weak self] in self?.stateListeners.removeAll { $0.0 == id } }
    }

    /** Sends one message; false while disconnected. */
    @discardableResult
    public func send(_ env: JSON) -> Bool {
        guard state == .open, let socket else { return false }
        return socket.send(env.text)
    }

    /**
     * Sends env and calls done with the next reply whose type is in
     * expect. An "error" reply fails with the server's text.
     */
    public func request(_ env: JSON, expect: Set<String>, done: @escaping (Result<Envelope, SignalError>) -> Void) {
        if state != .open {
            awaitOpen { [weak self] ok in
                guard let self else { return }
                if ok { self.request(env, expect: expect, done: done) } else { done(.failure(SignalError("signaling server unreachable", fromServer: false))) }
            }
            return
        }
        let entry = Pending(expect: expect, done: done)
        pending.append(entry)
        guard send(env) else {
            pending.removeAll { $0 === entry }
            entry.finish(.failure(SignalError("not connected", fromServer: false)))
            return
        }
        entry.timer = scheduler.after(ms: requestTimeoutMs) { [weak self] in
            self?.pending.removeAll { $0 === entry }
            entry.finish(.failure(SignalError("request timed out", fromServer: false)))
        }
    }

    /** request() for async code. */
    public func request(_ env: JSON, expect: Set<String>) async throws -> Envelope {
        try await withCheckedThrowingContinuation { cont in
            request(env, expect: expect) { cont.resume(with: $0) }
        }
    }

    /** Calls done(true) once the link is open (hello arrived), or done(false) on timeout. */
    public func awaitOpen(timeoutMs: Int? = nil, _ done: @escaping (Bool) -> Void) {
        if state == .open {
            done(true)
            return
        }
        nextId += 1
        let id = nextId
        var timer: CancelHandle?
        openWaiters.append((id, {
            timer?.cancel()
            done(true)
        }))
        timer = scheduler.after(ms: timeoutMs ?? requestTimeoutMs) { [weak self] in
            guard let self, self.openWaiters.contains(where: { $0.0 == id }) else { return }
            self.openWaiters.removeAll { $0.0 == id }
            done(false)
        }
    }

    private func open() {
        state = .connecting
        generation += 1
        let gen = generation
        let listener = ClosureSocketListener(
            message: { [weak self] text in
                guard let self, gen == self.generation else { return }
                self.handleRaw(text)
            },
            closed: { [weak self] in
                guard let self, gen == self.generation else { return } // a stale socket after reset()
                self.socket = nil
                self.handleDown()
                self.scheduleRetry()
            }
        )
        let s = factory.open(url: endpoint(url), listener: listener)
        if gen != generation { return } // closed while opening
        socket = s
        if s == nil { scheduleRetry() }
    }

    private func handleRaw(_ text: String) {
        guard let env = Envelope.parse(text) else { return }
        if env.type == "hello" {
            peerId = env.peerId
            iceServers = IceServers.parse(env.raw["ice_servers"])
            backoff = minBackoffMs
            state = .open
            let waiters = openWaiters
            openWaiters = []
            for (_, fn) in waiters { fn() }
            return
        }
        if let head = pending.first, env.type == "error" || head.expect.contains(env.type) {
            pending.removeFirst()
            if env.type == "error" {
                head.finish(.failure(SignalError(env.error.isEmpty ? "unknown error" : env.error, fromServer: true)))
            } else {
                head.finish(.success(env))
            }
        }
        for (_, fn) in listeners { fn(env) }
    }

    private func handleDown() {
        peerId = ""
        iceServers = []
        let failed = pending
        pending = []
        for p in failed { p.finish(.failure(SignalError("disconnected", fromServer: false))) }
        state = stopped ? .closed : .connecting
    }

    private func scheduleRetry() {
        guard !stopped else { return }
        // Full jitter: many clients do not reconnect at the same instant.
        let wait = Int(random() * Double(backoff) + Double(minBackoffMs) / 2)
        backoff = min(backoff * 2, maxBackoffMs)
        retryTimer?.cancel()
        retryTimer = scheduler.after(ms: wait) { [weak self] in
            guard let self else { return }
            self.retryTimer = nil
            if !self.stopped { self.open() }
        }
    }

    /**
     * Opens a throwaway connection and waits for hello. Used before saving
     * a custom signaling server. done gets nil when it answered, or a short
     * reason.
     */
    public static func test(
        url: String,
        factory: SignalSocketFactory,
        scheduler: Scheduler,
        timeoutMs: Int = 5000,
        done: @escaping (String?) -> Void
    ) {
        var finished = false
        var socket: SignalSocket?
        var timer: CancelHandle?
        var closeLater = false
        func finish(_ outcome: String?) {
            guard !finished else { return }
            finished = true
            timer?.cancel()
            if let socket { socket.close() } else { closeLater = true }
            done(outcome)
        }
        let listener = ClosureSocketListener(
            message: { text in
                if JSONText.parseObject(text)?["type"].strOrNil == "hello" { finish(nil) }
            },
            closed: { finish("could not connect") }
        )
        socket = factory.open(url: endpoint(url), listener: listener)
        guard let s = socket else {
            finish("could not connect")
            return
        }
        if closeLater {
            s.close()
            return
        }
        if !finished {
            timer = scheduler.after(ms: timeoutMs) { finish("no answer from the server") }
        }
    }

    /** test() for async code. */
    public static func test(url: String, factory: SignalSocketFactory, scheduler: Scheduler, timeoutMs: Int = 5000) async -> String? {
        await withCheckedContinuation { cont in
            test(url: url, factory: factory, scheduler: scheduler, timeoutMs: timeoutMs) { cont.resume(returning: $0) }
        }
    }
}
