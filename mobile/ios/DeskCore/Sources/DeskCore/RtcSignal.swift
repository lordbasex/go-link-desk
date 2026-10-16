// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// What travels inside signalhub's "signal" payload between the app and the
// computer (docs/protocol.md). The app knocks; the person at the computer
// accepts or not; the computer then offers and the app answers. signalhub
// never reads it.

/** An ICE candidate as the web's RTCIceCandidateInit. */
public struct IceCandidateInit: Equatable, Sendable {
    public let candidate: String
    public let sdpMid: String?
    public let sdpMLineIndex: Int

    public init(candidate: String, sdpMid: String?, sdpMLineIndex: Int) {
        self.candidate = candidate
        self.sdpMid = sdpMid
        self.sdpMLineIndex = sdpMLineIndex
    }
}

/** What the computer lets this viewer do. Seeing the screen always. */
public struct Permissions: Equatable, Sendable {
    public var control = false
    public var audio = false
    public var clipboard = false
    public var files = false
    /** Screenshots and recordings, each asked of the computer first. */
    public var record = false

    public init(control: Bool = false, audio: Bool = false, clipboard: Bool = false, files: Bool = false, record: Bool = false) {
        self.control = control
        self.audio = audio
        self.clipboard = clipboard
        self.files = files
        self.record = record
    }

    static func parse(_ v: JSON?) -> Permissions {
        let o = v.obj
        return Permissions(control: o["control"].isTrue, audio: o["audio"].isTrue, clipboard: o["clipboard"].isTrue, files: o["files"].isTrue,
                           record: o["record"].isTrue)
    }
}

/** What the computer says through signalhub. */
public enum HostSignal: Equatable, Sendable {
    case accepted(Permissions)
    /** declined, timeout, busy, unavailable or off. */
    case rejected(String)
    case offer(String)
    case candidate(IceCandidateInit)
}

public enum RtcSignals {
    static let rejectReasons: Set<String> = ["declined", "timeout", "busy", "unavailable", "off"]

    public static func parse(_ payload: JSON?) -> HostSignal? {
        guard case let .object(o)? = payload else { return nil }
        switch o["kind"].str(20) {
        case "accepted":
            return .accepted(Permissions.parse(o["permissions"]))
        case "rejected":
            let r = o["reason"].str(20)
            return .rejected(rejectReasons.contains(r) ? r : "declined")
        case "offer":
            guard let sdp = o["sdp"].strOrNil, !sdp.isEmpty else { return nil }
            return .offer(sdp)
        case "candidate":
            guard case let .object(c)? = o["candidate"], let text = c["candidate"].strOrNil else { return nil }
            return .candidate(IceCandidateInit(
                candidate: String(text.prefix(1000)),
                sdpMid: c["sdpMid"].strOrNil.map { String($0.prefix(32)) },
                // Clamp to a small non-negative range: the value is relayed
                // through the (untrusted) signaling server, and a huge one
                // would otherwise trap when narrowed to Int32 for WebRTC.
                sdpMLineIndex: c["sdpMLineIndex"].int.clamped(0, 255)
            ))
        default:
            return nil
        }
    }

    /**
     * The knock: who asks (a name the person at the computer sees, and the
     * app and system as "ua") and the video codecs this side decodes.
     * Cut as the host cuts them (40 and 60 characters).
     */
    public static func knock(name: String, ua: String, codecs: [String]) -> JSON {
        .obj(
            ("kind", "knock"),
            ("name", .string(String(name.trimmingCharacters(in: .whitespacesAndNewlines).prefix(40)))),
            ("ua", .string(String(ua.prefix(60)))),
            ("codecs", .array(codecs.map { .string($0) }))
        )
    }

    public static func answer(_ sdp: String) -> JSON { .obj(("kind", "answer"), ("sdp", .string(sdp))) }

    public static func candidate(_ c: IceCandidateInit) -> JSON {
        .obj(("kind", "candidate"), ("candidate", .obj(
            ("candidate", .string(c.candidate)),
            ("sdpMid", c.sdpMid.map { .string($0) }),
            ("sdpMLineIndex", .int(c.sdpMLineIndex))
        )))
    }
}
