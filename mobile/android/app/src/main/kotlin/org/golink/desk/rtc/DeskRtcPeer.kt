// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.rtc

import android.os.Handler
import android.os.Looper
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.suspendCancellableCoroutine
import org.golink.desk.core.FilesChannel
import org.golink.desk.core.FilesClient
import org.golink.desk.core.IceCandidateInit
import org.golink.desk.core.IceServer
import org.golink.desk.core.PeerState
import org.golink.desk.core.RtcPeer
import org.golink.desk.core.RtcPeerEvents
import org.golink.desk.core.RtcPeerFactory
import org.golink.desk.core.StatsMeter
import org.webrtc.AudioTrack
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.RTCStatsReport
import org.webrtc.RtpReceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import org.webrtc.VideoTrack
import java.nio.ByteBuffer
import java.util.concurrent.Executors
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** The computer's media, handed to the session screen on the main thread. */
interface MediaListener {
    fun onVideo(track: VideoTrack)

    fun onAudio(track: AudioTrack)

    /** The files channel opened (with the files permission). */
    fun onFiles(client: FilesClient)

    /** The connection is closing: its tracks go away. */
    fun onMediaClosed()
}

/**
 * The app's side of the WebRTC link with the computer, adapted from
 * go-link's player app (AndroidRtcPeer) and the iOS app's DeskRtcPeer: it
 * answers the computer's offer, receives its picture and sound, and speaks
 * the "control", "pointer" and "input" channels the computer opens, and its
 * "files" channel. A video call's tracks (the host's camera) are left alone:
 * this app does not have calls yet. Every event goes to the main thread.
 */
class DeskRtcPeer(
    private val engine: WebRtcEngine,
    iceServers: List<IceServer>,
    private val events: RtcPeerEvents,
    private val media: MediaListener,
    private val scope: CoroutineScope,
) : RtcPeer {
    private val main = Handler(Looper.getMainLooper())

    @Volatile private var control: DataChannel? = null

    @Volatile private var pointer: DataChannel? = null

    @Volatile private var input: DataChannel? = null

    @Volatile private var files: FilesAdapter? = null

    @Volatile private var closed = false

    private val pc: PeerConnection

    init {
        // STUN/TURN from signalhub's hello, never hardcoded.
        val servers = iceServers.map { s ->
            PeerConnection.IceServer.builder(s.urls)
                .setUsername(s.username ?: "")
                .setPassword(s.credential ?: "")
                .createIceServer()
        }
        val config = PeerConnection.RTCConfiguration(servers).apply {
            sdpSemantics = PeerConnection.SdpSemantics.UNIFIED_PLAN
            continualGatheringPolicy = PeerConnection.ContinualGatheringPolicy.GATHER_CONTINUALLY
        }
        pc = requireNotNull(engine.factory.createPeerConnection(config, Observer())) { "peer connection" }
    }

    private fun onMain(fn: () -> Unit) {
        main.post { if (!closed) fn() }
    }

    override suspend fun answer(offerSdp: String): String = try {
        pc.awaitSet(SessionDescription(SessionDescription.Type.OFFER, offerSdp))
        declineCall(offerSdp)
        val answer = pc.awaitCreateAnswer()
        pc.awaitSet(answer, local = true)
        answer.description
    } catch (e: Exception) {
        android.util.Log.w("go-link-desk", "answer: ${e.message}")
        throw e
    }

    /**
     * The video call's lines (the computer offers to receive this device's
     * camera and microphone) are turned down when this device could not
     * send them: an answer for a line with no codec in common fails, and
     * this app has no calls yet.
     */
    private fun declineCall(offer: String) {
        if (engine.canSendH264) return
        val mids = Regex("\\r?\\nm=").split(offer).drop(1)
            .filter { Regex("\\na=recvonly").containsMatchIn(it) }
            .mapNotNull { Regex("\\na=mid:([^\\r\\n]+)").find(it)?.groupValues?.get(1) }
            .toSet()
        for (t in pc.transceivers) if (t.mid in mids) runCatching { t.stop() }
    }

    override fun addCandidate(candidate: IceCandidateInit) {
        if (!closed) pc.addIceCandidate(IceCandidate(candidate.sdpMid ?: "", candidate.sdpMLineIndex, candidate.candidate))
    }

    override fun sendControl(text: String): Boolean {
        val ch = control ?: return false
        if (closed || ch.state() != DataChannel.State.OPEN) return false
        return ch.send(DataChannel.Buffer(ByteBuffer.wrap(text.toByteArray(Charsets.UTF_8)), false))
    }

    override fun sendPointer(packet: ByteArray): Boolean {
        // Only the newest position counts: a backed up channel drops it.
        val ch = pointer ?: return false
        if (closed || ch.state() != DataChannel.State.OPEN || ch.bufferedAmount() > 64 shl 10) return false
        return ch.send(DataChannel.Buffer(ByteBuffer.wrap(packet), true))
    }

    override fun sendInput(packet: ByteArray): Boolean {
        val ch = input ?: return false
        if (closed || ch.state() != DataChannel.State.OPEN) return false
        return ch.send(DataChannel.Buffer(ByteBuffer.wrap(packet), true))
    }

    /** One reading for the details panel and the loss report, delivered on the main thread. */
    fun sample(done: (StatsMeter.Sample) -> Unit) {
        if (closed) return
        pc.getStats { report -> val s = toSample(report); main.post { done(s) } }
    }

    private fun toSample(report: RTCStatsReport): StatsMeter.Sample {
        val all = report.statsMap
        fun num(v: Any?): Long? = (v as? Number)?.toLong()
        var s = StatsMeter.Sample(atMs = System.nanoTime() / 1e6)
        for (x in all.values) {
            if (x.type != "inbound-rtp" || x.members["kind"] != "video") continue
            val m = x.members
            s = s.copy(
                framesDecoded = num(m["framesDecoded"]),
                bytesReceived = num(m["bytesReceived"]),
                packetsReceived = num(m["packetsReceived"]),
                packetsLost = num(m["packetsLost"])?.coerceAtLeast(0),
                width = num(m["frameWidth"])?.toInt(),
                height = num(m["frameHeight"])?.toInt(),
            )
        }
        // The pair in use: the transport's selected one, or the nominated pair that succeeded.
        val selected = all.values.firstOrNull { it.type == "transport" }?.members?.get("selectedCandidatePairId") as? String
        val pair = all[selected] ?: all.values.firstOrNull {
            it.type == "candidate-pair" && it.members["nominated"] == true && it.members["state"] == "succeeded"
        }
        if (pair != null) {
            s = s.copy(
                rttSeconds = (pair.members["currentRoundTripTime"] as? Number)?.toDouble(),
                localType = all[pair.members["localCandidateId"] as? String]?.members?.get("candidateType") as? String,
                remoteType = all[pair.members["remoteCandidateId"] as? String]?.members?.get("candidateType") as? String,
            )
        }
        return s
    }

    override fun close() {
        if (closed) return
        media.onMediaClosed()
        files?.client?.onClosed()
        closed = true
        val channels = listOf(control, pointer, input, files?.channel)
        control = null
        pointer = null
        input = null
        files = null
        // Let the last control message (session_end) go, then dispose off the
        // main thread: it blocks on libwebrtc's threads.
        disposer.execute {
            Thread.sleep(200)
            for (c in channels) {
                c?.unregisterObserver()
                c?.dispose()
            }
            pc.dispose()
        }
    }

    private inner class Observer : PeerConnection.Observer {
        override fun onIceCandidate(c: IceCandidate) {
            val init = IceCandidateInit(c.sdp, c.sdpMid, c.sdpMLineIndex)
            onMain { events.onLocalCandidate(init) }
        }

        override fun onConnectionChange(newState: PeerConnection.PeerConnectionState) {
            val s = when (newState) {
                PeerConnection.PeerConnectionState.CONNECTED -> PeerState.CONNECTED
                PeerConnection.PeerConnectionState.DISCONNECTED -> PeerState.DISCONNECTED
                PeerConnection.PeerConnectionState.FAILED -> PeerState.FAILED
                else -> return
            }
            android.util.Log.i("go-link-desk", "peer $newState")
            onMain { events.onState(s) }
        }

        override fun onDataChannel(ch: DataChannel) {
            if (closed) return
            when (ch.label()) {
                "control" -> {
                    control = ch
                    // Listen at once: the computer's first message (welcome) may
                    // come before the main thread gets to this channel.
                    ch.registerObserver(object : DataChannel.Observer {
                        override fun onBufferedAmountChange(previousAmount: Long) = Unit

                        override fun onStateChange() {
                            if (ch.state() == DataChannel.State.OPEN) onMain { events.onControlOpen() }
                        }

                        override fun onMessage(buffer: DataChannel.Buffer) {
                            if (buffer.binary || buffer.data.remaining() > 64 * 1024) return
                            val bytes = ByteArray(buffer.data.remaining())
                            buffer.data.get(bytes)
                            val text = String(bytes, Charsets.UTF_8)
                            onMain { events.onControlMessage(text) }
                        }
                    })
                    if (ch.state() == DataChannel.State.OPEN) onMain { events.onControlOpen() }
                }
                "pointer" -> pointer = ch
                "input" -> input = ch
                "files" -> {
                    val a = FilesAdapter(ch)
                    files = a
                    onMain {
                        val client = FilesClient(a, scope)
                        a.client = client
                        media.onFiles(client)
                    }
                }
                else -> ch.dispose()
            }
        }

        override fun onAddTrack(receiver: RtpReceiver, streams: Array<out MediaStream>) {
            // A video call's tracks (the host's camera and microphone).
            if (closed || streams.firstOrNull()?.id == "go-link-desk-call") return
            when (val track = receiver.track()) {
                is VideoTrack -> onMain { media.onVideo(track) }
                is AudioTrack -> onMain { media.onAudio(track) }
            }
        }

        override fun onSignalingChange(state: PeerConnection.SignalingState) = Unit

        override fun onIceConnectionChange(state: PeerConnection.IceConnectionState) = Unit

        override fun onIceConnectionReceivingChange(receiving: Boolean) = Unit

        override fun onIceGatheringChange(state: PeerConnection.IceGatheringState) = Unit

        override fun onIceCandidatesRemoved(candidates: Array<out IceCandidate>) = Unit

        override fun onAddStream(stream: MediaStream) = Unit

        override fun onRemoveStream(stream: MediaStream) = Unit

        override fun onRenegotiationNeeded() = Unit
    }

    /**
     * The "files" data channel for FilesClient: messages go to the client on
     * the main thread, and so does the queue draining under half the upload
     * window. Its observer is set at once, so no first message is lost.
     */
    private inner class FilesAdapter(val channel: DataChannel) : FilesChannel {
        var client: FilesClient? = null

        init {
            channel.registerObserver(object : DataChannel.Observer {
                override fun onBufferedAmountChange(previousAmount: Long) {
                    if (channel.bufferedAmount() <= FilesClient.UPLOAD_WINDOW / 2) main.post { client?.onBufferedLow() }
                }

                override fun onStateChange() {
                    val s = channel.state()
                    if (s == DataChannel.State.CLOSING || s == DataChannel.State.CLOSED) main.post { client?.onClosed() }
                }

                override fun onMessage(buffer: DataChannel.Buffer) {
                    if (buffer.data.remaining() > 128 * 1024) return
                    val bytes = ByteArray(buffer.data.remaining())
                    buffer.data.get(bytes)
                    val binary = buffer.binary
                    main.post {
                        val c = client ?: return@post
                        if (binary) c.onBinary(bytes) else c.onText(String(bytes, Charsets.UTF_8))
                    }
                }
            })
        }

        override fun sendText(text: String): Boolean = channel.send(DataChannel.Buffer(ByteBuffer.wrap(text.toByteArray(Charsets.UTF_8)), false))

        override fun sendData(bytes: ByteArray): Boolean = channel.send(DataChannel.Buffer(ByteBuffer.wrap(bytes), true))

        override val bufferedAmount: Long get() = channel.bufferedAmount()
        override val isOpen: Boolean get() = !closed && channel.state() == DataChannel.State.OPEN
    }

    companion object {
        private val disposer = Executors.newSingleThreadExecutor()
    }
}

/** Makes the libwebrtc peers for DeskSession. */
class DeskPeers(private val engine: WebRtcEngine, private val scope: CoroutineScope) : RtcPeerFactory {
    lateinit var media: MediaListener
    var current: DeskRtcPeer? = null
        private set

    override fun create(iceServers: List<IceServer>, events: RtcPeerEvents): RtcPeer =
        DeskRtcPeer(engine, iceServers, events, media, scope).also { current = it }
}

private suspend fun PeerConnection.awaitSet(desc: SessionDescription, local: Boolean = false) =
    suspendCancellableCoroutine { cont ->
        val observer = object : SdpObserver {
            override fun onCreateSuccess(p0: SessionDescription?) = Unit

            override fun onCreateFailure(p0: String?) = Unit

            override fun onSetSuccess() {
                if (cont.isActive) cont.resume(Unit)
            }

            override fun onSetFailure(error: String?) {
                if (cont.isActive) cont.resumeWithException(IllegalStateException(error ?: "set description"))
            }
        }
        if (local) setLocalDescription(observer, desc) else setRemoteDescription(observer, desc)
    }

private suspend fun PeerConnection.awaitCreateAnswer(): SessionDescription =
    suspendCancellableCoroutine { cont ->
        createAnswer(
            object : SdpObserver {
                override fun onCreateSuccess(desc: SessionDescription) {
                    if (cont.isActive) cont.resume(desc)
                }

                override fun onCreateFailure(error: String?) {
                    if (cont.isActive) cont.resumeWithException(IllegalStateException(error ?: "create answer"))
                }

                override fun onSetSuccess() = Unit

                override fun onSetFailure(p0: String?) = Unit
            },
            MediaConstraints(),
        )
    }
