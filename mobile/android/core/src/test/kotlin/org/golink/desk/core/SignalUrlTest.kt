// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SignalUrlTest {
    @Test
    fun onlyWss() {
        assertEquals(SignalUrlCheck.Ok("wss://signal.example.org/ws"), SignalUrls.check(" wss://Signal.Example.org/ws "))
        assertEquals(SignalUrlCheck.Ok("wss://signal.example.org:8443/"), SignalUrls.check("wss://signal.example.org:8443"))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("ws://127.0.0.1:8090/ws"))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("https://signal.example.org/ws"))
        assertTrue(SignalUrls.check("wss://") is SignalUrlCheck.Bad)
        assertTrue(SignalUrls.check("wss://user:pass@signal.example.org/ws") is SignalUrlCheck.Bad)
        assertTrue(SignalUrls.check("not a url") is SignalUrlCheck.Bad)
    }

    @Test
    fun resolve() {
        assertEquals(SignalUrls.Choice(Protocol.OFFICIAL_SIGNAL_URL, false), SignalUrls.resolve(null))
        assertEquals(SignalUrls.Choice(Protocol.OFFICIAL_SIGNAL_URL, false), SignalUrls.resolve("ws://evil.example/ws"))
        assertEquals(SignalUrls.Choice("wss://mine.example/ws", true), SignalUrls.resolve("wss://mine.example/ws"))
    }

    @Test
    fun debugBuildsReachTheDevelopmentMachine() {
        // Release (the default): ws:// is refused even for local hosts.
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("ws://10.0.2.2:8191/ws"))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("ws://localhost:8191/ws"))
        // Debug: ws:// only to localhost, 127.0.0.1 and the emulator's 10.0.2.2.
        assertEquals(SignalUrlCheck.Ok("ws://10.0.2.2:8191/ws"), SignalUrls.check("ws://10.0.2.2:8191/ws", allowDevHosts = true))
        assertEquals(SignalUrlCheck.Ok("ws://localhost:8090/ws"), SignalUrls.check("WS://LocalHost:8090/ws", allowDevHosts = true))
        assertEquals(SignalUrlCheck.Ok("ws://127.0.0.1:8090/ws?v=1"), SignalUrls.check("ws://127.0.0.1:8090/ws?v=1", allowDevHosts = true))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("ws://signal.example.org/ws", allowDevHosts = true))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("ws://10.0.2.3/ws", allowDevHosts = true))
        assertEquals(SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME), SignalUrls.check("http://10.0.2.2:8191/ws", allowDevHosts = true))
        assertTrue(SignalUrls.check("ws://user:pass@10.0.2.2/ws", allowDevHosts = true) is SignalUrlCheck.Bad)
        // wss:// keeps working as always.
        assertEquals(SignalUrlCheck.Ok("wss://signal.example.org/ws"), SignalUrls.check("wss://signal.example.org/ws", allowDevHosts = true))
    }

    @Test
    fun resolveKeepsALocalServerOnlyInDebug() {
        assertEquals(SignalUrls.Choice(Protocol.OFFICIAL_SIGNAL_URL, false), SignalUrls.resolve("ws://10.0.2.2:8191/ws"))
        assertEquals(SignalUrls.Choice("ws://10.0.2.2:8191/ws", true), SignalUrls.resolve("ws://10.0.2.2:8191/ws", allowDevHosts = true))
        assertEquals(SignalUrls.Choice(Protocol.OFFICIAL_SIGNAL_URL, false), SignalUrls.resolve("ws://evil.example/ws", allowDevHosts = true))
    }

    @Test
    fun endpointAddsTheVersion() {
        assertEquals("wss://signal.go-link.org/ws?v=1", endpoint("wss://signal.go-link.org/ws"))
        assertEquals("wss://a.example/ws?x=1&v=1", endpoint("wss://a.example/ws?x=1"))
        assertEquals("wss://a.example/ws?v=2", endpoint("wss://a.example/ws?v=2"))
    }

    @Test
    fun iceServersFromHello() {
        val hello = parseObject(
            """{"type":"hello","peer_id":"p","ice_servers":[
              {"urls":["stun:signal.example.org:3478"]},
              {"urls":"turn:signal.example.org:3478?transport=udp","username":"u","credential":"c"},
              {"urls":["https://evil.example"]},
              "junk"]}""",
        )!!
        val servers = IceServers.parse(hello["ice_servers"])
        assertEquals(2, servers.size)
        assertEquals(IceServer(listOf("turn:signal.example.org:3478?transport=udp"), "u", "c"), servers[1])
    }
}
