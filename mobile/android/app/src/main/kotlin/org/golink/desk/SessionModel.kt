// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.content.ClipData
import android.content.ClipboardManager
import android.os.Handler
import android.os.Looper
import android.os.VibrationEffect
import android.os.Vibrator
import android.util.Log
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.viewModelScope
import org.golink.desk.core.CursorShape
import org.golink.desk.core.DeskSession
import org.golink.desk.core.Display
import org.golink.desk.core.Door
import org.golink.desk.core.FilesClient
import org.golink.desk.core.LiveStats
import org.golink.desk.core.Permissions
import org.golink.desk.core.Preset
import org.golink.desk.core.Recent
import org.golink.desk.core.SessionStatus
import org.golink.desk.core.SignalClient
import org.golink.desk.core.StatsMeter
import org.golink.desk.net.OkHttpSockets
import org.golink.desk.rtc.DeskPeers
import org.golink.desk.rtc.MediaListener
import org.golink.desk.rtc.WebRtcEngine
import org.webrtc.AudioTrack
import org.webrtc.VideoTrack

/** One line of the session's chat. */
data class ChatLine(val id: Long, val fromHost: Boolean, val text: String, val at: Long)

/**
 * One session as the screens see it: DeskSession with its own signalhub
 * connection and libwebrtc peer, the picture's track, what the computer
 * says (its name, displays, pointer) and the numbers of the details panel.
 * The Android counterpart of the iOS app's SessionModel.
 */
class SessionModel(private val app: AppModel, val door: Door, val label: String) : MediaListener {
    private val context = app.getApplication<android.app.Application>()
    private val main = Handler(Looper.getMainLooper())
    val engine = WebRtcEngine.get(context)
    private val peers = DeskPeers(engine, app.viewModelScope)
    lateinit var session: DeskSession
        private set

    /** The session code, for a session by code ("" otherwise). */
    val code: String get() = (door as? Door.Join)?.code ?: ""

    var status by mutableStateOf<SessionStatus>(SessionStatus.Joining)
        private set
    var hostName by mutableStateOf("")
        private set
    var hostOS by mutableStateOf("")
        private set
    var permissions by mutableStateOf(Permissions())
        private set
    var displays by mutableStateOf<List<Display>>(emptyList())
        private set
    var currentDisplay by mutableIntStateOf(0)
        private set
    var video by mutableStateOf<VideoTrack?>(null)
        private set
    var stats by mutableStateOf(LiveStats())
        private set
    var showStats by mutableStateOf(false)
    var keyboard by mutableStateOf(false)
    var muted by mutableStateOf(false)
        private set

    /** The computer's files, once its channel opens (with the files permission). */
    var files by mutableStateOf<FilesModel?>(null)
        private set
    var showFiles by mutableStateOf(false)
    var showGuide by mutableStateOf(false)

    /** The chat: its lines, how many came while it was closed, and the last one as a toast. */
    val chat = mutableStateListOf<ChatLine>()
    var unread by mutableIntStateOf(0)
        private set
    var showChat by mutableStateOf(false)
        private set
    var toast by mutableStateOf<String?>(null)
    private val hideToast = Runnable { toast = null }

    /** The person at the computer is writing a message. */
    var hostTyping by mutableStateOf(false)
        private set
    private val hostStopped = Runnable { hostTyping = false }

    /** This side writing: "typing" again every few seconds, "stopped" after a pause. */
    private var typingSentAt = 0L
    private val typingPause = Runnable { quiet() }

    /** The link made with the pairing code, saved again when the computer says its name. */
    private var pendingLink: org.golink.desk.core.DeviceLink? = null

    /** Linked in this session, to say so. */
    var justLinked by mutableStateOf(false)

    /** Locked to watch: the screen sends nothing until it is unlocked. */
    var locked by mutableStateOf(false)
        private set

    /** The computer's pointer image and where it is (0..1), when it moves by itself. */
    var cursor by mutableStateOf<CursorShape?>(null)
        private set
    var onHostPointer: ((Double, Double, Boolean) -> Unit)? = null

    private var audio: AudioTrack? = null
    private val meter = StatsMeter()
    private var ticking = false
    private val tick = object : Runnable {
        override fun run() {
            if (!ticking) return
            sample()
            main.postDelayed(this, 2000)
        }
    }

    /** The app is in the background: the picture is sent smaller. */
    var hidden = false
    private var nextLine = 0L

    /** password: the computer's unattended password kept with its link, sent by itself when asked. */
    fun start(password: String?) {
        peers.media = this
        val signal = SignalClient(app.signalURL, OkHttpSockets(), app.viewModelScope)
        val s = DeskSession(door, app.knockName, app.agent, signal, peers, app.viewModelScope, engine.codecs)
        session = s
        val ev = s.events
        ev.status = { statusChanged(it) }
        ev.host = { name, os ->
            hostName = name
            hostOS = os
            when (door) {
                is Door.Join -> app.remember(Recent(door.code, name, os, System.currentTimeMillis()))
                is Door.Reach -> app.touchLink(door.link.deviceId, name, os)
                is Door.Claim -> pendingLink?.let { app.link(it, name, os) }
            }
        }
        ev.permissions = { permissions = it }
        ev.displays = { d, current ->
            displays = d
            currentDisplay = current
        }
        ev.cursor = { cursor = it }
        ev.cursorPos = { x, y, visible -> onHostPointer?.invoke(x, y, visible) }
        ev.clipboard = { text ->
            context.getSystemService(ClipboardManager::class.java).setPrimaryClip(ClipData.newPlainText("go-link Desk", text))
        }
        ev.chat = { received(it) }
        ev.typing = { hostIsTyping(it) }
        ev.linked = { link ->
            pendingLink = link
            app.link(link, hostName, hostOS)
        }
        s.savedPassword = password
        ev.passwordAccepted = { pw -> (door as? Door.Reach)?.let { app.links.setPassword(it.link, pw) } }
        ev.passwordRejected = { (door as? Door.Reach)?.let { app.links.setPassword(it.link, null) } }
        ev.unlinked = { (door as? Door.Reach)?.let { app.forgetLink(it.link.deviceId) } }
        Log.i(TAG, "codecs ${engine.codecs}")
        s.start()
    }

    private fun statusChanged(st: SessionStatus) {
        status = st
        Log.i(TAG, "status $st")
        if ((st == SessionStatus.Connecting || st == SessionStatus.Live) && !ticking) {
            session.setQuality(app.preset)
            // The pointer comes as an image and a position, drawn here at once.
            session.setCursorInVideo(false)
            ticking = true
            main.postDelayed(tick, 2000)
        }
        if (st.isOver) {
            ticking = false
            main.removeCallbacks(tick)
        } else if (st == SessionStatus.Live && !app.guideSeen) {
            // The first time, how to use it.
            app.guideSeen = true
            showGuide = true
        }
    }

    private fun sample() {
        peers.current?.sample { s ->
            stats = meter.next(s)
            session.report(stats.lossPct, hidden)
        }
    }

    private fun hostIsTyping(on: Boolean) {
        main.removeCallbacks(hostStopped)
        hostTyping = on
        // Without news for a while, it stopped.
        if (on) main.postDelayed(hostStopped, 6000)
    }

    /** The chat's line changed: tell the computer this side is writing, or stopped. */
    fun wrote(text: String) {
        if (text.isBlank()) {
            quiet()
            return
        }
        val now = System.currentTimeMillis()
        if (typingSentAt == 0L || now - typingSentAt >= 3000) {
            typingSentAt = now
            session.sendTyping(true)
        }
        main.removeCallbacks(typingPause)
        main.postDelayed(typingPause, 4000)
    }

    fun quiet() {
        main.removeCallbacks(typingPause)
        if (typingSentAt != 0L) {
            typingSentAt = 0L
            session.sendTyping(false)
        }
    }

    private fun received(text: String) {
        hostIsTyping(false) // the message it was writing came
        chat += ChatLine(nextLine++, true, text, System.currentTimeMillis())
        // The message sound (Settings › Message sound).
        if (app.chatSound) ChatSound.play(context)
        if (chat.size > 500) chat.removeRange(0, chat.size - 500)
        if (showChat) return
        unread++
        toast = text
        main.removeCallbacks(hideToast)
        main.postDelayed(hideToast, 5000)
        context.getSystemService(Vibrator::class.java)?.vibrate(VibrationEffect.createPredefined(VibrationEffect.EFFECT_TICK))
    }

    fun openChat(on: Boolean) {
        showChat = on
        if (on) {
            unread = 0
            toast = null
        }
    }

    /** Sends a chat line; false when there is nothing to send or the session is over. */
    fun say(text: String): Boolean {
        if (!session.sendChat(text)) return false
        quiet()
        chat += ChatLine(nextLine++, false, text.trim(), System.currentTimeMillis())
        return true
    }

    fun hangUp() = session.hangUp()

    /** Back to Home. */
    fun close() {
        session.hangUp()
        main.removeCallbacksAndMessages(null)
        app.closeSession()
    }

    fun setPreset(p: Preset) {
        app.updatePreset(p)
        session.setQuality(p)
    }

    val preset: Preset get() = app.preset

    var touchMode: Boolean
        get() = app.touchMode
        set(v) = app.updateTouchMode(v)

    fun selectDisplay(id: Int) {
        currentDisplay = id
        session.selectDisplay(id)
    }

    fun mute(on: Boolean) {
        muted = on
        audio?.setEnabled(!on)
    }

    fun lock(on: Boolean) {
        locked = on
        if (on) keyboard = false
    }

    /** Sends this device's clipboard to the computer and pastes it there. */
    fun pasteHere() {
        val clip = context.getSystemService(ClipboardManager::class.java).primaryClip
        val text = clip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.coerceToText(context)?.toString()
        if (!text.isNullOrEmpty()) session.sendClipboard(text, paste = true)
    }

    // MediaListener

    override fun onVideo(track: VideoTrack) {
        Log.i(TAG, "video track")
        video = track
    }

    override fun onAudio(track: AudioTrack) {
        audio = track
        track.setEnabled(!muted)
    }

    override fun onFiles(client: FilesClient) {
        files = FilesModel(context, client)
    }

    override fun onMediaClosed() {
        main.post {
            video = null
            audio = null
            files = null
            showFiles = false
        }
    }

    companion object {
        /** The session's steps in the log (never a code or a name). */
        const val TAG = "go-link-desk"
    }
}
