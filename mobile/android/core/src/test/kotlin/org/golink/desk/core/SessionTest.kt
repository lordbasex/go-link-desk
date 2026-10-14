// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** A session against a fake signalhub and fake peers, moved by hand. */
@OptIn(ExperimentalCoroutinesApi::class)
class Rig(private val test: TestScope, door: Door = Door.Join("915355636"), name: String = "  Ana  ") {
    val sockets = FakeSockets()
    val peers = FakePeers()
    val statuses = mutableListOf<SessionStatus>()
    val session: DeskSession

    init {
        val signal = SignalClient("wss://signal.example/ws", sockets, test.backgroundScope, random = kotlin.random.Random(1))
        session = DeskSession(door, name, "go-link Desk 1.0 on Android", signal, peers, test.backgroundScope)
        session.events.status = { statuses += it }
        session.start()
        test.runCurrent()
    }

    val conn get() = sockets.last
    val peer get() = peers.created[0]

    fun receive(text: String) {
        conn.receive(text)
        test.runCurrent()
    }

    fun hello() = receive("""{"type":"hello","peer_id":"me","ice_servers":[{"urls":["stun:s.example:3478"]}]}""")

    fun fromHost(payload: String) = receive("""{"type":"signal","from":"H","payload":$payload}""")

    fun control(text: String) {
        peer.events.onControlMessage(text)
        test.runCurrent()
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class SessionTest {
    @Test
    fun joinKnockAcceptAnswerAndLive() = runTest {
        val r = Rig(this)
        val s = r.session
        assertEquals("wss://signal.example/ws?v=1", r.conn.url)
        r.hello()
        assertEquals(obj("""{"type":"join","app":"go-link-desk","code":"915355636"}"""), r.conn.sent.last())
        r.receive("""{"type":"joined","remote":"H","room_id":"R"}""")
        assertEquals(SessionStatus.Waiting, s.status)
        assertEquals(
            """{"type":"signal","to":"H","payload":{"kind":"knock","name":"Ana","ua":"go-link Desk 1.0 on Android","codecs":["h264-high","h264","vp8"]}}""",
            r.conn.sentText.last(),
        )

        // A candidate before the offer waits for the peer.
        r.fromHost("""{"kind":"candidate","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}""")
        r.fromHost("""{"kind":"accepted","permissions":{"view":true,"control":true,"clipboard":true}}""")
        assertEquals(SessionStatus.Connecting, s.status)
        assertTrue(s.permissions.control)
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        val peer = r.peer
        assertEquals(listOf("stun:s.example:3478"), r.peers.servers[0].first().urls)
        assertEquals(listOf("v=0"), peer.offers)
        assertEquals(listOf("c1"), peer.candidates.map { it.candidate })
        assertEquals(obj("""{"kind":"answer","sdp":"answer-for-v=0"}"""), r.conn.sent.last()["payload"])

        // Control waits for its channel.
        s.setVideoWant(2000, 1000)
        assertTrue(peer.control.isEmpty())
        peer.events.onControlOpen()
        assertEquals(listOf("""{"type":"video_want","width":2000,"height":1000}"""), peer.control)
        peer.events.onState(PeerState.CONNECTED)
        assertEquals(SessionStatus.Live, s.status)

        // The computer's messages.
        var displays = emptyList<Display>()
        s.events.displays = { d, _ -> displays = d }
        r.control("""{"type":"welcome","host":{"name":"Mac","os":"darwin"}}""")
        assertEquals("darwin", s.hostOS)
        r.control("""{"type":"displays","displays":[{"id":7,"name":"A","width":100,"height":50},{"id":9,"name":"B","width":10,"height":5}],"current":9}""")
        assertEquals(listOf(7, 9), displays.map { it.id })
        val typing = mutableListOf<Boolean>()
        s.events.typing = { typing += it }
        r.control("""{"type":"typing","on":true}""")
        r.control("""{"type":"typing","on":false}""")
        assertEquals(listOf(true, false), typing)
        s.sendTyping(true)
        assertEquals("""{"type":"typing","on":true}""", peer.control.last())
        r.control("""{"type":"ping","id":4}""")
        assertEquals("""{"type":"pong","id":4}""", peer.control.last())

        // Input goes with the current display's index.
        s.pointer(0.5, 1.0)
        assertEquals(hex(InputPacket.pointer(1, 1, 32768, 65535)), hex(peer.pointer.last()))
        s.button(InputPacket.Button.RIGHT, true, 0.0, 0.0)
        assertEquals(hex(InputPacket.buttonEvent(InputPacket.Button.RIGHT, true, 1, 0, 0)), hex(peer.input.last()))

        // Without control, nothing goes.
        r.control("""{"type":"permissions","permissions":{"view":true}}""")
        s.pointer(0.0, 0.0)
        assertEquals(1, peer.pointer.size)

        s.hangUp()
        assertEquals("""{"type":"session_end"}""", peer.control.last())
        assertEquals(SessionStatus.Ended("viewer"), s.status)
        assertTrue(peer.closed)
        assertTrue(r.conn.closed)
        assertEquals(
            listOf(SessionStatus.Joining, SessionStatus.Waiting, SessionStatus.Connecting, SessionStatus.Live, SessionStatus.Ended("viewer")),
            r.statuses,
        )
    }

    @Test
    fun rejectedAndUnknownCode() = runTest {
        val r = Rig(this)
        r.hello()
        r.receive("""{"type":"joined","remote":"H"}""")
        r.fromHost("""{"kind":"rejected","reason":"busy"}""")
        assertEquals(SessionStatus.Rejected("busy"), r.session.status)

        val t = Rig(this)
        t.hello()
        t.receive("""{"type":"error","error":"invalid or expired code"}""")
        assertEquals(SessionStatus.NotFound, t.session.status)

        val u = Rig(this)
        u.hello()
        u.receive("""{"type":"error","error":"rate limit exceeded"}""")
        assertEquals(SessionStatus.RateLimited, u.session.status)
    }

    @Test
    fun noAnswerFromTheServer() = runTest {
        val r = Rig(this)
        advanceTimeBy(DeskSession.JOIN_TIMEOUT_MS + 1)
        assertEquals(SessionStatus.Unreachable, r.session.status)
    }

    @Test
    fun signalsFromSomeoneElseAreIgnored() = runTest {
        val r = Rig(this)
        r.hello()
        r.receive("""{"type":"joined","remote":"H"}""")
        r.receive("""{"type":"signal","from":"X","payload":{"kind":"rejected"}}""")
        assertEquals(SessionStatus.Waiting, r.session.status)
        r.receive("""{"type":"peer_left","from":"H"}""")
        assertEquals(SessionStatus.Ended("signal"), r.session.status)
    }

    @Test
    fun theHostEndsTheSession() = runTest {
        val r = Rig(this)
        r.hello()
        r.receive("""{"type":"joined","remote":"H"}""")
        r.fromHost("""{"kind":"accepted","permissions":{}}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.control("""{"type":"session_end","reason":"idle"}""")
        assertEquals(SessionStatus.Ended("idle"), r.session.status)
    }

    @Test
    fun clipboardInPiecesBothWays() = runTest {
        val r = Rig(this)
        val s = r.session
        r.hello()
        r.receive("""{"type":"joined","remote":"H"}""")
        r.fromHost("""{"kind":"accepted","permissions":{"clipboard":true}}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        val peer = r.peer
        peer.events.onControlOpen()
        val got = mutableListOf<String>()
        s.events.clipboard = { got += it }
        r.control("""{"type":"clipboard","id":3,"part":0,"parts":2,"text":"ho"}""")
        r.control("""{"type":"clipboard","id":3,"part":1,"parts":2,"text":"la"}""")
        assertEquals(listOf("hola"), got)

        val big = "ñ".repeat(10_000) // 20000 bytes: two pieces
        assertTrue(s.sendClipboard(big, paste = true))
        val sent = peer.control.mapNotNull { parseObject(it) }.filter { it["type"] == JsonPrimitive("clipboard") }
        assertEquals(2, sent.size)
        assertNull(sent[0]["paste"])
        assertEquals(JsonPrimitive(true), sent[1]["paste"])
        assertEquals(big, sent.joinToString("") { it["text"].str(1 shl 20) })
        assertFalse(s.sendChat("   "))
    }

    @Test
    fun screenshotsAndRecordingAskTheComputerFirst() = runTest {
        val r = Rig(this)
        val s = r.session
        r.hello()
        r.receive("""{"type":"joined","remote":"H"}""")
        r.fromHost("""{"kind":"accepted","permissions":{"view":true}}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        val peer = r.peer
        peer.events.onControlOpen()
        val answers = mutableListOf<Boolean>()
        s.askScreenshot { answers += it }
        assertEquals(listOf(false), answers) // no permission: nothing asked
        assertTrue(peer.control.isEmpty())

        r.control("""{"type":"permissions","permissions":{"view":true,"record":true}}""")
        s.askScreenshot { answers += it }
        assertEquals("""{"type":"screenshot","id":1}""", peer.control.last())
        r.control("""{"type":"screenshot","id":1,"ok":true}""")
        assertEquals(listOf(false, true), answers)

        s.startRecording { answers += it }
        assertEquals("""{"type":"recording","on":true,"id":2}""", peer.control.last())
        assertFalse(s.recording)
        r.control("""{"type":"recording","id":2,"on":true}""")
        assertEquals(true, answers.last())
        assertTrue(s.recording)

        // The person at the computer unticks it: the recording stops here.
        var stopped = 0
        s.events.recordingStopped = { stopped++ }
        r.control("""{"type":"permissions","permissions":{"view":true}}""")
        assertFalse(s.recording)
        assertEquals(1, stopped)

        // No answer in time is a no, and a late yes is turned down.
        r.control("""{"type":"permissions","permissions":{"view":true,"record":true}}""")
        s.startRecording { answers += it }
        advanceTimeBy(DeskSession.ASK_TIMEOUT_MS + 1)
        assertEquals(false, answers.last())
        r.control("""{"type":"recording","id":3,"on":true}""")
        assertFalse(s.recording)
        assertEquals("""{"type":"recording","id":0,"on":false}""", peer.control.last())
    }
}
