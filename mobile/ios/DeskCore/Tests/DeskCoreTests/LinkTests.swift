// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** Linking with the pairing code, and coming back to a linked computer (docs/protocol.md, "Unattended access"). */
@MainActor
final class LinkTests: XCTestCase {
    private var sockets: FakeSockets!
    private var clock: ManualScheduler!
    private var peers: FakePeers!

    private func session(_ door: Door) -> DeskSession {
        sockets = FakeSockets()
        clock = ManualScheduler()
        peers = FakePeers()
        let signal = SignalClient(url: "wss://signal.example/ws", factory: sockets, scheduler: clock)
        let s = DeskSession(door: door, name: "Ana", ua: "go-link Desk on iOS", signal: signal, peers: peers, scheduler: clock)
        s.start()
        sockets.last.receive(#"{"type":"hello","peer_id":"me"}"#)
        return s
    }

    private func fromHost(_ payload: String) {
        sockets.last.receive(#"{"type":"signal","from":"H","payload":\#(payload)}"#)
    }

    func testTheProofMatchesTheSharedVector() throws {
        let data = try Data(contentsOf: repoRoot().appendingPathComponent("protocol/vectors/link-proof.json"))
        let root = try XCTUnwrap(JSONText.parseObject(String(decoding: data, as: UTF8.self)))
        for c in root["cases"].arr {
            XCTAssertEqual(DeskSession.linkProof(token: c["token"].str(), nonce: c["nonce"].str()), c["proof"].str())
        }
        for c in root["bound_cases"].arr {
            XCTAssertEqual(DeskSession.linkProofBound(token: c["token"].str(), nonce: c["nonce"].str(), host: c["host_fingerprint"].str(), viewer: c["viewer_fingerprint"].str()), c["proof2"].str())
        }
        for c in root["fingerprint_cases"].arr {
            XCTAssertEqual(DeskSession.fingerprint(sdp: c["sdp"].str(10000)), c["fingerprint"].str())
        }
        XCTAssertTrue(DeskSession.same("abc", "abc"))
        XCTAssertFalse(DeskSession.same("abc", "abd"))
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
        XCTAssertEqual(peer.control.last, #"{"type":"auth"}"#)
        peer.events.onControlMessage(#"{"type":"auth_ok","device_id":"D","link_id":"L","token":"T"}"#)
        XCTAssertEqual(linked, DeviceLink(deviceId: "D", linkId: "L", token: "T"))
    }

    func testReachProvesTheComputerBeforeTheToken() {
        let link = DeviceLink(deviceId: "D", linkId: "L", token: "secret-token")
        let s = session(.reach(link))
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
        XCTAssertFalse(peer.control.contains { $0.contains("secret-token") })
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","proof":"\#(DeskSession.linkProof(token: "secret-token", nonce: nonce))"}"#)
        XCTAssertEqual(peer.control.last, #"{"type":"auth","link_id":"L","token":"secret-token"}"#)
        // The password, wrong then right.
        peer.events.onControlMessage(#"{"type":"auth_ok","link_id":"L"}"#)
        XCTAssertEqual(s.status, .connecting) // the early acceptance applies now
    }

    /** Sends the computer's proofs to a reaching session; returns whether the token went. */
    private func proofs(_ link: DeviceLink, proof2Host: String?, bound: (() -> Void)? = nil) -> (DeskSession, Bool) {
        let s = session(.reach(link))
        s.events.bound = bound
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        // The fake answer repeats the offer, so both sides show "sha-256 HO:ST".
        fromHost(#"{"kind":"offer","sdp":"v=0\r\na=fingerprint:sha-256 ho:st\r\n"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        var m = #"{"type":"auth_proof","link_id":"L","proof":"\#(DeskSession.linkProof(token: "secret-token", nonce: nonce))""#
        if let h = proof2Host {
            m += #","proof2":"\#(DeskSession.linkProofBound(token: "secret-token", nonce: nonce, host: h, viewer: "sha-256 HO:ST"))""#
        }
        peer.events.onControlMessage(m + "}")
        return (s, peer.control.last == #"{"type":"auth","link_id":"L","token":"secret-token"}"#)
    }

    func testTheProofBoundToTheConnection() {
        var bound = 0
        let link = DeviceLink(deviceId: "D", linkId: "L", token: "secret-token")
        XCTAssertTrue(proofs(link, proof2Host: "sha-256 HO:ST", bound: { bound += 1 }).1)
        XCTAssertEqual(bound, 1)
        // A computer from before proof2 still works, until one gave it.
        XCTAssertTrue(proofs(link, proof2Host: nil).1)
        // Relayed from another connection: someone in the middle has its own certificate.
        let (relayed, sent) = proofs(link, proof2Host: "sha-256 MI:TM")
        XCTAssertFalse(sent)
        XCTAssertEqual(relayed.status, .untrusted)
        // Once bound, the plain proof alone is refused.
        let (plain, sentPlain) = proofs(DeviceLink(deviceId: "D", linkId: "L", token: "secret-token", bound: true), proof2Host: nil)
        XCTAssertFalse(sentPlain)
        XCTAssertEqual(plain.status, .untrusted)
    }

    func testAWrongProofSendsNothing() {
        let link = DeviceLink(deviceId: "D", linkId: "L", token: "secret-token")
        let s = session(.reach(link))
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","proof":"forged"}"#)
        XCTAssertEqual(s.status, .untrusted)
        XCTAssertFalse(peer.control.contains { $0.contains("secret-token") })
    }

    func testTheSavedPasswordGoesByItself() {
        let link = DeviceLink(deviceId: "D", linkId: "L", token: "t")
        let s = session(.reach(link))
        s.savedPassword = "kept one"
        var accepted: String?, rejected = false
        s.events.passwordAccepted = { accepted = $0 }
        s.events.passwordRejected = { rejected = true }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","proof":"\#(DeskSession.linkProof(token: "t", nonce: nonce))"}"#)
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
        let link = DeviceLink(deviceId: "D", linkId: "L", token: "t")
        let s = session(.reach(link))
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        let peer = peers.created[0]
        peer.events.onControlOpen()
        let nonce = JSONText.parseObject(peer.control.last!)!["nonce"].str()
        peer.events.onControlMessage(#"{"type":"auth_proof","link_id":"L","proof":"\#(DeskSession.linkProof(token: "t", nonce: nonce))"}"#)
        peer.events.onControlMessage(#"{"type":"auth_ok","link_id":"L"}"#)
        peer.events.onControlMessage(#"{"type":"password_required"}"#)
        XCTAssertEqual(s.status, .password(PasswordPrompt()))
        s.sendPassword("nope")
        XCTAssertEqual(peer.control.last, #"{"type":"password","password":"nope"}"#)
        peer.events.onControlMessage(#"{"type":"password_result","ok":false,"reason":"wrong","left":4}"#)
        XCTAssertEqual(s.status, .password(PasswordPrompt(wrong: true, left: 4, checking: false)))
        peer.events.onControlMessage(#"{"type":"password_result","ok":true}"#)
        XCTAssertEqual(s.status, .waiting)

        let u = session(.reach(link))
        var forgot = false
        u.events.unlinked = { forgot = true }
        sockets.last.receive(#"{"type":"paired","remote":"H"}"#)
        fromHost(#"{"kind":"offer","sdp":"v=0"}"#)
        peers.created[0].events.onControlMessage(#"{"type":"auth_failed"}"#)
        XCTAssertTrue(forgot)
        XCTAssertEqual(u.status, .unlinked)

        let off = session(.reach(link))
        sockets.last.receive(#"{"type":"error","error":"device not connected"}"#)
        XCTAssertEqual(off.status, .offline)
    }
}
