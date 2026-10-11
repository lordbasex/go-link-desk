// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/** A STUN or TURN server from signalhub's hello (same shape as RTCIceServer). */
public struct IceServer: Equatable, Sendable {
    public let urls: [String]
    public let username: String?
    public let credential: String?

    public init(urls: [String], username: String? = nil, credential: String? = nil) {
        self.urls = urls
        self.username = username
        self.credential = credential
    }
}

public enum IceServers {
    /**
     * The STUN/TURN servers of a hello, checked: at most 10, each with
     * stun:/turn:/turns: URLs and string credentials. Anything else is
     * dropped. They live in memory only and are taken again on every
     * reconnect (TURN credentials expire).
     */
    public static func parse(_ v: JSON?) -> [IceServer] {
        v.arr.prefix(10).compactMap { s -> IceServer? in
            guard case let .object(o) = s else { return nil }
            let raw = o["urls"]
            let list: [String]
            if case let .string(one)? = raw {
                list = [one]
            } else {
                list = raw.arr.compactMap { Optional($0).strOrNil }
            }
            let urls = Array(list.filter { isIceURL($0) && $0.count <= 256 }.prefix(10))
            if urls.isEmpty { return nil }
            return IceServer(
                urls: urls,
                username: o["username"].strOrNil.map { String($0.prefix(256)) },
                credential: o["credential"].strOrNil.map { String($0.prefix(256)) }
            )
        }
    }

    private static func isIceURL(_ s: String) -> Bool {
        let l = s.lowercased()
        return l.hasPrefix("stun:") || l.hasPrefix("turn:") || l.hasPrefix("turns:")
    }
}
