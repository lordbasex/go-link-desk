// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import Foundation
@preconcurrency import WebRTC

/** The computer's media, handed to the session screen on the main actor. */
@MainActor
protocol MediaListener: AnyObject {
    func onVideo(_ track: RTCVideoTrack)
    func onAudio(_ track: RTCAudioTrack)
    /** The connection is closing: its tracks go away. */
    func onMediaClosed()
}

/**
 * The app's side of the WebRTC link with the computer, adapted from
 * go-link's player app (IOSRtcPeer): it answers the computer's offer,
 * receives its picture and sound, and speaks the "control", "pointer" and
 * "input" channels the computer opens. A video call's tracks (the host's
 * camera) and the files channel are left alone: this app does not have
 * them yet.
 */
@MainActor
final class DeskRtcPeer: NSObject, RtcPeer {
    private let events: RtcPeerEvents
    private weak var media: MediaListener?
    private var pc: RTCPeerConnection!
    private var control: RTCDataChannel?
    private var pointer: RTCDataChannel?
    private var input: RTCDataChannel?
    private var closed = false

    init(engine: WebRtcEngine, iceServers: [IceServer], events: RtcPeerEvents, media: MediaListener) {
        self.events = events
        self.media = media
        super.init()
        let config = RTCConfiguration()
        // STUN/TURN from signalhub's hello, never hardcoded.
        config.iceServers = iceServers.map { RTCIceServer(urlStrings: $0.urls, username: $0.username, credential: $0.credential) }
        config.sdpSemantics = .unifiedPlan
        config.continualGatheringPolicy = .gatherContinually
        let constraints = RTCMediaConstraints(mandatoryConstraints: nil, optionalConstraints: nil)
        pc = engine.factory.peerConnection(with: config, constraints: constraints, delegate: self)
    }

    func answer(offerSdp: String, done: @escaping (Result<String, Error>) -> Void) {
        guard let pc, !closed else {
            done(.failure(PeerError.closed))
            return
        }
        pc.setRemoteDescription(RTCSessionDescription(type: .offer, sdp: offerSdp)) { [weak self] error in
            DispatchQueue.main.async {
                guard let self, !self.closed else { return done(.failure(PeerError.closed)) }
                if let error { return done(.failure(error)) }
                let constraints = RTCMediaConstraints(mandatoryConstraints: nil, optionalConstraints: nil)
                pc.answer(for: constraints) { answer, error in
                    DispatchQueue.main.async {
                        guard let answer, error == nil else { return done(.failure(error ?? PeerError.noAnswer)) }
                        pc.setLocalDescription(answer) { error in
                            DispatchQueue.main.async {
                                if let error { done(.failure(error)) } else { done(.success(answer.sdp)) }
                            }
                        }
                    }
                }
            }
        }
    }

    func addCandidate(_ candidate: IceCandidateInit) {
        guard !closed else { return }
        pc.add(RTCIceCandidate(sdp: candidate.candidate, sdpMLineIndex: Int32(candidate.sdpMLineIndex), sdpMid: candidate.sdpMid)) { _ in }
    }

    func sendControl(_ text: String) -> Bool {
        guard !closed, let ch = control, ch.readyState == .open, let data = text.data(using: .utf8) else { return false }
        return ch.sendData(RTCDataBuffer(data: data, isBinary: false))
    }

    func sendPointer(_ packet: [UInt8]) -> Bool {
        // Only the newest position counts: a backed up channel drops it.
        guard !closed, let ch = pointer, ch.readyState == .open, ch.bufferedAmount < 64 << 10 else { return false }
        return ch.sendData(RTCDataBuffer(data: Data(packet), isBinary: true))
    }

    func sendInput(_ packet: [UInt8]) -> Bool {
        guard !closed, let ch = input, ch.readyState == .open else { return false }
        return ch.sendData(RTCDataBuffer(data: Data(packet), isBinary: true))
    }

    /** libwebrtc's statistics, for the details panel and the loss report. */
    func stats(_ done: @escaping (RTCStatisticsReport) -> Void) {
        guard !closed else { return }
        pc.statistics { report in DispatchQueue.main.async { done(report) } }
    }

    func close() {
        guard !closed else { return }
        closed = true
        media?.onMediaClosed()
        control?.delegate = nil
        // Let the last control message (session_end) go before closing.
        let old = pc
        let channels = [control, pointer, input]
        control = nil
        pointer = nil
        input = nil
        pc.delegate = nil
        // Closing waits on libwebrtc's threads: never on the main thread.
        DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + 0.2) {
            for c in channels { c?.close() }
            old?.close()
        }
    }

    enum PeerError: Error { case closed, noAnswer }
}

extension DeskRtcPeer: RTCPeerConnectionDelegate {
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didGenerate candidate: RTCIceCandidate) {
        let c = IceCandidateInit(candidate: candidate.sdp, sdpMid: candidate.sdpMid, sdpMLineIndex: Int(candidate.sdpMLineIndex))
        DispatchQueue.main.async {
            if !self.closed { self.events.onLocalCandidate(c) }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCPeerConnectionState) {
        DispatchQueue.main.async {
            guard !self.closed else { return }
            switch newState {
            case .connected: self.events.onState(.connected)
            case .disconnected: self.events.onState(.disconnected)
            case .failed: self.events.onState(.failed)
            default: break
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didOpen dataChannel: RTCDataChannel) {
        // Listen at once: the computer's first message (welcome) may come
        // before the main queue gets to this channel.
        if dataChannel.label == "control" { dataChannel.delegate = self }
        DispatchQueue.main.async {
            guard !self.closed else { return }
            switch dataChannel.label {
            case "control":
                self.control = dataChannel
                if dataChannel.readyState == .open { self.events.onControlOpen() }
            case "pointer":
                self.pointer = dataChannel
            case "input":
                self.input = dataChannel
            default:
                dataChannel.close() // files: not in this app yet
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didAdd rtpReceiver: RTCRtpReceiver, streams mediaStreams: [RTCMediaStream]) {
        let stream = mediaStreams.first?.streamId ?? ""
        let track = rtpReceiver.track
        DispatchQueue.main.async {
            // A video call's tracks (the host's camera and microphone).
            guard !self.closed, let track, stream != "go-link-desk-call" else { return }
            if let v = track as? RTCVideoTrack {
                self.media?.onVideo(v)
            } else if let a = track as? RTCAudioTrack {
                self.media?.onAudio(a)
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange stateChanged: RTCSignalingState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didAdd stream: RTCMediaStream) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didRemove stream: RTCMediaStream) {}
    nonisolated func peerConnectionShouldNegotiate(_ peerConnection: RTCPeerConnection) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCIceConnectionState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCIceGatheringState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didRemove candidates: [RTCIceCandidate]) {}
}

extension DeskRtcPeer: RTCDataChannelDelegate {
    nonisolated func dataChannelDidChangeState(_ dataChannel: RTCDataChannel) {
        let open = dataChannel.readyState == .open
        DispatchQueue.main.async {
            if open && !self.closed && dataChannel === self.control { self.events.onControlOpen() }
        }
    }

    nonisolated func dataChannel(_ dataChannel: RTCDataChannel, didReceiveMessageWith buffer: RTCDataBuffer) {
        guard !buffer.isBinary, buffer.data.count <= 64 * 1024, let text = String(data: buffer.data, encoding: .utf8) else { return }
        DispatchQueue.main.async {
            if !self.closed && dataChannel.label == "control" { self.events.onControlMessage(text) }
        }
    }
}

/** Makes the libwebrtc peers for DeskSession. */
@MainActor
final class DeskPeers: RtcPeerFactory {
    weak var media: MediaListener?
    private(set) var current: DeskRtcPeer?

    func create(iceServers: [IceServer], events: RtcPeerEvents) -> RtcPeer {
        let p = DeskRtcPeer(engine: .shared, iceServers: iceServers, events: events, media: media!)
        current = p
        return p
    }
}
