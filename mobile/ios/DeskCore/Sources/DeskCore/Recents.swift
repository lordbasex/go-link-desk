// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/** A computer this app connected to with its code, for Home's list. */
public struct Recent: Codable, Equatable, Sendable, Identifiable {
    public var code: String
    public var name: String
    public var os: String
    public var at: Date

    public var id: String { code }

    public init(code: String, name: String, os: String, at: Date) {
        self.code = code
        self.name = name
        self.os = os
        self.at = at
    }
}

public enum Recents {
    public static let max = 12

    /** Puts r first, without its older entry, keeping at most max. */
    public static func add(_ r: Recent, to list: [Recent]) -> [Recent] {
        Array(([r] + list.filter { $0.code != r.code }).prefix(max))
    }

    /** The 9 digits of what was typed, or nil. */
    public static func digits(_ typed: String) -> String? {
        let d = typed.filter { ("0"..."9").contains($0) }
        let rest = typed.filter { !("0"..."9").contains($0) && !$0.isWhitespace && $0 != "-" && $0 != "." }
        return d.count == CodeInput.digits && rest.isEmpty ? d : nil
    }

    /** The system's name, as Home shows it. */
    public static func systemName(_ os: String) -> String {
        switch os {
        case "darwin": return "macOS"
        case "windows": return "Windows"
        case "linux": return "Linux"
        default: return os
        }
    }
}

/** The numbers the details panel shows and the loss the computer is told about. */
public struct LiveStats: Equatable, Sendable {
    public var fps: Double = 0
    public var width = 0
    public var height = 0
    public var kbps = 0
    public var lossPct: Double = 0
    public var rttMs: Int?
    /** "direct" or "relay", once known. */
    public var path: String?

    public init() {}
}

/** Turns successive cumulative counters into rates. */
public struct StatsMeter: Sendable {
    public struct Sample: Sendable {
        public var atMs: Double
        public var framesDecoded: Int64?
        public var bytesReceived: Int64?
        public var packetsReceived: Int64?
        public var packetsLost: Int64?
        public var width: Int?
        public var height: Int?
        public var rttSeconds: Double?
        public var localType: String?
        public var remoteType: String?

        public init(atMs: Double) { self.atMs = atMs }
    }

    private var last: Sample?

    public init() {}

    public mutating func next(_ s: Sample) -> LiveStats {
        var out = LiveStats()
        out.width = s.width ?? 0
        out.height = s.height ?? 0
        out.rttMs = s.rttSeconds.map { Int(($0 * 1000).rounded()) }
        if let l = s.localType, let r = s.remoteType {
            out.path = l == "relay" || r == "relay" ? "relay" : "direct"
        }
        if let p = last, s.atMs > p.atMs {
            let dt = (s.atMs - p.atMs) / 1000
            if let a = s.framesDecoded, let b = p.framesDecoded, a >= b { out.fps = Double(a - b) / dt }
            if let a = s.bytesReceived, let b = p.bytesReceived, a >= b { out.kbps = Int(Double(a - b) * 8 / 1000 / dt) }
            if let rx = s.packetsReceived, let prx = p.packetsReceived, let lost = s.packetsLost, let plost = p.packetsLost {
                let got = max(0, rx - prx), missed = max(0, lost - plost)
                if got + missed > 0 { out.lossPct = Double(missed) * 100 / Double(got + missed) }
            }
        }
        last = s
        return out
    }
}
