// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// Which signaling server the app uses: the official one, or one the person
// typed in Settings. It is only changed by hand (after a test that the
// server answers hello), never from a link or a QR code, so nobody can send
// a session to a hostile server.
//
// Debug builds (and only they: the app passes allowDevHosts only in DEBUG)
// also accept ws:// to the development machine, like the website's
// loopback exception: localhost and 127.0.0.1 (the iOS Simulator reaches
// the Mac it runs on through them).

public enum SignalUrlCheck: Equatable, Sendable {
    case ok(String)
    case bad(Problem)

    public enum Problem: Equatable, Sendable { case format, scheme }
}

public enum SignalUrls {
    /** Hosts a debug build may reach over plain ws:// (the developer's own machine). */
    public static let devHosts: Set<String> = ["localhost", "127.0.0.1"]

    /**
     * Accepts only wss:// URLs with a host (the app talks to the internet:
     * always encrypted). With allowDevHosts (debug builds only), ws:// to
     * one of devHosts is accepted too.
     */
    public static func check(_ raw: String, allowDevHosts: Bool = false) -> SignalUrlCheck {
        let text = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.isEmpty || text.count > 300 { return .bad(.format) }
        guard let c = URLComponents(string: text) else { return .bad(.format) }
        let scheme = c.scheme?.lowercased()
        let host = c.host?.lowercased() ?? ""
        let devWs = allowDevHosts && scheme == "ws" && devHosts.contains(host)
        if scheme != "wss" && !devWs { return .bad(.scheme) }
        if host.isEmpty || c.user != nil || c.password != nil { return .bad(.format) }
        let path = c.percentEncodedPath.isEmpty ? "/" : c.percentEncodedPath
        let port = c.port.map { ":\($0)" } ?? ""
        let query = c.percentEncodedQuery.map { "?\($0)" } ?? ""
        return .ok("\(scheme!)://\(host)\(port)\(path)\(query)")
    }

    /** The server to use: a valid stored custom one, else the official one. */
    public static func resolve(_ stored: String?, allowDevHosts: Bool = false) -> Choice {
        if let stored, case let .ok(url) = check(stored, allowDevHosts: allowDevHosts), url != DeskProtocol.officialSignalURL {
            return Choice(url: url, custom: true)
        }
        return Choice(url: DeskProtocol.officialSignalURL, custom: false)
    }

    public struct Choice: Equatable, Sendable {
        public let url: String
        public let custom: Bool

        public init(url: String, custom: Bool) {
            self.url = url
            self.custom = custom
        }
    }
}
