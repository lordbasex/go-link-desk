// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.app.Activity
import android.app.Application
import android.content.Context
import android.os.Build
import android.provider.Settings
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.launch
import org.golink.desk.core.CodeInput
import org.golink.desk.core.DeviceLink
import org.golink.desk.core.Door
import org.golink.desk.core.Preset
import org.golink.desk.core.Recent
import org.golink.desk.core.Recents
import org.golink.desk.core.SignalUrls

/**
 * What the app keeps between launches (this app's preferences only): the
 * name the person at the computer sees, the recent computers, the
 * signaling server, and how the screen is handled. The Android
 * counterpart of the iOS app's AppModel.
 */
class AppModel(application: Application) : AndroidViewModel(application) {
    private val prefs = application.getSharedPreferences("desk", Context.MODE_PRIVATE)

    var name by mutableStateOf(prefs.getString("name", "") ?: "")
        private set
    var recents by mutableStateOf(Recents.decode(prefs.getString("recents", null)))
        private set

    /** A custom signaling server ("" for the official one). */
    var customSignal by mutableStateOf(prefs.getString("signal", "") ?: "")
        private set

    /** Trackpad (the default) or touch: how a finger moves the pointer. */
    var touchMode by mutableStateOf(prefs.getBoolean("touch_mode", false))
        private set
    var preset by mutableStateOf(Preset.entries.firstOrNull { it.wire == prefs.getString("preset", "") } ?: Preset.BALANCED)
        private set

    /** A sound when a chat message comes (on by default). */
    var chatSound by mutableStateOf(prefs.getBoolean("chat_sound", true))
        private set
    var session by mutableStateOf<SessionModel?>(null)
        private set
    val links = LinkStore(application)

    /** Bumped when the linked computers change, so My computers redraws. */
    var linksVersion by mutableIntStateOf(0)
        private set

    /** Why the last link or unlock failed, for Home. */
    var linkError by mutableStateOf<String?>(null)

    fun updateName(v: String) {
        name = v
        prefs.edit().putString("name", v).apply()
    }

    fun updateSignal(v: String) {
        customSignal = v
        prefs.edit().putString("signal", v).apply()
    }

    fun updateTouchMode(v: Boolean) {
        touchMode = v
        prefs.edit().putBoolean("touch_mode", v).apply()
    }

    fun updatePreset(v: Preset) {
        preset = v
        prefs.edit().putString("preset", v.wire).apply()
    }

    fun updateChatSound(v: Boolean) {
        chatSound = v
        prefs.edit().putBoolean("chat_sound", v).apply()
    }

    var guideSeen: Boolean
        get() = prefs.getBoolean("guide_seen", false)
        set(v) {
            prefs.edit().putBoolean("guide_seen", v).apply()
        }

    /** The server in use: a valid custom one, else the official one. */
    val signalURL: String get() = SignalUrls.resolve(customSignal.ifEmpty { null }, allowDevHosts = BuildConfig.DEBUG).url

    /** This device's name, as the system shows it. */
    val deviceName: String
        get() = Settings.Global.getString(getApplication<Application>().contentResolver, Settings.Global.DEVICE_NAME)
            ?: "${Build.MANUFACTURER} ${Build.MODEL}"

    /** The name sent with the knock: the one typed, else the device's. */
    val knockName: String get() = name.trim().ifEmpty { deviceName }

    /** "go-link Desk 1.0.0 on Android 16": what the person at the computer sees as the browser. */
    val agent: String get() = "go-link Desk ${BuildConfig.VERSION_NAME} on Android ${Build.VERSION.RELEASE}"

    val version: String get() = "v${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})" + if (BuildConfig.DEBUG) " · debug" else ""

    fun connect(code: String) {
        val digits = Recents.digits(code) ?: return
        open(Door.Join(digits), CodeInput.group(digits))
    }

    /** Links this device to a computer with its pairing code (Settings › Unattended access there). */
    fun link(code: String) {
        if (!links.canProtect) {
            linkError = text(R.string.link_no_lock)
            return
        }
        val digits = Recents.digits(code) ?: return
        open(Door.Claim(digits), CodeInput.group(digits))
    }

    /** Comes back to a linked computer: the fingerprint, face or screen lock first. */
    fun reach(activity: Activity, c: LinkedComputer) {
        if (session != null) return
        val name = c.name.ifEmpty { text(R.string.the_computer) }
        viewModelScope.launch {
            try {
                val (link, password) = links.unlock(activity, c, text(R.string.connect_to_x, name), text(R.string.unlock_subtitle))
                open(Door.Reach(link), c.name, password)
            } catch (e: LinkStore.LinkError) {
                if (e.kind != LinkStore.LinkError.Kind.CANCELLED) linkError = errorText(e)
            }
        }
    }

    /** What went wrong with a link, for Home. */
    private fun errorText(e: LinkStore.LinkError): String = when (e.kind) {
        LinkStore.LinkError.Kind.NO_PASSCODE -> text(R.string.link_no_lock)
        LinkStore.LinkError.Kind.CANCELLED -> text(R.string.cancelled)
        LinkStore.LinkError.Kind.KEYSTORE -> text(R.string.link_keystore) + if (e.detail.isNotEmpty()) " (${e.detail})" else ""
    }

    private fun text(id: Int, vararg args: Any): String = getApplication<Application>().getString(id, *args)

    /** "your fingerprint or face", or "the screen lock" without them: what opens My computers. */
    val unlockName: String get() = text(if (links.hasBiometrics) R.string.unlock_biometrics else R.string.unlock_screen_lock)

    private fun open(door: Door, label: String, password: String? = null) {
        if (session != null) return
        val s = SessionModel(this, door, label)
        session = s
        s.start(password)
    }

    /** Keeps a link made with the pairing code (again when the computer says its name). */
    fun link(link: DeviceLink, name: String, os: String) {
        try {
            links.save(link, name, os)
            session?.justLinked = true
        } catch (e: LinkStore.LinkError) {
            linkError = errorText(e)
        }
        linksVersion++
    }

    fun touchLink(deviceId: String, name: String, os: String) {
        links.update(deviceId) { it.copy(name = name, os = os, lastUsed = System.currentTimeMillis()) }
        linksVersion++
    }

    fun forgetLink(deviceId: String) {
        links.forget(deviceId)
        linksVersion++
    }

    fun remember(r: Recent) {
        recents = Recents.add(r, recents)
        save()
    }

    fun forget(code: String) {
        recents = recents.filter { it.code != code }
        save()
    }

    fun clearRecents() {
        recents = emptyList()
        save()
    }

    /** Back to Home. */
    fun closeSession() {
        session = null
    }

    private fun save() {
        prefs.edit().putString("recents", Recents.encode(recents)).apply()
    }

    override fun onCleared() {
        session?.hangUp()
    }
}
