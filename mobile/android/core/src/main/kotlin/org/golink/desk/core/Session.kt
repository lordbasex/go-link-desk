// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

// One visit to a computer with its session code, the Android port of the
// iOS app's DeskSession (mobile/ios/DeskCore), itself a port of the Mac
// app's viewer (backend-device/internal/viewer) and the website's session
// (frontend/apps/web/src/session/session.ts): join the computer's room,
// knock, wait for the person there to accept, answer the offer, and then
// speak the control, pointer and input channels. The computer cannot tell
// it from the website.
//
// Every method runs on the scope's thread (the main thread in the app), and
// so do the peer's events: the app's peer moves them there.

/** The app's side of the WebRTC link: libwebrtc in the app, a fake in the tests. */
interface RtcPeer {
    /** Applies the computer's offer and returns the answer's SDP. */
    suspend fun answer(offerSdp: String): String

    fun addCandidate(candidate: IceCandidateInit)

    /** JSON text on the reliable "control" channel. */
    fun sendControl(text: String): Boolean

    /** A pointer position on the "pointer" channel (unordered: only the newest counts). */
    fun sendPointer(packet: ByteArray): Boolean

    /** A button, wheel, key or text packet on the "input" channel. */
    fun sendInput(packet: ByteArray): Boolean

    fun close()
}

enum class PeerState { CONNECTED, DISCONNECTED, FAILED }

/** Peer events, delivered on the session's thread. */
interface RtcPeerEvents {
    fun onLocalCandidate(candidate: IceCandidateInit)

    fun onState(state: PeerState)

    fun onControlOpen()

    fun onControlMessage(text: String)
}

fun interface RtcPeerFactory {
    fun create(iceServers: List<IceServer>, events: RtcPeerEvents): RtcPeer
}

/** A computer linked before: its id, the link's id and this device's secret for it. */
/**
 * bound: the computer once proved itself bound to the connection (proof2);
 * from then on a proof without it is refused, so nobody in the middle can
 * fall back to the plain one.
 */
data class DeviceLink(val deviceId: String, val linkId: String, val token: String, val bound: Boolean = false)

/** How a session reaches its computer. */
sealed interface Door {
    /** The 9-digit session code: the person there accepts this time. */
    data class Join(val code: String) : Door

    /** The computer's pairing code: the person there accepts, and this device is linked. */
    data class Claim(val code: String) : Door

    /** A computer linked before: nobody accepts (unattended access). */
    data class Reach(val link: DeviceLink) : Door
}

/** The unattended password the computer asks for. */
data class PasswordPrompt(
    /** The last try was wrong, with this many tries left. */
    val wrong: Boolean = false,
    val left: Int = 0,
    /** A password is being checked. */
    val checking: Boolean = false,
)

/** Where a session is, from the knock to its end. */
sealed interface SessionStatus {
    data object Joining : SessionStatus

    /** The person at the computer decides. */
    data object Waiting : SessionStatus

    data object Connecting : SessionStatus

    data object Live : SessionStatus

    data object Reconnecting : SessionStatus

    /** A linked computer asks for its unattended password. */
    data class Password(val prompt: PasswordPrompt) : SessionStatus

    /** declined, timeout, busy, unavailable or off. */
    data class Rejected(val reason: String) : SessionStatus

    /** host, viewer, network, signal, idle or revoked. */
    data class Ended(val reason: String) : SessionStatus

    data object NotFound : SessionStatus

    data object RateLimited : SessionStatus

    data object Unreachable : SessionStatus

    /** The linked computer is not on. */
    data object Offline : SessionStatus

    /** The computer no longer has this link: it was forgotten here. */
    data object Unlinked : SessionStatus

    /** Whoever answered could not prove it is the linked computer. Nothing was sent to it. */
    data object Untrusted : SessionStatus

    /** Too many wrong passwords: blocked (this device) or locked (retryAfter seconds). */
    data class PasswordRefused(val reason: String, val retryAfter: Int) : SessionStatus

    val isOver: Boolean
        get() = when (this) {
            Joining, Waiting, Connecting, Live, Reconnecting, is Password -> false
            else -> true
        }
}

/** One of the computer's screens. */
data class Display(
    val id: Int,
    val name: String,
    val width: Int,
    val height: Int,
    /** Its size in points (the pointer's image is sized in them). */
    val pointsWidth: Int,
    val pointsHeight: Int,
    val primary: Boolean,
)

/** What the computer says it sends (stream_stats). */
data class Sent(val fps: Double, val width: Int, val height: Int, val kbps: Int, val codec: String)

/** The computer's pointer image: a PNG, its hot spot in the image's pixels, its size in the computer's points. */
class CursorShape(
    val id: String,
    val png: ByteArray,
    val hotX: Double,
    val hotY: Double,
    val width: Double,
    val height: Double,
    val pixels: Double,
)

/** What the screen learns. Every callback is optional and runs on the session's thread. */
class SessionEvents {
    var status: ((SessionStatus) -> Unit)? = null
    var host: ((name: String, os: String) -> Unit)? = null
    var permissions: ((Permissions) -> Unit)? = null
    var displays: ((displays: List<Display>, current: Int) -> Unit)? = null
    var sent: ((Sent) -> Unit)? = null
    var notice: ((String) -> Unit)? = null
    var cursor: ((CursorShape) -> Unit)? = null
    var cursorPos: ((x: Double, y: Double, visible: Boolean) -> Unit)? = null
    var clipboard: ((String) -> Unit)? = null
    var chat: ((String) -> Unit)? = null

    /** The person at the computer is writing a chat message (on), or stopped. */
    var typing: ((Boolean) -> Unit)? = null

    /** Linked with the pairing code: keep it to come back without a code. */
    var linked: ((DeviceLink) -> Unit)? = null

    /** The computer no longer has this link: forget it. */
    var unlinked: (() -> Unit)? = null

    /** The computer proved itself bound to the connection for the first time: remember it (DeviceLink.bound). */
    var bound: (() -> Unit)? = null

    /** The computer took this unattended password: keep it with the link. */
    var passwordAccepted: ((String) -> Unit)? = null

    /** The password kept with the link was wrong (it changed there): forget it. */
    var passwordRejected: (() -> Unit)? = null

    /** The computer stopped this viewer's recording (it took the permission away, or the session ended). */
    var recordingStopped: (() -> Unit)? = null
}

/** Video presets (docs/protocol.md, "Video"). */
enum class Preset(val wire: String) { SPEED("speed"), BALANCED("balanced"), QUALITY("quality") }

class DeskSession(
    private val door: Door,
    private val name: String,
    private val ua: String,
    private val signal: SignalClient,
    private val peers: RtcPeerFactory,
    private val scope: CoroutineScope,
    /** The video codecs this device decodes, best first (CODECS when all). */
    private val codecs: List<String> = CODECS,
) : RtcPeerEvents {
    val events = SessionEvents()

    var status: SessionStatus = SessionStatus.Joining
        private set
    var permissions = Permissions()
        private set
    var hostOS = ""
        private set
    var displays: List<Display> = emptyList()
        private set
    var currentDisplay = 0
        private set

    /** The display shown now, if the computer listed it. */
    val display: Display? get() = displays.firstOrNull { it.id == currentDisplay }

    /** The unattended password kept with the link: sent by itself when the computer asks for it. */
    var savedPassword: String? = null

    private var removeListener: (() -> Unit)? = null
    private var stateJob: Job? = null
    private var joinTimer: Job? = null
    private var host = ""
    private var asked = false
    private var admitted = false
    private var connected = false
    private var peer: RtcPeer? = null
    private val backlog = mutableListOf<HostSignal>()
    private val pendingControl = mutableListOf<String>()
    private var controlOpen = false
    private var clipSeq = 0L
    private val clipIn = ClipboardAssembler()
    private val shapes = HashMap<String, CursorShape>()
    private var pointerSeq = 0
    private var nonce = ""

    /** The connection's SDPs, the computer's offer and this viewer's answer: their certificates bind the link's proof. */
    private var offerSdp = ""
    private var answerSdp = ""
    private var proven = false
    private var authed = false
    private var early: Permissions? = null
    private var lastPassword = ""
    private var autoTried = false
    private val asks = HashMap<Long, (Boolean) -> Unit>()
    private val askTimers = HashMap<Long, Job>()
    private var askSeq = 0L

    /** The computer shows that this viewer records its screen. */
    var recording = false
        private set

    fun start() {
        removeListener = signal.addListener { handle(it) }
        // The state from now on (StateFlow replays the current one: dropped).
        stateJob = scope.launch {
            signal.state.drop(1).collect { state ->
                if (state == ConnectionState.OPEN) {
                    ask()
                } else if (state == ConnectionState.CONNECTING && asked) {
                    // The room went with the connection.
                    finish(SessionStatus.Ended("signal"))
                }
            }
        }
        joinTimer = scope.launch {
            delay(JOIN_TIMEOUT_MS)
            if (status == SessionStatus.Joining) finish(SessionStatus.Unreachable)
        }
        emit()
        signal.connect()
        if (signal.state.value == ConnectionState.OPEN) ask()
    }

    /** Leaves: the computer is told, and the session ends. */
    fun hangUp() {
        if (status.isOver) return
        control(buildJsonObject { put("type", "session_end") })
        finish(SessionStatus.Ended("viewer"))
    }

    // Signaling

    private fun ask() {
        if (asked || status.isOver) return
        asked = true
        signal.send(
            when (val d = door) {
                is Door.Join -> Envelopes.join(d.code)
                is Door.Claim -> Envelopes.claim(d.code)
                is Door.Reach -> Envelopes.reach(d.link.deviceId)
            },
        )
    }

    private fun handle(env: Envelope) {
        when (env.type) {
            "joined", "paired" -> inRoom(env.remote)
            "error" -> {
                if (status != SessionStatus.Joining) return
                finish(
                    when (env.error) {
                        ServerError.RATE_LIMITED -> SessionStatus.RateLimited
                        ServerError.DEVICE_OFFLINE -> SessionStatus.Offline
                        else -> SessionStatus.NotFound
                    },
                )
            }
            "signal" -> {
                if (host.isEmpty() || env.from != host) return
                RtcSignals.parse(env.payload)?.let { onSignal(it) }
            }
            "peer_left" -> if (host.isNotEmpty() && env.from == host) finish(SessionStatus.Ended("signal"))
        }
    }

    private fun inRoom(remote: String) {
        if (host.isNotEmpty() || status.isOver || remote.isEmpty()) return
        host = remote
        joinTimer?.cancel()
        set(SessionStatus.Waiting)
        sendSignal(RtcSignals.knock(name, ua, codecs))
        // A linked computer proves itself first: the token goes only to the
        // computer it was linked to (sent when the control channel opens).
        val d = door
        if (d is Door.Reach) {
            nonce = randomNonce()
            control(
                buildJsonObject {
                    put("type", "auth_challenge")
                    put("link_id", d.link.linkId)
                    put("nonce", nonce)
                },
            )
        }
    }

    private fun onSignal(s: HostSignal) {
        when (s) {
            is HostSignal.Accepted -> {
                if (status.isOver) return
                // A linked computer lets this device in only once it proved
                // itself and took the token; its acceptance may overtake auth_ok.
                if (door is Door.Reach && !authed) {
                    early = s.permissions
                    return
                }
                admit(s.permissions)
            }
            is HostSignal.Rejected -> finish(SessionStatus.Rejected(s.reason))
            is HostSignal.Offer -> {
                if (peer == null) peer = peers.create(signal.iceServers, this)
                apply(s)
                val b = backlog.toList()
                backlog.clear()
                for (x in b) apply(x)
            }
            is HostSignal.Candidate -> {
                if (peer == null) {
                    if (backlog.size < 50) backlog += s
                    return
                }
                apply(s)
            }
        }
    }

    private fun admit(p: Permissions) {
        admitted = true
        permissions = p
        set(if (connected) SessionStatus.Live else SessionStatus.Connecting)
        events.permissions?.invoke(p)
        // Accepted with the pairing code: now ask for the link's token.
        if (door is Door.Claim) control(buildJsonObject { put("type", "auth") })
    }

    private fun apply(s: HostSignal) {
        val p = peer ?: return
        when (s) {
            is HostSignal.Offer -> scope.launch {
                val answer = runCatching { p.answer(s.sdp) }
                if (status.isOver) return@launch
                answer.fold({
                    offerSdp = s.sdp
                    answerSdp = it
                    sendSignal(RtcSignals.answer(it))
                }, { finish(SessionStatus.Ended("network")) })
            }
            is HostSignal.Candidate -> p.addCandidate(s.candidate)
            else -> Unit
        }
    }

    private fun sendSignal(payload: JsonObject) {
        if (host.isEmpty()) return
        signal.send(Envelopes.signal(host, payload))
    }

    // RtcPeerEvents

    override fun onLocalCandidate(candidate: IceCandidateInit) {
        sendSignal(RtcSignals.candidate(candidate))
    }

    override fun onState(state: PeerState) {
        if (status.isOver) return
        connected = state == PeerState.CONNECTED
        if (!admitted) {
            if (state == PeerState.FAILED) finish(SessionStatus.Ended("network"))
            return
        }
        when (state) {
            PeerState.CONNECTED -> set(SessionStatus.Live)
            PeerState.DISCONNECTED -> set(SessionStatus.Reconnecting)
            PeerState.FAILED -> finish(SessionStatus.Ended("network"))
        }
    }

    override fun onControlOpen() {
        controlOpen = true
        val p = pendingControl.toList()
        pendingControl.clear()
        for (t in p) peer?.sendControl(t)
    }

    override fun onControlMessage(text: String) {
        if (text.length > 64 shl 10) return
        val m = parseObject(text) ?: return
        when (m["type"].str(40)) {
            "ping" -> control(
                buildJsonObject {
                    put("type", "pong")
                    put("id", m["id"] ?: JsonNull)
                },
            )
            "welcome" -> {
                val h = m["host"].obj()
                hostOS = h["os"].str(16)
                events.host?.invoke(h["name"].str(64), hostOS)
            }
            "stream_stats" -> events.sent?.invoke(
                Sent(m["fps"].num(), m["width"].num().toInt(), m["height"].num().toInt(), m["kbps"].num().toInt(), m["codec"].str(16)),
            )
            "displays" -> {
                displays = m["displays"].arr().take(16).map { d ->
                    val x = d.obj()
                    Display(
                        id = x["id"].num().toInt(),
                        name = x["name"].str(64),
                        width = x["width"].num().toInt(),
                        height = x["height"].num().toInt(),
                        pointsWidth = x["points_width"].num().toInt(),
                        pointsHeight = x["points_height"].num().toInt(),
                        primary = x["primary"].isTrue(),
                    )
                }
                currentDisplay = m["current"].num().toInt()
                events.displays?.invoke(displays, currentDisplay)
            }
            "permissions" -> {
                permissions = Permissions.parse(m["permissions"])
                if (!permissions.record && recording) {
                    recording = false
                    events.recordingStopped?.invoke()
                }
                events.permissions?.invoke(permissions)
            }
            "screenshot" -> answer(m["id"].num().coerceAtLeast(0.0).toLong(), m["ok"].isTrue())
            "recording" -> {
                // The answer to startRecording, or the computer stopping it (id 0).
                val id = m["id"].num().coerceAtLeast(0.0).toLong()
                val on = m["on"].isTrue() && permissions.record
                if (on && id !in asks) {
                    stopMessage() // a yes that came too late: nothing records here
                } else {
                    if (on) {
                        recording = true
                    } else if (recording) {
                        recording = false
                        if (id !in asks) events.recordingStopped?.invoke()
                    }
                    answer(id, on)
                }
            }
            "notice" -> events.notice?.invoke(m["code"].str(40))
            "cursor" -> cursor(m)
            "cursor_pos" -> events.cursorPos?.invoke(m["x"].num(), m["y"].num(), m["visible"].isTrue())
            "clipboard" -> {
                val id = m["id"].num().coerceAtLeast(0.0).toLong()
                val t = clipIn.add(id, m["part"].num().toInt(), m["parts"].num().toInt(), m["text"].str(ClipboardAssembler.PIECE))
                if (t != null && permissions.clipboard) events.clipboard?.invoke(t)
            }
            "chat" -> {
                val t = m["text"].str(MAX_CHAT)
                if (t.isNotEmpty()) events.chat?.invoke(t)
            }
            "typing" -> events.typing?.invoke(m["on"].isTrue())
            "session_end" -> {
                val r = m["reason"].str(20)
                finish(SessionStatus.Ended(if (r in END_REASONS) r else "host"))
            }
            else -> linkMessage(m)
        }
    }

    /** The link's messages: the computer's proof, the token, the password. */
    private fun linkMessage(m: JsonObject) {
        val d = door
        when (m["type"].str(40)) {
            "auth_proof" -> {
                if (d !is Door.Reach || nonce.isEmpty()) return
                // proof2 ties the proof to this connection's certificates; a
                // computer that once gave it must always give it, or someone
                // in the middle could fall back to the plain proof.
                val host = fingerprint(offerSdp)
                val mine = fingerprint(answerSdp)
                val canBind = host.isNotEmpty() && mine.isNotEmpty()
                val proof2 = if (m.containsKey("proof2")) m["proof2"].str(100) else null
                val bound = canBind && proof2 != null && same(proof2, linkProofBound(d.link.token, nonce, host, mine))
                val plain = same(m["proof"].str(100), linkProof(d.link.token, nonce))
                val ok = bound || (plain && !d.link.bound && (proof2 == null || !canBind))
                if (m["link_id"].str(64) != d.link.linkId || !ok) {
                    finish(SessionStatus.Untrusted)
                    return
                }
                if (bound && !d.link.bound) events.bound?.invoke()
                nonce = ""
                proven = true
                control(
                    buildJsonObject {
                        put("type", "auth")
                        put("link_id", d.link.linkId)
                        put("token", d.link.token)
                    },
                )
            }
            "auth_ok" -> {
                if (d is Door.Claim) {
                    val device = m["device_id"].str(64)
                    val linkId = m["link_id"].str(64)
                    val token = m["token"].str(200)
                    if (device.isNotEmpty() && linkId.isNotEmpty() && token.isNotEmpty()) {
                        events.linked?.invoke(DeviceLink(device, linkId, token))
                    }
                    return
                }
                if (d !is Door.Reach || !proven || m["link_id"].str(64) != d.link.linkId) return
                authed = true
                early?.let {
                    early = null
                    admit(it)
                }
            }
            "auth_failed", "unlinked" -> {
                if (d is Door.Join) return
                events.unlinked?.invoke()
                finish(SessionStatus.Unlinked)
            }
            "password_required" -> {
                if (d !is Door.Reach || !authed || status.isOver) return
                set(SessionStatus.Password(PasswordPrompt()))
                val p = savedPassword
                if (!p.isNullOrEmpty() && !autoTried) {
                    autoTried = true
                    sendPassword(p)
                }
            }
            "password_result" -> {
                val prompt = (status as? SessionStatus.Password)?.prompt ?: return
                if (m["ok"].isTrue()) {
                    events.passwordAccepted?.invoke(lastPassword)
                    set(SessionStatus.Waiting)
                    return
                }
                if (autoTried && lastPassword == savedPassword) {
                    savedPassword = null
                    events.passwordRejected?.invoke()
                }
                val reason = m["reason"].str(20)
                if (reason == "blocked" || reason == "locked") {
                    finish(SessionStatus.PasswordRefused(reason, m["retry_after"].num().coerceAtLeast(0.0).toInt()))
                    return
                }
                set(SessionStatus.Password(prompt.copy(wrong = true, checking = false, left = m["left"].num().coerceAtLeast(0.0).toInt())))
            }
        }
    }

    /** Sends the unattended password the computer asked for. */
    fun sendPassword(password: String) {
        val prompt = (status as? SessionStatus.Password)?.prompt ?: return
        if (password.isEmpty()) return
        set(SessionStatus.Password(prompt.copy(checking = true)))
        lastPassword = password.take(512)
        control(
            buildJsonObject {
                put("type", "password")
                put("password", lastPassword)
            },
        )
    }

    private fun cursor(m: JsonObject) {
        val id = m["id"].strOrNull()?.take(32) ?: return
        var shape = shapes[id]
        val b64 = m["png"].strOrNull()
        if (shape == null && b64 != null && b64.length < 300 shl 10 && shapes.size < 64) {
            val png = runCatching { Base64.getDecoder().decode(b64) }.getOrNull()
            if (png != null) {
                shape = CursorShape(id, png, m["hot_x"].num(), m["hot_y"].num(), m["width"].num(), m["height"].num(), m["pixels"].num())
                shapes[id] = shape
            }
        }
        shape?.let { events.cursor?.invoke(it) }
    }

    // To the computer

    /** The size the picture is shown at, in device pixels. */
    fun setVideoWant(width: Int, height: Int) = control(
        buildJsonObject {
            put("type", "video_want")
            put("width", width)
            put("height", height)
        },
    )

    fun setQuality(p: Preset, retina: Boolean = false) = control(
        buildJsonObject {
            put("type", "quality")
            put("preset", p.wire)
            put("retina", retina)
        },
    )

    fun selectDisplay(id: Int) = control(
        buildJsonObject {
            put("type", "display_select")
            put("id", id)
        },
    )

    /** Asks the computer before saving a picture of its screen: yes once the person there is shown it. */
    fun askScreenshot(done: (Boolean) -> Unit) {
        if (!permissions.record) return done(false)
        ask(buildJsonObject { put("type", "screenshot") }, done)
    }

    /** Asks the computer to show that this viewer records: yes once the person there sees it, and only then may it start. */
    fun startRecording(done: (Boolean) -> Unit) {
        if (!permissions.record) return done(false)
        ask(
            buildJsonObject {
                put("type", "recording")
                put("on", true)
            },
            done,
        )
    }

    /** Tells the computer the recording stopped. */
    fun stopRecording() {
        if (!recording) return
        recording = false
        if (!status.isOver) stopMessage()
    }

    private fun stopMessage() = control(
        buildJsonObject {
            put("type", "recording")
            put("id", 0)
            put("on", false)
        },
    )

    private fun ask(msg: JsonObject, done: (Boolean) -> Unit) {
        if (status.isOver) return done(false)
        askSeq = if (askSeq >= 0xffff_ffffL) 1 else askSeq + 1
        val id = askSeq
        asks[id] = done
        askTimers[id] = scope.launch {
            delay(ASK_TIMEOUT_MS)
            answer(id, false)
        }
        control(JsonObject(msg + ("id" to JsonPrimitive(id))))
    }

    private fun answer(id: Long, yes: Boolean) {
        askTimers.remove(id)?.cancel()
        asks.remove(id)?.invoke(yes)
    }

    /** true: the computer draws its pointer into the picture; false: it sends its image and position. */
    fun setCursorInVideo(on: Boolean) = control(
        buildJsonObject {
            put("type", "cursor_mode")
            put("in_video", on)
        },
    )

    /** The viewer's report every 2 seconds: video loss and whether the picture is hidden. */
    fun report(lossPct: Double, hidden: Boolean) = control(
        buildJsonObject {
            put("type", "client_report")
            put(
                "metrics",
                buildJsonObject {
                    put("freeze_ms", 0)
                    put("video_loss_pct", lossPct)
                    put("jitter_ms", 0)
                    put("hidden", hidden)
                },
            )
        },
    )

    /** Tells the computer this side is writing a chat message (on), or stopped. */
    fun sendTyping(on: Boolean) {
        if (status.isOver) return
        control(
            buildJsonObject {
                put("type", "typing")
                put("on", on)
            },
        )
    }

    fun sendChat(text: String): Boolean {
        val t = text.trim().take(MAX_CHAT)
        if (t.isEmpty() || status.isOver) return false
        control(
            buildJsonObject {
                put("type", "chat")
                put("text", t)
            },
        )
        return true
    }

    /** Sends this device's clipboard; paste asks the computer to paste it where its cursor is. */
    fun sendClipboard(text: String, paste: Boolean): Boolean {
        if (!permissions.clipboard || !live || text.toByteArray(Charsets.UTF_8).size > ClipboardAssembler.MAX_TEXT) return false
        clipSeq = (clipSeq + 1) and 0xFFFFFFFFL
        for (piece in ClipboardAssembler.pieces(clipSeq, text, paste)) control(piece)
        return true
    }

    /** The pointer at x, y (0..1 over the current display). */
    fun pointer(x: Double, y: Double) {
        if (!canControl) return
        pointerSeq = (pointerSeq + 1) and 0xFFFF
        peer?.sendPointer(InputPacket.pointer(pointerSeq, displayIndex, InputPacket.coord(x), InputPacket.coord(y)))
    }

    fun button(b: InputPacket.Button, down: Boolean, x: Double, y: Double) {
        if (!canControl) return
        peer?.sendInput(InputPacket.buttonEvent(b, down, displayIndex, InputPacket.coord(x), InputPacket.coord(y)))
    }

    fun wheel(dx: Double, dy: Double) {
        if (!canControl) return
        fun c(v: Double) = Math.round(v).toInt().coerceIn(-32767, 32767)
        peer?.sendInput(InputPacket.wheelEvent(c(dx), c(dy), lines = false))
    }

    fun input(packet: ByteArray) {
        if (!canControl) return
        peer?.sendInput(packet)
    }

    val canControl: Boolean get() = live && permissions.control

    private val live: Boolean get() = admitted && !status.isOver

    /** The current display's index in the computer's list (what the input packets carry). */
    private val displayIndex: Int get() = displays.indexOfFirst { it.id == currentDisplay }.coerceAtLeast(0)

    private fun control(msg: JsonObject) {
        val text = msg.toString()
        val p = peer
        if (!controlOpen || p == null) {
            if (pendingControl.size < 20) pendingControl += text
            return
        }
        p.sendControl(text)
    }

    // State

    private fun set(s: SessionStatus) {
        if (status == s) return
        status = s
        emit()
    }

    private fun emit() {
        events.status?.invoke(status)
    }

    private fun finish(s: SessionStatus) {
        if (status.isOver) return
        joinTimer?.cancel()
        status = s
        for (id in asks.keys.toList()) answer(id, false)
        if (recording) {
            recording = false
            events.recordingStopped?.invoke()
        }
        removeListener?.invoke()
        removeListener = null
        stateJob?.cancel()
        val p = peer
        peer = null
        p?.close()
        signal.close()
        emit()
    }

    companion object {
        val CODECS = listOf("h264-high", "h264", "vp8")
        const val JOIN_TIMEOUT_MS = 20_000L

        /** How long the computer has to answer a screenshot or recording request. */
        const val ASK_TIMEOUT_MS = 5_000L
        const val MAX_CHAT = 2000
        private val END_REASONS = setOf("host", "viewer", "network", "signal", "idle", "revoked")

        /** The proof a linked computer gives: base64url of HMAC-SHA256 keyed with SHA-256(token) (protocol/vectors/link-proof.json). */
        fun linkProof(token: String, nonce: String): String {
            val key = MessageDigest.getInstance("SHA-256").digest(token.toByteArray(Charsets.UTF_8))
            val mac = Mac.getInstance("HmacSHA256")
            mac.init(SecretKeySpec(key, "HmacSHA256"))
            val out = mac.doFinal(("go-link-desk link proof\n" + nonce).toByteArray(Charsets.UTF_8))
            return Base64.getUrlEncoder().withoutPadding().encodeToString(out)
        }

        /**
         * The proof tied to the connection too: HMAC over the nonce and the
         * DTLS fingerprints of the computer's and this viewer's certificates
         * (protocol/vectors/link-proof.json, bound_cases).
         */
        fun linkProofBound(token: String, nonce: String, host: String, viewer: String): String {
            val key = MessageDigest.getInstance("SHA-256").digest(token.toByteArray(Charsets.UTF_8))
            val mac = Mac.getInstance("HmacSHA256")
            mac.init(SecretKeySpec(key, "HmacSHA256"))
            val out = mac.doFinal(("go-link-desk link proof 2\n" + nonce + "\n" + host + "\n" + viewer).toByteArray(Charsets.UTF_8))
            return Base64.getUrlEncoder().withoutPadding().encodeToString(out)
        }

        /** An SDP's first sha-256 certificate fingerprint as "sha-256 AB:CD:…", or "" without one. */
        fun fingerprint(sdp: String): String {
            for (raw in sdp.lineSequence()) {
                val line = raw.trim()
                if (!line.startsWith("a=fingerprint:")) continue
                val parts = line.removePrefix("a=fingerprint:").trim().split(Regex("\\s+"), limit = 2)
                if (parts.size == 2 && parts[0].equals("sha-256", ignoreCase = true)) return "sha-256 " + parts[1].trim().uppercase()
            }
            return ""
        }

        /** Compares two proofs in constant time. */
        fun same(a: String, b: String): Boolean = MessageDigest.isEqual(a.toByteArray(Charsets.UTF_8), b.toByteArray(Charsets.UTF_8))

        /** 32 random bytes, base64url. */
        fun randomNonce(): String {
            val b = ByteArray(32)
            SecureRandom().nextBytes(b)
            return Base64.getUrlEncoder().withoutPadding().encodeToString(b)
        }
    }
}

/** The clipboard's pieces (docs/protocol.md, "clipboard"). */
class ClipboardAssembler {
    private var id = 0L
    private val parts = mutableListOf<String>()

    /** Takes a piece; the whole text once the last one arrives. */
    fun add(id: Long, part: Int, parts: Int, text: String): String? {
        if (parts < 1 || parts > MAX_TEXT / PIECE + 1 || part < 0 || part >= parts) return null
        if (part == 0 || id != this.id) {
            this.id = id
            this.parts.clear()
        }
        if (part != this.parts.size) {
            this.parts.clear()
            return null
        }
        this.parts += text
        if (this.parts.size != parts) return null
        val out = this.parts.joinToString("")
        this.parts.clear()
        return if (out.toByteArray(Charsets.UTF_8).size <= MAX_TEXT) out else null
    }

    companion object {
        const val PIECE = 16 shl 10
        const val MAX_TEXT = 1 shl 20

        /** A text cut into pieces of at most 16 KiB of UTF-8, never splitting a character. */
        fun pieces(id: Long, text: String, paste: Boolean): List<JsonObject> {
            val chunks = mutableListOf<String>()
            var bytes = text.toByteArray(Charsets.UTF_8)
            while (bytes.size > PIECE) {
                var cut = PIECE
                while (cut > 0 && (bytes[cut].toInt() and 0xC0) == 0x80) cut--
                chunks += String(bytes, 0, cut, Charsets.UTF_8)
                bytes = bytes.copyOfRange(cut, bytes.size)
            }
            chunks += String(bytes, Charsets.UTF_8)
            return chunks.mapIndexed { i, c ->
                buildJsonObject {
                    put("type", "clipboard")
                    put("id", id)
                    put("part", i)
                    put("parts", chunks.size)
                    put("text", c)
                    if (paste && i == chunks.size - 1) put("paste", true)
                }
            }
        }
    }
}
