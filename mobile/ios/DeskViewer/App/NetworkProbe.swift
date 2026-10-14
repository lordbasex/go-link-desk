// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import Network

/**
 Test my connection: the connection test (desk.go-link.org/network) with
 what only the app sees, after "#d=" (docs/network.md): this device's
 address on its network and whether it is on Wi-Fi or mobile data (which
 is almost always the provider's CGNAT). The fragment is never sent to a
 server.
 */
enum NetworkProbe {
    static func url(web: String = "https://desk.go-link.org") async -> URL? {
        let kind = await networkKind()
        var info: [String: Any] = ["src": "ios", "net": kind]
        if let lan = localIPv4(cellular: kind == "cellular") { info["lan"] = lan }
        guard let json = try? JSONSerialization.data(withJSONObject: info, options: [.sortedKeys]) else { return nil }
        let b64 = json.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
        return URL(string: web + "/network#d=" + b64)
    }

    /** wifi, cellular, ethernet or other, from the current path. */
    static func networkKind() async -> String {
        await withCheckedContinuation { done in
            let monitor = NWPathMonitor()
            let once = Once()
            monitor.pathUpdateHandler = { path in
                guard once.first() else { return }
                monitor.cancel()
                if path.usesInterfaceType(.wifi) { done.resume(returning: "wifi") }
                else if path.usesInterfaceType(.cellular) { done.resume(returning: "cellular") }
                else if path.usesInterfaceType(.wiredEthernet) { done.resume(returning: "ethernet") }
                else { done.resume(returning: "other") }
            }
            monitor.start(queue: DispatchQueue(label: "network-probe"))
        }
    }

    /** The IPv4 address of Wi-Fi (en0) or mobile data (pdp_ip0). */
    static func localIPv4(cellular: Bool) -> String? {
        var list: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&list) == 0, let first = list else { return nil }
        defer { freeifaddrs(list) }
        let wanted = cellular ? "pdp_ip0" : "en0"
        for p in sequence(first: first, next: { $0.pointee.ifa_next }) {
            let ifa = p.pointee
            guard let addr = ifa.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET),
                  String(cString: ifa.ifa_name) == wanted else { continue }
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            if getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 {
                return String(cString: host)
            }
        }
        return nil
    }
}

/** True the first time only, from any thread. */
private final class Once: @unchecked Sendable {
    private var done = false
    private let lock = NSLock()
    func first() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if done { return false }
        done = true
        return true
    }
}
