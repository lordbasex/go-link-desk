// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** A key store for the tests: software keys, no Secure Enclave. */
final class TestKeys: LinkKeys, @unchecked Sendable {
    var signers: [String: SoftwareLinkKey] = [:]
    var made: SoftwareLinkKey?

    func register(_ linkId: String) -> SoftwareLinkKey {
        let k = SoftwareLinkKey()
        signers[linkId] = k
        return k
    }

    func make() throws -> MadeKey {
        let k = SoftwareLinkKey()
        made = k
        return MadeKey(publicKey: k.publicKey, signer: k, persist: { id in self.signers[id] = k })
    }

    func signer(linkId: String) -> LinkSigner? { signers[linkId] }
}

/** Linking with the pairing code, and coming back to a linked computer (docs/protocol.md, "Unattended access"). */
@MainActor
final class LinkTests: XCTestCase {
    private var sockets: FakeSockets!
    private var clock: ManualScheduler!
    private var peers: FakePeers!
    private let keys = TestKeys()
    private let hostKey = SoftwareLinkKey()

    private func session(_ door: Door) -> DeskSession {
        sockets = FakeSockets()
        clock = ManualScheduler()
        peers = FakePeers()
        let signal = SignalClient(url: "wss://signal.example/ws", factory: sockets, scheduler: clock)
        let s = DeskSession(door: door, name: "Ana", ua: "go-link Desk on iOS", signal: signal, peers: peers, scheduler: clock, keys: keys)
        s.start()
        sockets.last.receive(#"{"type":"hello","peer_id":"me"}"#)
        return s
    }

    /** A reach link pinned to the test host key, with its viewer key registered. */
    private func reachLink() -> DeviceLink {
        DeviceLink(deviceId: "D", linkId: "L", hostKey: hostKey.publicKey)
    }

    private func hostProof(_ nonce: String, _ host: String = "", _ viewer: String = "") -> String {
        try! hostKey.sign(LinkAuth.hostProofMessage(nonce: nonce, host: host, viewer: viewer))
    }

    private func fromHost(_ payload: String) {
        sockets.last.receive(#"{"type":"signal","from":"H","payload":\#(payload)}"#)
    }

    func testTheProofMatchesTheSharedVector() throws {
        let data = try Data(contentsOf: repoRoot().appendingPathComponent("protocol/vectors/link-proof.json"))
        let root = try XCTUnwrap(JSONText.parseObject(String(decoding: data, as: UTF8.self)))
        let nonce = root["nonce"].str(), h = root["host_fingerprint"].str(), v = root["viewer_fingerprint"].str()
        // ECDSA is randomized, so verify the signatures against the public
        // keys rather than reproducing the bytes.
        XCTAssertTrue(LinkAuth.verify(publicKey: root["host_pub"].str(200), message: LinkAuth.hostProofMessage(nonce: nonce, host: h, viewer: v), sig: root["host_proof_sig"].str(200)))
        XCTAssertTrue(LinkAuth.verify(publicKey: root["link_pub"].str(200), message: LinkAuth.linkProofMessage(nonce: nonce, host: h, viewer: v), sig: root["link_proof_sig"].str(200)))
        XCTAssertTrue(LinkAuth.verify(publicKey: root["host_pub"].str(200), message: LinkAuth.hostUnlinkMessage(nonce: nonce, host: h, viewer: v), sig: root["host_unlink_sig"].str(200)))
        // The wrong key or message does not verify.
        XCTAssertFalse(LinkAuth.verify(publicKey: root["link_pub"].str(200), message: LinkAuth.hostProofMessage(nonce: nonce, host: h, viewer: v), sig: root["host_proof_sig"].str(200)))
        for c in root["fingerprint_cases"].arr {
            XCTAssertEqual(DeskSession.fingerprint(sdp: c["sdp"].str(10000)), c["fingerprint"].str())
        }
        XCTAssertGreaterThanOrEqual(DeskSession.randomNonce().count, 40)
    }

    func testClaimLinksThisDevice() {
        let s = session(.claim("123456789"))
        XCTAssertEqual(sockets.last.sent.last, obj(#"{"type":"claim","app":"go-link-desk","code":"123456789"}"#))
        var linked: DeviceLink?
        s.events.linked = { linked = $0 }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        XCTAssertEqual(s.status, .waiting)
        fromHost(#"{"kind":"accepted","permissions":{"control":true}}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        // The viewer sent its public key, not a request for a token.
        let auth = JSONText.parseObject(peer.control.last!)!
        XCTAssertEqual(auth["type"], "auth")
        XCTAssertFalse(auth["pub"].str().isEmpty)
        peer.events.onControlMessage(#"{"type":"auth_ok","device_id":"D","link_id":"L","host_key":"\#(hostKey.publicKey)"}"#)
        XCTAssertEqual(linked, DeviceLink(deviceId: "D", linkId: "L", hostKey: hostKey.publicKey))
        // The viewer's key was kept (the test store keys it by the id it got).
        XCTAssertNotNil(keys.signer(linkId: "D"))
    }

    func testReachProvesTheComputerBeforeTheToken() {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        XCTAssertEqual(sockets.last.sent.last, obj(#"{"type":"reach","app":"go-link-desk","device_id":"D"}"#))
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let challenge = JSONText.parseObject(peer.control.last!)!
        XCTAssertEqual(challenge["type"], "auth_challenge")
        let nonce = challenge["nonce"].str()
        // accepted may overtake auth_ok: it waits.
        fromHost(#"{"kind":"accepted","permissions":{"control":true}}"#)
        XCTAssertEqual(s.status, .waiting)
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","sig":"\#(hostProof(nonce))"}"#)
        let auth = JSONText.parseObject(peer.control.last!)!
        XCTAssertEqual(auth["type"], "auth")
        XCTAssertFalse(auth["sig"].str().isEmpty)
        peer.events.onControlMessage(#"{"type":"auth_ok","link_id":"L"}"#)
        XCTAssertEqual(s.status, .connecting) // the early acceptance applies now
    }

    /** Sends the computer's proof to a reaching session; returns whether this viewer proved back. */
    private func prove(signHost: String) -> (DeskSession, Bool) {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        // The fake answer repeats the offer, so both sides show "sha-256 HO:ST".
        fromHost(#"{"kind":"offer","sdp":"v=0\r\na=fingerprint:sha-256 ho:st\r\n"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","sig":"\#(hostProof(nonce, signHost, "sha-256 HO:ST"))"}"#)
        let last = JSONText.parseObject(peer.control.last!)!
        return (s, last["type"].str() == "auth" && !last["sig"].str().isEmpty)
    }

    func testTheProofBoundToTheConnection() {
        // Bound to this connection's fingerprints: proved back.
        XCTAssertTrue(prove(signHost: "sha-256 HO:ST").1)
        // Relayed from another connection: the computer signed over the
        // fingerprints it saw, not what this viewer sees, so it does not
        // verify and nothing is proved back.
        let (relayed, sent) = prove(signHost: "sha-256 MI:TM")
        XCTAssertFalse(sent)
        XCTAssertEqual(relayed.status, .untrusted)
    }

    // On a reach session, an unproven peer (a signaling MITM that finished
    // DTLS) cannot write the clipboard, inject chat or end the session: the
    // side-effecting control messages are dropped until the link proof and
    // auth_ok authenticate the computer.
    func testReachDropsSideEffectsBeforeAuth() {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        var clip: String?, chat: String?
        s.events.clipboard = { clip = $0 }
        s.events.chat = { chat = $0 }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        peer.events.onControlMessage(#"{"type":"permissions","permissions":{"clipboard":true}}"#)
        peer.events.onControlMessage(#"{"type":"clipboard","id":1,"part":0,"parts":1,"text":"evil"}"#)
        peer.events.onControlMessage(#"{"type":"chat","text":"fake"}"#)
        peer.events.onControlMessage(#"{"type":"session_end","reason":"host"}"#)
        XCTAssertNil(clip)
        XCTAssertNil(chat)
        XCTAssertNotEqual(s.status, .ended("host"))
    }

    func testAWrongProofSendsNothing() {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","sig":"forged"}"#)
        XCTAssertEqual(s.status, .untrusted)
        XCTAssertFalse(peer.control.contains { $0.contains(#""type":"auth","#) && $0.contains("sig") })
    }

    func testTheSavedPasswordGoesByItself() {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        s.savedPassword = "kept one"
        var accepted: String?, rejected = false
        s.events.passwordAccepted = { accepted = $0 }
        s.events.passwordRejected = { rejected = true }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","sig":"\#(hostProof(nonce))"}"#)
        peer.events.onControlMessage(#"{"type":"auth_ok","link_id":"L"}"#)
        peer.events.onControlMessage(#"{"type":"password_required"}"#)
        XCTAssertEqual(peer.control.last, #"{"type":"password","password":"kept one"}"#)
        // Changed on the computer: forgotten here, and asked.
        peer.events.onControlMessage(#"{"type":"password_result","ok":false,"reason":"wrong","left":4}"#)
        XCTAssertTrue(rejected)
        XCTAssertEqual(s.status, .password(PasswordPrompt(wrong: true, left: 4, checking: false)))
        s.sendPassword("new one")
        peer.events.onControlMessage(#"{"type":"password_result","ok":true}"#)
        XCTAssertEqual(accepted, "new one")
    }

    func testPasswordAndUnlinked() {
        let s = session(.reach(reachLink()))
        _ = keys.register("L")
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","sig":"\#(hostProof(nonce))"}"#)
        peer.events.onControlMessage(#"{"type":"auth_ok","link_id":"L"}"#)
        peer.events.onControlMessage(#"{"type":"password_required"}"#)
        XCTAssertEqual(s.status, .password(PasswordPrompt()))
        s.sendPassword("nope")
        XCTAssertEqual(peer.control.last, #"{"type":"password","password":"nope"}"#)
        peer.events.onControlMessage(#"{"type":"password_result","ok":false,"reason":"wrong","left":4}"#)
        XCTAssertEqual(s.status, .password(PasswordPrompt(wrong: true, left: 4, checking: false)))
        peer.events.onControlMessage(#"{"type":"password_result","ok":true}"#)
        XCTAssertEqual(s.status, .waiting)

        // A revoked link: before any proof, an unsigned refusal is ignored; a
        // refusal the computer signed is honored.
        let u = session(.reach(reachLink()))
        _ = keys.register("L")
        var forgot = false
        u.events.unlinked = { forgot = true }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let up = peers.created[0]
        up.events.onControlOpen()
        let un = JSONText.parseObject(up.control.last!)!["nonce"].str()
        up.events.onControlMessage(#"{"type":"auth_failed"}"#)
        XCTAssertFalse(forgot)
        XCTAssertNotEqual(u.status, .unlinked)
        let sig = try! hostKey.sign(LinkAuth.hostUnlinkMessage(nonce: un, host: "", viewer: ""))
        up.events.onControlMessage(#"{"type":"auth_failed","sig":"\#(sig)"}"#)
        XCTAssertTrue(forgot)
        XCTAssertEqual(u.status, .unlinked)

        let off = session(.reach(reachLink()))
        sockets.last.receive(#"{"type":"error","error":"device not connected"}"#)
        XCTAssertEqual(off.status, .offline)
    }
}
