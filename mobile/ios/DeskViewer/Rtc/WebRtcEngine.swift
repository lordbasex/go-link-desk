// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
@preconcurrency import WebRTC

/**
 * The process-wide libwebrtc objects: one factory, with the hardware H.264
 * decoder (VideoToolbox) and the app's own audio device, which only plays
 * (see DeskAudioDevice): this app never opens the microphone.
 */
@MainActor
final class WebRtcEngine {
    static let shared = WebRtcEngine()

    let audioDevice = DeskAudioDevice()
    let factory: RTCPeerConnectionFactory

    private init() {
        RTCInitializeSSL()
        factory = RTCPeerConnectionFactory(
            encoderFactory: RTCDefaultVideoEncoderFactory(),
            decoderFactory: RTCDefaultVideoDecoderFactory(),
            audioDevice: audioDevice
        )
    }
}
