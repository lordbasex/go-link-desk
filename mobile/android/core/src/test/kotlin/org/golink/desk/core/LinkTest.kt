// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** A key store for the tests: software keys, no Android Keystore. */
class TestKeys : LinkKeys {
    val signers = HashMap<String, SoftwareLinkKey>()
    var made: SoftwareLinkKey? = null

    fun register(linkId: String): SoftwareLinkKey = SoftwareLinkKey().also { signers[linkId] = it }

    override fun make(): MadeKey {
        val k = SoftwareLinkKey()
        made = k
        return MadeKey(k.publicKey, k) { id -> signers[id] = k }
    }

    override fun signer(linkId: String): LinkSigner? = signers[linkId]
}

/** Linking with the pairing code, and coming back to a linked computer (docs/protocol.md, "Unattended access"). */
@OptIn(ExperimentalCoroutinesApi::class)
class LinkTest {
    private val hostKey = SoftwareLinkKey()
    private fun link() = DeviceLink("D", "L", hostKey.publicKey)
    private fun hostProof(nonce: String, host: String = "", viewer: String = "") =
        hostKey.sign(LinkAuth.hostProofMessage(nonce, host, viewer))

    @Test
    fun theProofMatchesTheSharedVector() {
        val root = parseObject(repoFile("protocol/vectors/link-proof.json").readText())!!
        val nonce = root["nonce"].str()
        val h = root["host_fingerprint"].str()
        val v = root["viewer_fingerprint"].str()
        // ECDSA is randomized, so verify the signatures against the public
        // keys rather than reproducing the bytes.
        assertTrue(LinkAuth.verify(root["host_pub"].str(200), LinkAuth.hostProofMessage(nonce, h, v), root["host_proof_sig"].str(200)))
        assertTrue(LinkAuth.verify(root["link_pub"].str(200), LinkAuth.linkProofMessage(nonce, h, v), root["link_proof_sig"].str(200)))
        assertTrue(LinkAuth.verify(root["host_pub"].str(200), LinkAuth.hostUnlinkMessage(nonce, h, v), root["host_unlink_sig"].str(200)))
        assertFalse(LinkAuth.verify(root["link_pub"].str(200), LinkAuth.hostProofMessage(nonce, h, v), root["host_proof_sig"].str(200)))
        for (c in root["fingerprint_cases"].arr()) {
            val o = c.obj()
            assertEquals(o["fingerprint"].str(), DeskSession.fingerprint(o["sdp"].str(10000)))
        }
        assertTrue(DeskSession.randomNonce().length >= 40)
    }

    @Test
    fun claimLinksThisDevice() = runTest {
        val keys = TestKeys()
        val r = Rig(this, Door.Claim("123456789"), "Ana", keys)
        r.receive("""{"type":"hello","peer_id":"me"}""")
        assertEquals(obj("""{"type":"claim","app":"go-link-desk","code":"123456789"}"""), r.conn.sent.last())
        var linked: DeviceLink? = null
        r.session.events.linked = { linked = it }
        r.receive("""{"type":"paired","remote":"H"}""")
        assertEquals(SessionStatus.Waiting, r.session.status)
        r.fromHost("""{"kind":"accepted","permissions":{"control":true}}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        val auth = parseObject(r.peer.control.last())!!
        assertEquals("auth", auth["type"].str())
        assertTrue(auth["pub"].str(200).isNotEmpty())
        r.control("""{"type":"auth_ok","device_id":"D","link_id":"L","host_key":"${hostKey.publicKey}"}""")
        assertEquals(DeviceLink("D", "L", hostKey.publicKey), linked)
        assertNotNull(keys.signer("D")) // kept under the id it got
    }

    @Test
    fun reachProvesTheComputerBeforeTheToken() = runTest {
        val keys = TestKeys().also { it.register("L") }
        val r = Rig(this, Door.Reach(link()), "Ana", keys)
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
        r.control("""{"type":"auth_proof","link_id":"L","sig":"${hostProof(nonce)}"}""")
        val auth = parseObject(peer.control.last())!!
        assertEquals("auth", auth["type"].str())
        assertTrue(auth["sig"].str(200).isNotEmpty())
        r.control("""{"type":"auth_ok","link_id":"L"}""")
        assertEquals(SessionStatus.Connecting, r.session.status) // the early acceptance applies now
    }

    /** Sends the computer's proof to a reaching session; returns the rig and whether it proved back. */
    private fun kotlinx.coroutines.test.TestScope.prove(signHost: String): Pair<Rig, Boolean> {
        val keys = TestKeys().also { it.register("L") }
        val r = Rig(this, Door.Reach(link()), "Ana", keys)
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        // The fake answer repeats the offer, so both sides show "sha-256 HO:ST".
        r.fromHost("""{"kind":"offer","sdp":"v=0\r\na=fingerprint:sha-256 ho:st\r\n"}""")
        testScheduler.advanceUntilIdle()
        r.peer.events.onControlOpen()
        val nonce = parseObject(r.peer.control.last())!!["nonce"].str()
        r.control("""{"type":"auth_proof","link_id":"L","sig":"${hostProof(nonce, signHost, "sha-256 HO:ST")}"}""")
        val last = parseObject(r.peer.control.last())!!
        return r to (last["type"].str() == "auth" && last["sig"].str(200).isNotEmpty())
    }

    @Test
    fun theProofBoundToTheConnection() = runTest {
        // Bound to this connection's fingerprints: proved back.
        assertTrue(prove("sha-256 HO:ST").second)
        // Relayed from another connection: the computer signed over the
        // fingerprints it saw, not what this viewer sees, so it does not
        // verify and nothing is proved back.
        val (relayed, sent) = prove("sha-256 MI:TM")
        assertFalse(sent)
        assertEquals(SessionStatus.Untrusted, relayed.session.status)
    }

    @Test
    fun aWrongProofSendsNothing() = runTest {
        val keys = TestKeys().also { it.register("L") }
        val r = Rig(this, Door.Reach(link()), "Ana", keys)
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        r.control("""{"type":"auth_proof","link_id":"L","sig":"forged"}""")
        assertEquals(SessionStatus.Untrusted, r.session.status)
        assertFalse(r.peer.control.any { it.contains(""""type":"auth"""") && it.contains("sig") })
    }

    @Test
    fun theSavedPasswordGoesByItself() = runTest {
        val keys = TestKeys().also { it.register("L") }
        val r = Rig(this, Door.Reach(link()), "Ana", keys)
        val s = r.session
        s.savedPassword = "kept one"
        var accepted: String? = null
        var rejected = false
        s.events.passwordAccepted = { accepted = it }
        s.events.passwordRejected = { rejected = true }
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        val nonce = parseObject(r.peer.control.last())!!["nonce"].str()
        r.control("""{"type":"auth_proof","link_id":"L","sig":"${hostProof(nonce)}"}""")
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
        val keys = TestKeys().also { it.register("L") }
        val r = Rig(this, Door.Reach(link()), "Ana", keys)
        val s = r.session
        r.receive("""{"type":"hello","peer_id":"me"}""")
        r.receive("""{"type":"paired","remote":"H"}""")
        r.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        r.peer.events.onControlOpen()
        val nonce = parseObject(r.peer.control.last())!!["nonce"].str()
        r.control("""{"type":"auth_proof","link_id":"L","sig":"${hostProof(nonce)}"}""")
        r.control("""{"type":"auth_ok","link_id":"L"}""")
        r.control("""{"type":"password_required"}""")
        assertEquals(SessionStatus.Password(PasswordPrompt()), s.status)
        s.sendPassword("nope")
        assertEquals("""{"type":"password","password":"nope"}""", r.peer.control.last())
        r.control("""{"type":"password_result","ok":false,"reason":"wrong","left":4}""")
        assertEquals(SessionStatus.Password(PasswordPrompt(wrong = true, left = 4, checking = false)), s.status)
        r.control("""{"type":"password_result","ok":true}""")
        assertEquals(SessionStatus.Waiting, s.status)

        // A revoked link: before any proof, an unsigned refusal is ignored; a
        // refusal the computer signed is honored.
        val u = Rig(this, Door.Reach(link()), "Ana", TestKeys().also { it.register("L") })
        var forgot = false
        u.session.events.unlinked = { forgot = true }
        u.receive("""{"type":"hello","peer_id":"me"}""")
        u.receive("""{"type":"paired","remote":"H"}""")
        u.fromHost("""{"kind":"offer","sdp":"v=0"}""")
        u.peer.events.onControlOpen()
        val un = parseObject(u.peer.control.last())!!["nonce"].str()
        u.control("""{"type":"auth_failed"}""")
        assertFalse(forgot)
        assertFalse(u.session.status == SessionStatus.Unlinked)
        u.control("""{"type":"auth_failed","sig":"${hostKey.sign(LinkAuth.hostUnlinkMessage(un, "", ""))}"}""")
        assertTrue(forgot)
        assertEquals(SessionStatus.Unlinked, u.session.status)

        val off = Rig(this, Door.Reach(link()), "Ana", TestKeys().also { it.register("L") })
        off.receive("""{"type":"hello","peer_id":"me"}""")
        off.receive("""{"type":"error","error":"device not connected"}""")
        assertEquals(SessionStatus.Offline, off.session.status)
    }
}
