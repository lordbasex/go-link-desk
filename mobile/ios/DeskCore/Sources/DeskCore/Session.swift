// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import CryptoKit
import Foundation

// One visit to a computer with its session code, the iOS port of the Mac
// app's viewer (backend-device/internal/viewer) and the website's
// session (frontend/apps/web/src/session/session.ts): join the computer's
// room, knock, wait for the person there to accept, answer the offer, and
// then speak the control, pointer and input channels. The computer cannot
// tell it from the website.

/** The app's side of the WebRTC link: libwebrtc in the app, a fake in the tests. */
@MainActor
public protocol RtcPeer: AnyObject {
    /** Applies the computer's offer and returns the answer's SDP. */
    func answer(offerSdp: String, done: @escaping (Result<String, Error>) -> Void)
    func addCandidate(_ candidate: IceCandidateInit)
    /** JSON text on the reliable "control" channel. */
    @discardableResult func sendControl(_ text: String) -> Bool
    /** A pointer position on the "pointer" channel (unordered: only the newest counts). */
    @discardableResult func sendPointer(_ packet: [UInt8]) -> Bool
    /** A button, wheel, key or text packet on the "input" channel. */
    @discardableResult func sendInput(_ packet: [UInt8]) -> Bool
    func close()
}

public enum PeerState: Sendable { case connected, disconnected, failed }

/** Peer events, delivered on the main actor. */
@MainActor
public protocol RtcPeerEvents: AnyObject {
    func onLocalCandidate(_ candidate: IceCandidateInit)
    func onState(_ state: PeerState)
    func onControlOpen()
    func onControlMessage(_ text: String)
}

@MainActor
public protocol RtcPeerFactory: AnyObject {
    func create(iceServers: [IceServer], events: RtcPeerEvents) -> RtcPeer
}

/** A computer linked before: its id, the link's id and this device's secret for it. */
public struct DeviceLink: Equatable, Sendable {
    public let deviceId: String
    public let linkId: String
    public let token: String

    public init(deviceId: String, linkId: String, token: String) {
        self.deviceId = deviceId
        self.linkId = linkId
        self.token = token
    }
}

/** How a session reaches its computer. */
public enum Door: Equatable, Sendable {
    /** The 9-digit session code: the person there accepts this time. */
    case join(String)
    /** The computer's pairing code: the person there accepts, and this device is linked. */
    case claim(String)
    /** A computer linked before: nobody accepts (unattended access). */
    case reach(DeviceLink)
}

/** The unattended password the computer asks for. */
public struct PasswordPrompt: Equatable, Sendable {
    /** The last try was wrong, with this many tries left. */
    public var wrong = false
    public var left = 0
    /** A password is being checked. */
    public var checking = false
}

/** Where a session is, from the knock to its end. */
public enum SessionStatus: Equatable, Sendable {
    case joining
    /** The person at the computer decides. */
    case waiting
    case connecting
    case live
    case reconnecting
    /** A linked computer asks for its unattended password. */
    case password(PasswordPrompt)
    /** declined, timeout, busy, unavailable or off. */
    case rejected(String)
    /** host, viewer, network, signal or idle. */
    case ended(String)
    case notFound
    case rateLimited
    case unreachable
    /** The linked computer is not on. */
    case offline
    /** The computer no longer has this link: it was forgotten here. */
    case unlinked
    /** Whoever answered could not prove it is the linked computer. Nothing was sent to it. */
    case untrusted
    /** Too many wrong passwords: blocked (this device) or locked (retryAfter seconds). */
    case passwordRefused(reason: String, retryAfter: Int)

    public var isOver: Bool {
        switch self {
        case .joining, .waiting, .connecting, .live, .reconnecting, .password: return false
        default: return true
        }
    }
}

/** One of the computer's screens. */
public struct Display: Equatable, Sendable {
    public let id: Int
    public let name: String
    public let width: Int
    public let height: Int
    /** Its size in points (the pointer's image is sized in them). */
    public let pointsWidth: Int
    public let pointsHeight: Int
    public let primary: Bool
}

/** What the computer says it sends (stream_stats). */
public struct Sent: Equatable, Sendable {
    public let fps: Double
    public let width: Int
    public let height: Int
    public let kbps: Int
    public let codec: String
}

/** The computer's pointer image: a PNG, its hot spot in the image's pixels, its size in the computer's points. */
public struct CursorShape: Equatable, Sendable {
    public let id: String
    public let png: Data
    public let hotX: Double
    public let hotY: Double
    public let width: Double
    public let height: Double
    public let pixels: Double
}

/** What the screen learns. Every closure is optional and runs on the main actor. */
@MainActor
public final class SessionEvents {
    public var status: ((SessionStatus) -> Void)?
    public var host: ((_ name: String, _ os: String) -> Void)?
    public var permissions: ((Permissions) -> Void)?
    public var displays: ((_ displays: [Display], _ current: Int) -> Void)?
    public var sent: ((Sent) -> Void)?
    public var notice: ((String) -> Void)?
    public var cursor: ((CursorShape) -> Void)?
    public var cursorPos: ((_ x: Double, _ y: Double, _ visible: Bool) -> Void)?
    public var clipboard: ((String) -> Void)?
    public var chat: ((String) -> Void)?
    /** Linked with the pairing code: keep it to come back without a code. */
    public var linked: ((DeviceLink) -> Void)?
    /** The computer no longer has this link: forget it. */
    public var unlinked: (() -> Void)?

    public init() {}
}

/** Video presets (docs/protocol.md, "Video"). */
public enum Preset: String, Sendable, CaseIterable { case speed, balanced, quality }

@MainActor
public final class DeskSession: RtcPeerEvents {
    public static let codecs = ["h264-high", "h264", "vp8"]
    static let joinTimeoutMs = 20_000
    static let maxChat = 2000

    public let events = SessionEvents()
    public private(set) var status: SessionStatus = .joining
    public private(set) var permissions = Permissions()
    public private(set) var hostOS = ""
    public private(set) var displays: [Display] = []
    public private(set) var currentDisplay = 0

    /** The display shown now, if the computer listed it. */
    public var display: Display? { displays.first { $0.id == currentDisplay } }

    private let door: Door
    private let name: String
    private let ua: String
    private let signal: SignalClient
    private let peers: RtcPeerFactory
    private let scheduler: Scheduler
    private var listener: CancelHandle?
    private var stateListener: CancelHandle?
    private var joinTimer: CancelHandle?
    private var host = ""
    private var asked = false
    private var admitted = false
    private var connected = false
    private var peer: RtcPeer?
    private var backlog: [HostSignal] = []
    private var pendingControl: [String] = []
    private var controlOpen = false
    private var clipSeq: UInt32 = 0
    private var clipIn = ClipboardAssembler()
    private var shapes: [String: CursorShape] = [:]
    private var pointerSeq: UInt16 = 0
    private var nonce = ""
    private var proven = false
    private var authed = false
    private var early: Permissions?

    /**
     * code: the 9 digits. name and ua: what the person at the computer
     * sees ("Ana", "go-link Desk 1.0 on iPadOS 27"). The session owns
     * signal: a signalhub connection of its own, closed at the end
     * (leaving signalhub is how a viewer leaves the room).
     */
    public convenience init(code: String, name: String, ua: String, signal: SignalClient, peers: RtcPeerFactory, scheduler: Scheduler) {
        self.init(door: .join(code), name: name, ua: ua, signal: signal, peers: peers, scheduler: scheduler)
    }

    public init(door: Door, name: String, ua: String, signal: SignalClient, peers: RtcPeerFactory, scheduler: Scheduler) {
        self.door = door
        self.name = name
        self.ua = ua
        self.signal = signal
        self.peers = peers
        self.scheduler = scheduler
    }

    public func start() {
        listener = signal.addListener { [weak self] env in self?.handle(env) }
        stateListener = signal.addStateListener { [weak self] state in
            guard let self else { return }
            if state == .open {
                self.ask()
            } else if state == .connecting && self.asked {
                // The room went with the connection.
                self.finish(.ended("signal"))
            }
        }
        joinTimer = scheduler.after(ms: DeskSession.joinTimeoutMs) { [weak self] in
            guard let self, self.status == .joining else { return }
            self.finish(.unreachable)
        }
        emit()
        signal.connect()
        if signal.state == .open { ask() }
    }

    /** Leaves: the computer is told, and the session ends. */
    public func hangUp() {
        guard !status.isOver else { return }
        control(.obj(("type", "session_end")))
        finish(.ended("viewer"))
    }

    // MARK: - Signaling

    private func ask() {
        guard !asked, !status.isOver else { return }
        asked = true
        switch door {
        case let .join(code): signal.send(Envelopes.join(code: code))
        case let .claim(code): signal.send(.obj(("type", "claim"), ("app", .string(DeskProtocol.app)), ("code", .string(code))))
        case let .reach(link): signal.send(.obj(("type", "reach"), ("app", .string(DeskProtocol.app)), ("device_id", .string(link.deviceId))))
        }
    }

    private func handle(_ env: Envelope) {
        switch env.type {
        case "joined", "paired":
            inRoom(env.remote)
        case "error":
            guard status == .joining else { return }
            switch env.error {
            case ServerError.rateLimited: finish(.rateLimited)
            case ServerError.deviceOffline: finish(.offline)
            default: finish(.notFound)
            }
        case "signal":
            guard !host.isEmpty, env.from == host, let s = RtcSignals.parse(env.payload) else { return }
            onSignal(s)
        case "peer_left":
            if !host.isEmpty && env.from == host { finish(.ended("signal")) }
        default:
            break
        }
    }

    private func inRoom(_ remote: String) {
        guard host.isEmpty, !status.isOver, !remote.isEmpty else { return }
        host = remote
        joinTimer?.cancel()
        set(.waiting)
        sendSignal(RtcSignals.knock(name: name, ua: ua, codecs: DeskSession.codecs))
        // A linked computer proves itself first: the token goes only to the
        // computer it was linked to (sent when the control channel opens).
        if case let .reach(link) = door {
            nonce = DeskSession.randomNonce()
            control(.obj(("type", "auth_challenge"), ("link_id", .string(link.linkId)), ("nonce", .string(nonce))))
        }
    }

    private func onSignal(_ s: HostSignal) {
        switch s {
        case let .accepted(p):
            guard !status.isOver else { return }
            // A linked computer lets this device in only once it proved
            // itself and took the token; its acceptance may overtake auth_ok.
            if case .reach = door, !authed {
                early = p
                return
            }
            admit(p)
        case let .rejected(reason):
            finish(.rejected(reason))
        case .offer:
            if peer == nil {
                peer = peers.create(iceServers: signal.iceServers, events: self)
            }
            apply(s)
            let b = backlog
            backlog = []
            for x in b { apply(x) }
        case .candidate:
            if peer == nil {
                if backlog.count < 50 { backlog.append(s) }
                return
            }
            apply(s)
        }
    }

    private func admit(_ p: Permissions) {
        admitted = true
        permissions = p
        set(connected ? .live : .connecting)
        events.permissions?(p)
        // Accepted with the pairing code: now ask for the link's token.
        if case .claim = door { control(.obj(("type", "auth"))) }
    }

    private func apply(_ s: HostSignal) {
        guard let peer else { return }
        switch s {
        case let .offer(sdp):
            peer.answer(offerSdp: sdp) { [weak self] result in
                guard let self, !self.status.isOver else { return }
                switch result {
                case let .success(answer): self.sendSignal(RtcSignals.answer(answer))
                case .failure: self.finish(.ended("network"))
                }
            }
        case let .candidate(c):
            peer.addCandidate(c)
        default:
            break
        }
    }

    private func sendSignal(_ payload: JSON) {
        guard !host.isEmpty else { return }
        signal.send(Envelopes.signal(to: host, payload: payload))
    }

    // MARK: - RtcPeerEvents

    public func onLocalCandidate(_ candidate: IceCandidateInit) {
        sendSignal(RtcSignals.candidate(candidate))
    }

    public func onState(_ state: PeerState) {
        guard !status.isOver else { return }
        connected = state == .connected
        guard admitted else {
            if state == .failed { finish(.ended("network")) }
            return
        }
        switch state {
        case .connected: set(.live)
        case .disconnected: set(.reconnecting)
        case .failed: finish(.ended("network"))
        }
    }

    public func onControlOpen() {
        controlOpen = true
        let p = pendingControl
        pendingControl = []
        for t in p { peer?.sendControl(t) }
    }

    public func onControlMessage(_ text: String) {
        guard text.utf8.count <= 64 << 10, let m = JSONText.parseObject(text) else { return }
        let o = JSON.object(m)
        switch m["type"].str(40) {
        case "ping":
            control(.obj(("type", "pong"), ("id", m["id"] ?? .null)))
        case "welcome":
            let h = m["host"].obj
            hostOS = JSON.object(h)["os"].str(16)
            events.host?(JSON.object(h)["name"].str(64), hostOS)
        case "stream_stats":
            events.sent?(Sent(fps: o["fps"].num, width: Int(o["width"].num), height: Int(o["height"].num),
                              kbps: Int(o["kbps"].num), codec: o["codec"].str(16)))
        case "displays":
            displays = m["displays"].arr.prefix(16).map { d in
                let x = Optional(d)
                return Display(id: Int(x?["id"].num ?? 0), name: x?["name"].str(64) ?? "", width: Int(x?["width"].num ?? 0),
                               height: Int(x?["height"].num ?? 0), pointsWidth: Int(x?["points_width"].num ?? 0),
                               pointsHeight: Int(x?["points_height"].num ?? 0), primary: x?["primary"].isTrue ?? false)
            }
            currentDisplay = Int(o["current"].num)
            events.displays?(displays, currentDisplay)
        case "permissions":
            permissions = Permissions.parse(m["permissions"])
            events.permissions?(permissions)
        case "notice":
            events.notice?(o["code"].str(40))
        case "cursor":
            cursor(m)
        case "cursor_pos":
            events.cursorPos?(o["x"].num, o["y"].num, o["visible"].isTrue)
        case "clipboard":
            let id = UInt32(max(0, o["id"].num))
            if let t = clipIn.add(id: id, part: Int(o["part"].num), parts: Int(o["parts"].num), text: o["text"].str(ClipboardAssembler.piece)),
               permissions.clipboard {
                events.clipboard?(t)
            }
        case "chat":
            let t = String(o["text"].str(DeskSession.maxChat))
            if !t.isEmpty { events.chat?(t) }
        case "session_end":
            let r = o["reason"].str(20)
            finish(.ended(["host", "viewer", "network", "signal", "idle", "revoked"].contains(r) ? r : "host"))
        default:
            linkMessage(m)
        }
    }

    /** The link's messages: the computer's proof, the token, the password. */
    private func linkMessage(_ m: JSONObject) {
        let o = JSON.object(m)
        switch m["type"].str(40) {
        case "auth_proof":
            guard case let .reach(link) = door, !nonce.isEmpty else { return }
            let proof = o["proof"].str(100)
            guard o["link_id"].str(64) == link.linkId, DeskSession.same(proof, DeskSession.linkProof(token: link.token, nonce: nonce)) else {
                finish(.untrusted)
                return
            }
            nonce = ""
            proven = true
            control(.obj(("type", "auth"), ("link_id", .string(link.linkId)), ("token", .string(link.token))))
        case "auth_ok":
            if case .claim = door {
                let device = o["device_id"].str(64), linkId = o["link_id"].str(64), token = o["token"].str(200)
                if !device.isEmpty, !linkId.isEmpty, !token.isEmpty {
                    events.linked?(DeviceLink(deviceId: device, linkId: linkId, token: token))
                }
                return
            }
            guard case let .reach(link) = door, proven, o["link_id"].str(64) == link.linkId else { return }
            authed = true
            if let p = early {
                early = nil
                admit(p)
            }
        case "auth_failed", "unlinked":
            if case .join = door { return }
            events.unlinked?()
            finish(.unlinked)
        case "password_required":
            guard case .reach = door, authed, !status.isOver else { return }
            set(.password(PasswordPrompt()))
        case "password_result":
            guard case let .password(prompt) = status else { return }
            if o["ok"].isTrue {
                set(.waiting)
                return
            }
            let reason = o["reason"].str(20)
            if reason == "blocked" || reason == "locked" {
                finish(.passwordRefused(reason: reason, retryAfter: Int(max(0, o["retry_after"].num))))
                return
            }
            var next = prompt
            next.wrong = true
            next.checking = false
            next.left = Int(max(0, o["left"].num))
            set(.password(next))
        default:
            break
        }
    }

    /** Sends the unattended password the computer asked for. */
    public func sendPassword(_ password: String) {
        guard case var .password(prompt) = status, !password.isEmpty else { return }
        prompt.checking = true
        set(.password(prompt))
        control(.obj(("type", "password"), ("password", .string(String(password.prefix(512))))))
    }

    /** The proof a linked computer gives: base64url of HMAC-SHA256 keyed with SHA-256(token) (protocol/vectors/link-proof.json). */
    static func linkProof(token: String, nonce: String) -> String {
        let key = SymmetricKey(data: SHA256.hash(data: Data(token.utf8)))
        let mac = HMAC<SHA256>.authenticationCode(for: Data(("go-link-desk link proof\n" + nonce).utf8), using: key)
        return Data(mac).base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }

    /** Compares two proofs in constant time. */
    static func same(_ a: String, _ b: String) -> Bool {
        let x = Array(a.utf8), y = Array(b.utf8)
        guard x.count == y.count else { return false }
        return zip(x, y).reduce(0) { $0 | ($1.0 ^ $1.1) } == 0
    }

    /** 32 random bytes, base64url. */
    static func randomNonce() -> String {
        var b = [UInt8](repeating: 0, count: 32)
        _ = SecRandomCopyBytes(kSecRandomDefault, b.count, &b)
        return Data(b).base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }

    private func cursor(_ m: JSONObject) {
        let o = JSON.object(m)
        guard let id = m["id"].strOrNil.map({ String($0.prefix(32)) }) else { return }
        var shape = shapes[id]
        if shape == nil, let b64 = m["png"].strOrNil, b64.count < 300 << 10, shapes.count < 64, let png = Data(base64Encoded: b64) {
            let s = CursorShape(id: id, png: png, hotX: o["hot_x"].num, hotY: o["hot_y"].num,
                                width: o["width"].num, height: o["height"].num, pixels: o["pixels"].num)
            shapes[id] = s
            shape = s
        }
        if let shape { events.cursor?(shape) }
    }

    // MARK: - To the computer

    /** The size the picture is shown at, in device pixels. */
    public func setVideoWant(width: Int, height: Int) {
        control(.obj(("type", "video_want"), ("width", .int(width)), ("height", .int(height))))
    }

    public func setQuality(_ p: Preset, retina: Bool = false) {
        control(.obj(("type", "quality"), ("preset", .string(p.rawValue)), ("retina", .bool(retina))))
    }

    public func selectDisplay(_ id: Int) {
        control(.obj(("type", "display_select"), ("id", .int(id))))
    }

    /** true: the computer draws its pointer into the picture; false: it sends its image and position. */
    public func setCursorInVideo(_ on: Bool) {
        control(.obj(("type", "cursor_mode"), ("in_video", .bool(on))))
    }

    /** The viewer's report every 2 seconds: video loss and whether the picture is hidden. */
    public func report(lossPct: Double, hidden: Bool) {
        control(.obj(("type", "client_report"), ("metrics", .obj(
            ("freeze_ms", 0), ("video_loss_pct", .number(lossPct)), ("jitter_ms", 0), ("hidden", .bool(hidden))
        ))))
    }

    public func sendChat(_ text: String) -> Bool {
        let t = String(text.trimmingCharacters(in: .whitespacesAndNewlines).prefix(DeskSession.maxChat))
        guard !t.isEmpty, !status.isOver else { return false }
        control(.obj(("type", "chat"), ("text", .string(t))))
        return true
    }

    /** Sends this device's clipboard; paste asks the computer to paste it where its cursor is. */
    @discardableResult
    public func sendClipboard(_ text: String, paste: Bool) -> Bool {
        guard permissions.clipboard, live, text.utf8.count <= ClipboardAssembler.maxText else { return false }
        clipSeq &+= 1
        for piece in ClipboardAssembler.pieces(id: clipSeq, text: text, paste: paste) { control(piece) }
        return true
    }

    /** The pointer at x, y (0..1 over the current display). */
    public func pointer(x: Double, y: Double) {
        guard canControl else { return }
        pointerSeq &+= 1
        peer?.sendPointer(InputPacket.pointer(seq: pointerSeq, display: displayIndex, x: InputPacket.coord(x), y: InputPacket.coord(y)))
    }

    public func button(_ b: InputPacket.Button, down: Bool, x: Double, y: Double) {
        guard canControl else { return }
        peer?.sendInput(InputPacket.buttonEvent(b, down: down, display: displayIndex, x: InputPacket.coord(x), y: InputPacket.coord(y)))
    }

    public func wheel(dx: Double, dy: Double) {
        guard canControl else { return }
        func c(_ v: Double) -> Int16 { Int16(v.rounded().clamped(-32767, 32767)) }
        peer?.sendInput(InputPacket.wheelEvent(dx: c(dx), dy: c(dy), lines: false))
    }

    public func input(_ packet: [UInt8]) {
        guard canControl else { return }
        peer?.sendInput(packet)
    }

    public var canControl: Bool { live && permissions.control }

    private var live: Bool { admitted && !status.isOver }

    /** The current display's index in the computer's list (what the input packets carry). */
    private var displayIndex: UInt8 {
        UInt8(displays.firstIndex { $0.id == currentDisplay } ?? 0)
    }

    private func control(_ msg: JSON) {
        let text = msg.text
        guard controlOpen, let peer else {
            if pendingControl.count < 20 { pendingControl.append(text) }
            return
        }
        peer.sendControl(text)
    }

    // MARK: - State

    private func set(_ s: SessionStatus) {
        guard status != s else { return }
        status = s
        emit()
    }

    private func emit() { events.status?(status) }

    private func finish(_ s: SessionStatus) {
        guard !status.isOver else { return }
        joinTimer?.cancel()
        status = s
        listener?.cancel()
        stateListener?.cancel()
        let p = peer
        peer = nil
        p?.close()
        signal.close()
        emit()
    }
}

/** The clipboard's pieces (docs/protocol.md, "clipboard"). */
struct ClipboardAssembler {
    static let piece = 16 << 10
    static let maxText = 1 << 20

    private var id: UInt32 = 0
    private var parts: [String] = []

    /** Takes a piece; the whole text once the last one arrives. */
    mutating func add(id: UInt32, part: Int, parts total: Int, text: String) -> String? {
        guard total >= 1, total <= ClipboardAssembler.maxText / ClipboardAssembler.piece + 1, part >= 0, part < total else { return nil }
        if part == 0 || id != self.id {
            self.id = id
            parts = []
        }
        guard part == parts.count else {
            parts = []
            return nil
        }
        parts.append(text)
        guard parts.count == total else { return nil }
        let out = parts.joined()
        parts = []
        return out.utf8.count <= ClipboardAssembler.maxText ? out : nil
    }

    /** A text cut into pieces of at most 16 KiB of UTF-8, never splitting a character. */
    static func pieces(id: UInt32, text: String, paste: Bool) -> [JSON] {
        var chunks: [String] = []
        var bytes = ArraySlice(Array(text.utf8))
        while bytes.count > piece {
            var cut = bytes.startIndex + piece
            while cut > bytes.startIndex && (bytes[cut] & 0xC0) == 0x80 { cut -= 1 }
            chunks.append(String(decoding: bytes[..<cut], as: UTF8.self))
            bytes = bytes[cut...]
        }
        chunks.append(String(decoding: bytes, as: UTF8.self))
        return chunks.enumerated().map { i, c in
            .obj(("type", "clipboard"), ("id", .number(Double(id))), ("part", .int(i)), ("parts", .int(chunks.count)),
                 ("text", .string(c)), ("paste", paste && i == chunks.count - 1 ? .bool(true) : nil))
        }
    }
}
