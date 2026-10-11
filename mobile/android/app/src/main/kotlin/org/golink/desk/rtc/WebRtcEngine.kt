// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.rtc

import android.content.Context
import org.webrtc.DefaultVideoDecoderFactory
import org.webrtc.DefaultVideoEncoderFactory
import org.webrtc.EglBase
import org.webrtc.PeerConnectionFactory
import org.webrtc.audio.JavaAudioDeviceModule

/**
 * The process-wide libwebrtc objects, adapted from go-link's player app:
 * one factory with the hardware decoders, one EGL context shared by the
 * decoder and the picture, and one audio device module that only plays
 * the computer's sound: this app never opens the microphone.
 */
class WebRtcEngine private constructor(context: Context) {
    val eglBase: EglBase = EglBase.create()

    private val adm = JavaAudioDeviceModule.builder(context)
        .setUseStereoOutput(true)
        .createAudioDeviceModule()

    private val decoders = DefaultVideoDecoderFactory(eglBase.eglBaseContext)

    /**
     * The codecs this device decodes, in the knock's names and order: the
     * computer picks the first it can send. H.264 High and Baseline come
     * only with a hardware decoder that has them (an emulator may not).
     */
    val codecs: List<String> by lazy {
        val info = decoders.supportedCodecs
        fun h264(profile: String) = info.any { it.name.equals("H264", true) && (it.params["profile-level-id"] ?: "").lowercase().startsWith(profile) }
        buildList {
            if (h264("64")) add("h264-high")
            if (h264("42")) add("h264")
            if (info.any { it.name.equals("VP8", true) }) add("vp8")
        }.ifEmpty { listOf("vp8") }
    }

    private val encoders = DefaultVideoEncoderFactory(eglBase.eglBaseContext, true, true)

    /** Whether this device could send H.264 (a video call's camera line asks for it). */
    val canSendH264: Boolean by lazy { encoders.supportedCodecs.any { it.name.equals("H264", true) } }

    val factory: PeerConnectionFactory = PeerConnectionFactory.builder()
        .setAudioDeviceModule(adm)
        .setVideoDecoderFactory(decoders)
        .setVideoEncoderFactory(encoders)
        .createPeerConnectionFactory()

    companion object {
        @Volatile private var instance: WebRtcEngine? = null

        fun get(context: Context): WebRtcEngine = instance ?: synchronized(this) {
            instance ?: run {
                PeerConnectionFactory.initialize(
                    PeerConnectionFactory.InitializationOptions.builder(context.applicationContext).createInitializationOptions(),
                )
                WebRtcEngine(context.applicationContext).also { instance = it }
            }
        }
    }
}
