// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

final class SignalUrlTests: XCTestCase {
    func testOnlyWss() {
        XCTAssertEqual(.ok("wss://signal.example.org/ws"), SignalUrls.check(" wss://Signal.Example.org/ws "))
        XCTAssertEqual(.ok("wss://signal.example.org:8443/"), SignalUrls.check("wss://signal.example.org:8443"))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("ws://127.0.0.1:8090/ws"))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("https://signal.example.org/ws"))
        XCTAssertNotEqual(SignalUrls.check("wss://").isOk, true)
        XCTAssertNotEqual(SignalUrls.check("wss://user:pass@signal.example.org/ws").isOk, true)
        XCTAssertNotEqual(SignalUrls.check("not a url").isOk, true)
    }

    func testResolve() {
        XCTAssertEqual(SignalUrls.Choice(url: DeskProtocol.officialSignalURL, custom: false), SignalUrls.resolve(nil))
        XCTAssertEqual(SignalUrls.Choice(url: DeskProtocol.officialSignalURL, custom: false), SignalUrls.resolve("ws://evil.example/ws"))
        XCTAssertEqual(SignalUrls.Choice(url: "wss://mine.example/ws", custom: true), SignalUrls.resolve("wss://mine.example/ws"))
    }

    func testDebugBuildsReachTheDevelopmentMachine() {
        // Release (the default): ws:// is refused even for local hosts.
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("ws://localhost:8191/ws"))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("ws://127.0.0.1:8191/ws"))
        // Debug: ws:// only to localhost and 127.0.0.1.
        XCTAssertEqual(.ok("ws://localhost:8090/ws"), SignalUrls.check("WS://LocalHost:8090/ws", allowDevHosts: true))
        XCTAssertEqual(.ok("ws://127.0.0.1:8090/ws?v=1"), SignalUrls.check("ws://127.0.0.1:8090/ws?v=1", allowDevHosts: true))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("ws://signal.example.org/ws", allowDevHosts: true))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("ws://10.0.2.2/ws", allowDevHosts: true))
        XCTAssertEqual(.bad(.scheme), SignalUrls.check("http://127.0.0.1:8191/ws", allowDevHosts: true))
        XCTAssertNotEqual(SignalUrls.check("ws://user:pass@127.0.0.1/ws", allowDevHosts: true).isOk, true)
        // wss:// keeps working as always.
        XCTAssertEqual(.ok("wss://signal.example.org/ws"), SignalUrls.check("wss://signal.example.org/ws", allowDevHosts: true))
    }

    func testResolveKeepsALocalServerOnlyInDebug() {
        XCTAssertEqual(SignalUrls.Choice(url: DeskProtocol.officialSignalURL, custom: false), SignalUrls.resolve("ws://127.0.0.1:8191/ws"))
        XCTAssertEqual(SignalUrls.Choice(url: "ws://127.0.0.1:8191/ws", custom: true), SignalUrls.resolve("ws://127.0.0.1:8191/ws", allowDevHosts: true))
        XCTAssertEqual(SignalUrls.Choice(url: DeskProtocol.officialSignalURL, custom: false), SignalUrls.resolve("ws://evil.example/ws", allowDevHosts: true))
    }

    func testEndpointAddsTheVersion() {
        XCTAssertEqual("wss://signal.go-link.org/ws?v=1", endpoint("wss://signal.go-link.org/ws"))
        XCTAssertEqual("wss://a.example/ws?x=1&v=1", endpoint("wss://a.example/ws?x=1"))
        XCTAssertEqual("wss://a.example/ws?v=2", endpoint("wss://a.example/ws?v=2"))
    }

    func testIceServersFromHello() {
        let hello = obj("""
        {"type":"hello","peer_id":"p","ice_servers":[
          {"urls":["stun:signal.example.org:3478"]},
          {"urls":"turn:signal.example.org:3478?transport=udp","username":"u","credential":"c"},
          {"urls":["https://evil.example"]},
          "junk"]}
        """)
        let servers = IceServers.parse(hello["ice_servers"])
        XCTAssertEqual(2, servers.count)
        XCTAssertEqual(IceServer(urls: ["turn:signal.example.org:3478?transport=udp"], username: "u", credential: "c"), servers[1])
    }
}

extension SignalUrlCheck {
    var isOk: Bool {
        if case .ok = self { return true }
        return false
    }
}
