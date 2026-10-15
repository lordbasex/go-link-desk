// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Linking with the pairing code, and coming back to a linked computer (docs/protocol.md, "Unattended access"). */
@OptIn(ExperimentalCoroutinesApi::class)
class LinkTest {
    private val link = DeviceLink("D", "L", "secret-token")

    private fun Rig.prove(token: String) {
        peer.events.onControlOpen()
        val nonce = parseObject(peer.control.last())!!["nonce"].str()
        control("""{"type":"auth_proof","link_id":"L","proof":"${DeskSession.linkProof(token, nonce)}"}""")
    }

    @Test
    fun theProofMatchesTheSharedVector() {
        val root = parseObject(repoFile("protocol/vectors/link-proof.json").readText())!!
        for (c in root["cases"].arr()) {
            val o = c.obj()
            assertEquals(o["proof"].str(), DeskSession.linkProof(o["token"].str(), o["nonce"].str()))
        }
        for (c in root["bound_cases"].arr()) {
            val o = c.obj()
            assertEquals(o["proof2"].str(), DeskSession.linkProofBound(o["token"].str(), o["nonce"].str(), o["host_fingerprint"].str(), o["viewer_fingerprint"].str()))
        }
        for (c in root["fingerprint_cases"].arr()) {
            val o = c.obj()
            assertEquals(o["fingerprint"].str(), DeskSession.fingerprint(o["sdp"].str(10000)))
        }
        assertTrue(DeskSession.same("abc", "abc"))
        assertFalse(DeskSession.same("abc", "abd"))
        assertTrue(DeskSession.randomNonce().length >= 40)
    }

    @Test
    fun claimLinksThisDevice() = runTest {
        val r = Rig(this, Door.Claim("123456789"), "Ana")
        r.receive("""{"type":"hello","peer_id":"me"}""")
        assertEquals(obj("""{"type":"claim","app":"go-link-desk","code":"123456789"}"""), r.conn.sent.last())
        var linked: DeviceLink? = null
        r.session.events.linked = { linked = it }
        r.receive("""{"type":"paired","remote":"H"}""")
        assertEquals(SessionStatus.Waiting, r.session.status)
        r.fromHost("""{"kind":"accepted","permissions":{"control":true}}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        assertEquals("""{"type":"auth"}""", r.peer.control.last())
        r.control("""{"type":"auth_ok","device_id":"D","link_id":"L","token":"T"}""")
        assertEquals(DeviceLink("D", "L", "T"), linked)
    }

    @Test
    fun reachProvesTheComputerBeforeTheToken() = runTest {
        val r = Rig(this, Door.Reach(link), "Ana")
        r.receive("""{"type":"hello","peer_id":"me"}""")
        assertEquals(obj("""{"type":"reach","app":"go-link-desk","device_id":"D"}"""), r.conn.sent.last())
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        val peer = r.peer
        peer.events.onControlOpen()
        val challenge = parseObject(peer.control.last())!!
        assertEquals("auth_challenge", challenge["type"].str())
        val nonce = challenge["nonce"].str()
        // accepted may overtake auth_ok: it waits.
        r.fromHost("""{"kind":"accepted","permissions":{"control":true}}""")
        assertEquals(SessionStatus.Waiting, r.session.status)
        assertFalse(peer.control.any { "secret-token" in it })
        r.control("""{"type":"auth_proof","link_id":"L","proof":"${DeskSession.linkProof("secret-token", nonce)}"}""")
        assertEquals("""{"type":"auth","link_id":"L","token":"secret-token"}""", peer.control.last())
        r.control("""{"type":"auth_ok","link_id":"L"}""")
        assertEquals(SessionStatus.Connecting, r.session.status) // the early acceptance applies now
    }

    /** Sends the computer's proofs to a reaching session; returns the rig and whether the token went. */
    private fun kotlinx.coroutines.test.TestScope.proofs(l: DeviceLink, proof2Host: String?, bound: (() -> Unit)? = null): Pair<Rig, Boolean> {
        val r = Rig(this, Door.Reach(l), "Ana")
        r.session.events.bound = bound
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        // The fake answer repeats the offer, so both sides show "sha-256 HO:ST".
        r.fromHost("""{"kind":"offer","sdp":"v=0\r\na=fingerprint:sha-256 ho:st\r\n"}""")
        testScheduler.advanceUntilIdle()
        r.peer.events.onControlOpen()
        val nonce = parseObject(r.peer.control.last())!!["nonce"].str()
        var m = """{"type":"auth_proof","link_id":"L","proof":"${DeskSession.linkProof("secret-token", nonce)}""""
        if (proof2Host != null) m += ""","proof2":"${DeskSession.linkProofBound("secret-token", nonce, proof2Host, "sha-256 HO:ST")}""""
        r.control("$m}")
        return r to (r.peer.control.last() == """{"type":"auth","link_id":"L","token":"secret-token"}""")
    }

    @Test
    fun theProofBoundToTheConnection() = runTest {
        var bound = 0
        assertTrue(proofs(link, "sha-256 HO:ST") { bound++ }.second)
        assertEquals(1, bound)
        // A computer from before proof2 still works, until one gave it.
        assertTrue(proofs(link, null).second)
        // Relayed from another connection: someone in the middle has its own certificate.
        val (relayed, sent) = proofs(link, "sha-256 MI:TM")
        assertFalse(sent)
        assertEquals(SessionStatus.Untrusted, relayed.session.status)
        // Once bound, the plain proof alone is refused.
        val (plain, sentPlain) = proofs(link.copy(bound = true), null)
        assertFalse(sentPlain)
        assertEquals(SessionStatus.Untrusted, plain.session.status)
    }

    @Test
    fun aWrongProofSendsNothing() = runTest {
        val r = Rig(this, Door.Reach(link), "Ana")
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        r.control("""{"type":"auth_proof","link_id":"L","proof":"forged"}""")
        assertEquals(SessionStatus.Untrusted, r.session.status)
        assertFalse(r.peer.control.any { "secret-token" in it })
    }

    @Test
    fun theSavedPasswordGoesByItself() = runTest {
        val r = Rig(this, Door.Reach(DeviceLink("D", "L", "t")), "Ana")
        val s = r.session
        s.savedPassword = "kept one"
        var accepted: String? = null
        var rejected = false
        s.events.passwordAccepted = { accepted = it }
        s.events.passwordRejected = { rejected = true }
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.prove("t")
        r.control("""{"type":"auth_ok","link_id":"L"}""")
        r.control("""{"type":"password_required"}""")
        assertEquals("""{"type":"password","password":"kept one"}""", r.peer.control.last())
        // Changed on the computer: forgotten here, and asked.
        r.control("""{"type":"password_result","ok":false,"reason":"wrong","left":4}""")
        assertTrue(rejected)
        assertEquals(SessionStatus.Password(PasswordPrompt(wrong = true, left = 4, checking = false)), s.status)
        s.sendPassword("new one")
        r.control("""{"type":"password_result","ok":true}""")
        assertEquals("new one", accepted)
    }

    @Test
    fun passwordAndUnlinked() = runTest {
        val l = DeviceLink("D", "L", "t")
        val r = Rig(this, Door.Reach(l), "Ana")
        val s = r.session
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.prove("t")
        r.control("""{"type":"auth_ok","link_id":"L"}""")
        r.control("""{"type":"password_required"}""")
        assertEquals(SessionStatus.Password(PasswordPrompt()), s.status)
        s.sendPassword("nope")
        assertEquals("""{"type":"password","password":"nope"}""", r.peer.control.last())
        r.control("""{"type":"password_result","ok":false,"reason":"wrong","left":4}""")
        assertEquals(SessionStatus.Password(PasswordPrompt(wrong = true, left = 4, checking = false)), s.status)
        r.control("""{"type":"password_result","ok":true}""")
        assertEquals(SessionStatus.Waiting, s.status)

        val u = Rig(this, Door.Reach(l), "Ana")
        var forgot = false
        u.session.events.unlinked = { forgot = true }
        u.receive("""{"type":"hello","peer_id":"me"}""")
        u.receive("""{"type":"paired","remote":"H"}""")
        u.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        u.control("""{"type":"auth_failed"}""")
        assertTrue(forgot)
        assertEquals(SessionStatus.Unlinked, u.session.status)

        val off = Rig(this, Door.Reach(l), "Ana")
        off.receive("""{"type":"hello","peer_id":"me"}""")
        off.receive("""{"type":"error","error":"device not connected"}""")
        assertEquals(SessionStatus.Offline, off.session.status)
    }
}
