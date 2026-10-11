// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import android.view.HapticFeedbackConstants
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Chat
import androidx.compose.material.icons.automirrored.outlined.HelpOutline
import androidx.compose.material.icons.automirrored.outlined.VolumeOff
import androidx.compose.material.icons.automirrored.outlined.VolumeUp
import androidx.compose.material.icons.outlined.BarChart
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Keyboard
import androidx.compose.material.icons.outlined.KeyboardArrowDown
import androidx.compose.material.icons.outlined.KeyboardArrowUp
import androidx.compose.material.icons.outlined.KeyboardCommandKey
import androidx.compose.material.icons.outlined.KeyboardHide
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Monitor
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import kotlinx.coroutines.delay
import org.golink.desk.R
import org.golink.desk.SessionModel
import org.golink.desk.core.InputPacket
import org.golink.desk.core.LiveStats
import org.golink.desk.core.PasswordPrompt
import org.golink.desk.core.Preset
import org.golink.desk.core.SessionStatus
import org.golink.desk.core.Shortcut

private val glass = Color(0xD91C1C1E)

/** A session: the computer's screen, a small toolbar over it, and what the session is doing. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun SessionScreen(model: SessionModel, unlockName: String) {
    DeskTheme(dark = true) {
        val status = model.status
        var barHidden by remember { mutableStateOf(false) }
        var screen by remember { mutableStateOf<RemoteScreenView?>(null) }
        var latched by remember { mutableIntStateOf(0) }
        val view = LocalView.current
        val imeVisible = WindowInsets.isImeVisible
        var imeWasVisible by remember { mutableStateOf(false) }

        // The screen stays on while the computer is shown.
        DisposableEffect(Unit) {
            view.keepScreenOn = true
            onDispose { view.keepScreenOn = false }
        }
        // The picture is sent smaller while the app is in the background.
        LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
            model.hidden = true
            screen?.releaseAll()
        }
        LifecycleEventEffect(Lifecycle.Event.ON_START) { model.hidden = false }
        // The person closed the keyboard (Back, or its own button).
        LaunchedEffect(imeVisible) {
            if (imeVisible) imeWasVisible = true else if (imeWasVisible && model.keyboard) {
                imeWasVisible = false
                model.keyboard = false
            }
        }
        BackHandler { if (status.isOver) model.close() else model.hangUp() }

        BoxWithConstraints(Modifier.fillMaxSize().background(Color.Black)) {
            val narrow = maxWidth < 600.dp
            val showPicture = status == SessionStatus.Live || status == SessionStatus.Reconnecting || (status == SessionStatus.Connecting && model.video != null)
            if (showPicture) {
                AndroidView(
                    factory = { ctx ->
                        RemoteScreenView(ctx, model.engine.eglBase.eglBaseContext).also { v ->
                            v.session = model.session
                            v.onLatch = { latched = it }
                            model.onHostPointer = { x, y, visible -> v.hostPointer(x, y, visible) }
                            screen = v
                        }
                    },
                    update = { v ->
                        v.show(model.video)
                        v.touchMode = model.touchMode
                        v.locked = model.locked
                        v.setCursor(model.cursor, model.displays.firstOrNull { it.id == model.currentDisplay }?.pointsWidth ?: 0)
                        v.setSoftKeyboard(model.keyboard)
                        if (model.status.isOver) v.releaseAll()
                    },
                    onRelease = { v ->
                        v.releaseAll()
                        v.release()
                        model.onHostPointer = null
                        screen = null
                    },
                    modifier = Modifier.fillMaxSize(),
                )
            }
            Box(Modifier.fillMaxSize().safeDrawingPadding(), contentAlignment = Alignment.TopCenter) {
                when {
                    status is SessionStatus.Password -> PasswordCard(status.prompt, onSend = { model.session.sendPassword(it) }, onCancel = { model.close() })
                    else -> statusText(model)?.let { StatusCard(it, busy = !status.isOver, over = status.isOver) { model.close() } }
                }
                if (model.justLinked && status == SessionStatus.Live) {
                    Pill(Icons.Outlined.Link, stringResource(R.string.linked_notice, unlockName), Modifier.padding(top = 56.dp).clickable { model.justLinked = false })
                    LaunchedEffect(Unit) {
                        delay(6000)
                        model.justLinked = false
                    }
                }
                if (!status.isOver && status != SessionStatus.Joining && status != SessionStatus.Waiting) {
                    if (model.locked) {
                        LockBadge(Modifier.align(Alignment.TopEnd)) { model.lock(false) }
                    } else {
                        Toolbar(model, narrow, barHidden) { barHidden = it }
                    }
                }
                if (model.showStats && status == SessionStatus.Live) {
                    StatsPanel(model.stats, Modifier.align(Alignment.TopStart).padding(top = 64.dp, start = 12.dp))
                }
                AnimatedVisibility(
                    visible = model.toast != null && !model.showChat,
                    enter = slideInVertically() + fadeIn(),
                    exit = slideOutVertically() + fadeOut(),
                    modifier = Modifier.padding(top = 56.dp),
                ) {
                    Pill(Icons.AutoMirrored.Outlined.Chat, model.toast ?: "", Modifier.clickable { model.openChat(true) })
                }
            }
            if (model.keyboard && model.permissions.control && !model.locked) {
                Box(Modifier.fillMaxSize().imePadding(), contentAlignment = Alignment.BottomCenter) {
                    KeyBar(mac = model.hostOS == "darwin", latched = latched, onKey = { screen?.press(it) }, onLatch = { screen?.toggleLatch(it) })
                }
            }
        }
        if (model.showGuide) GuideSheet(model.touchMode) { model.showGuide = false }
        if (model.showChat) ChatSheet(model) { model.openChat(false) }
        val files = model.files
        if (model.showFiles && files != null) FilesSheet(files) { model.showFiles = false }
    }
}

@Composable
private fun statusText(model: SessionModel): String? = when (val s = model.status) {
    SessionStatus.Joining -> stringResource(R.string.st_connecting)
    SessionStatus.Waiting -> stringResource(R.string.st_waiting)
    SessionStatus.Connecting -> if (model.video == null) stringResource(R.string.st_accepted) else null
    SessionStatus.Live -> null
    SessionStatus.Reconnecting -> stringResource(R.string.st_reconnecting)
    is SessionStatus.Rejected -> when (s.reason) {
        "timeout" -> stringResource(R.string.st_rej_timeout)
        "busy" -> stringResource(R.string.st_rej_busy)
        "off", "unavailable" -> stringResource(R.string.st_rej_off)
        else -> stringResource(R.string.st_rej_declined)
    }
    is SessionStatus.Ended -> when (s.reason) {
        "viewer" -> stringResource(R.string.st_end_viewer)
        "network", "signal" -> stringResource(R.string.st_end_network)
        "idle" -> stringResource(R.string.st_end_idle)
        else -> stringResource(R.string.st_end_host)
    }
    SessionStatus.NotFound -> stringResource(R.string.st_not_found)
    SessionStatus.RateLimited -> stringResource(R.string.st_rate_limited)
    SessionStatus.Unreachable -> stringResource(R.string.st_unreachable)
    is SessionStatus.Password -> null
    SessionStatus.Offline -> stringResource(R.string.st_offline)
    SessionStatus.Unlinked -> stringResource(R.string.st_unlinked)
    SessionStatus.Untrusted -> stringResource(R.string.st_untrusted)
    is SessionStatus.PasswordRefused ->
        if (s.reason == "locked") stringResource(R.string.st_pw_locked, maxOf(1, s.retryAfter / 60)) else stringResource(R.string.st_pw_blocked)
}

/** The toolbar over the screen, in a capsule; on a phone held upright the name goes, and lock and help move to the screen menu. */
@Composable
private fun Toolbar(model: SessionModel, narrow: Boolean, hidden: Boolean, setHidden: (Boolean) -> Unit) {
    Row(
        Modifier.padding(top = 6.dp).background(glass, CircleShape).padding(horizontal = 8.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (hidden) {
            BarButton(Icons.Outlined.KeyboardArrowDown, stringResource(R.string.show_toolbar)) { setHidden(false) }
            return@Row
        }
        BarButton(Icons.Outlined.Close, stringResource(R.string.disconnect), tint = Theme.danger) { model.hangUp() }
        if (!narrow) {
            Text(
                model.hostName.ifEmpty { model.label },
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                color = Color.White,
                maxLines = 1,
                modifier = Modifier.padding(horizontal = 6.dp).widthIn(max = 200.dp),
            )
            Box(Modifier.width(1.dp).height(20.dp).background(Color.White.copy(alpha = 0.25f)))
        }
        if (model.permissions.control) {
            BarButton(if (model.keyboard) Icons.Outlined.KeyboardHide else Icons.Outlined.Keyboard, stringResource(R.string.keyboard)) {
                model.keyboard = !model.keyboard
            }
            KeysMenu(model)
        }
        Box {
            BarButton(Icons.AutoMirrored.Outlined.Chat, stringResource(R.string.chat)) { model.openChat(true) }
            if (model.unread > 0) {
                Text(
                    "${minOf(model.unread, 99)}",
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    color = Color.White,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.align(Alignment.TopEnd).offset(x = 2.dp, y = 2.dp)
                        .background(Color.Red, CircleShape).defaultMinSize(minWidth = 16.dp).padding(horizontal = 4.dp),
                )
            }
        }
        if (model.permissions.files && model.files != null) {
            BarButton(Icons.Outlined.Folder, stringResource(R.string.files)) { model.showFiles = true }
        }
        ScreenMenu(model, narrow)
        if (!narrow) {
            BarButton(Icons.Outlined.Lock, stringResource(R.string.lock)) { model.lock(true) }
            BarButton(Icons.AutoMirrored.Outlined.HelpOutline, stringResource(R.string.how_to_use)) { model.showGuide = true }
        }
        BarButton(Icons.Outlined.KeyboardArrowUp, stringResource(R.string.hide_toolbar)) { setHidden(true) }
    }
}

@Composable
private fun BarButton(icon: ImageVector, label: String, tint: Color = Color.White, onClick: () -> Unit) {
    Box(
        Modifier.size(40.dp).clip(CircleShape).clickable(onClick = onClick).semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, null, tint = tint, modifier = Modifier.size(22.dp))
    }
}

/** A menu item with a check when it is the one in use. */
@Composable
private fun Choice(text: String, on: Boolean, onClick: () -> Unit) {
    DropdownMenuItem(
        text = { Text(text) },
        leadingIcon = { if (on) Icon(Icons.Outlined.Check, null, tint = Theme.accent) else Spacer(Modifier.size(24.dp)) },
        onClick = onClick,
    )
}

@Composable
private fun MenuHeader(text: String) {
    Text(text.uppercase(), fontSize = 12.sp, color = colors.secondary, modifier = Modifier.padding(start = 16.dp, top = 8.dp, bottom = 4.dp))
}

@Composable
private fun KeysMenu(model: SessionModel) {
    var open by remember { mutableStateOf(false) }
    Box {
        BarButton(Icons.Outlined.KeyboardCommandKey, stringResource(R.string.special_keys)) { open = true }
        DropdownMenu(open, { open = false }) {
            for (s in Shortcut.forSystem(model.hostOS)) {
                DropdownMenuItem(text = { Text(shortcutTitle(s.id)) }, onClick = {
                    open = false
                    for (p in s.packets()) model.session.input(p)
                })
            }
            if (model.permissions.clipboard) {
                HorizontalDivider()
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.paste_clipboard)) },
                    leadingIcon = { Icon(Icons.Outlined.ContentPaste, null) },
                    onClick = {
                        open = false
                        model.pasteHere()
                    },
                )
            }
        }
    }
}

@Composable
private fun ScreenMenu(model: SessionModel, narrow: Boolean) {
    var open by remember { mutableStateOf(false) }
    val close = { open = false }
    Box {
        BarButton(Icons.Outlined.Monitor, stringResource(R.string.screen)) { open = true }
        DropdownMenu(open, close) {
            if (model.displays.size > 1) {
                MenuHeader(stringResource(R.string.display))
                for (d in model.displays) Choice(d.name, d.id == model.currentDisplay) { close(); model.selectDisplay(d.id) }
                HorizontalDivider()
            }
            MenuHeader(stringResource(R.string.picture))
            for (p in Preset.entries) Choice(presetTitle(p), p == model.preset) { close(); model.setPreset(p) }
            if (model.permissions.control) {
                HorizontalDivider()
                MenuHeader(stringResource(R.string.fingers))
                Choice(stringResource(R.string.trackpad), !model.touchMode) { close(); model.touchMode = false }
                Choice(stringResource(R.string.touch), model.touchMode) { close(); model.touchMode = true }
            }
            HorizontalDivider()
            if (model.permissions.audio) {
                DropdownMenuItem(
                    text = { Text(stringResource(if (model.muted) R.string.sound_on else R.string.mute)) },
                    leadingIcon = { Icon(if (model.muted) Icons.AutoMirrored.Outlined.VolumeUp else Icons.AutoMirrored.Outlined.VolumeOff, null) },
                    onClick = { close(); model.mute(!model.muted) },
                )
            }
            if (narrow) {
                DropdownMenuItem(text = { Text(stringResource(R.string.lock)) }, leadingIcon = { Icon(Icons.Outlined.Lock, null) }, onClick = { close(); model.lock(true) })
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.how_to_use)) },
                    leadingIcon = { Icon(Icons.AutoMirrored.Outlined.HelpOutline, null) },
                    onClick = { close(); model.showGuide = true },
                )
            }
            DropdownMenuItem(
                text = { Text(stringResource(if (model.showStats) R.string.hide_details else R.string.show_details)) },
                leadingIcon = { Icon(Icons.Outlined.BarChart, null) },
                onClick = { close(); model.showStats = !model.showStats },
            )
        }
    }
}

@Composable
private fun presetTitle(p: Preset) = stringResource(
    when (p) {
        Preset.SPEED -> R.string.speed
        Preset.BALANCED -> R.string.balanced
        Preset.QUALITY -> R.string.quality
    },
)

@Composable
private fun shortcutTitle(id: String): String = when (id) {
    "copy" -> stringResource(R.string.copy)
    "paste" -> stringResource(R.string.paste)
    "undo" -> stringResource(R.string.undo)
    "select_all" -> stringResource(R.string.select_all)
    "switch_apps" -> stringResource(R.string.switch_apps)
    "spotlight" -> stringResource(R.string.spotlight)
    "force_quit" -> stringResource(R.string.force_quit)
    "lock" -> stringResource(R.string.lock_screen)
    "screenshot" -> stringResource(R.string.screenshot)
    "start" -> stringResource(R.string.start_menu)
    "ctrl_alt_del" -> "Ctrl+Alt+Del"
    "task_manager" -> stringResource(R.string.task_manager)
    else -> id
}

/** Locked: a small lock in a corner; holding it for a second unlocks. */
@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun LockBadge(modifier: Modifier, unlock: () -> Unit) {
    val view = LocalView.current
    var pressing by remember { mutableStateOf(false) }
    Row(
        modifier.padding(top = 6.dp, end = 12.dp)
            .background(Color.Black.copy(alpha = 0.45f), CircleShape)
            .combinedClickable(onClick = { pressing = !pressing }, onLongClick = {
                view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
                unlock()
            })
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Outlined.Lock, null, tint = Color.White.copy(alpha = 0.85f), modifier = Modifier.size(16.dp))
        Text(stringResource(R.string.hold_to_unlock), fontSize = 12.sp, color = Color.White.copy(alpha = 0.85f), modifier = Modifier.padding(start = 6.dp))
    }
}

/** A short notice in a capsule over the screen. */
@Composable
private fun Pill(icon: ImageVector, text: String, modifier: Modifier = Modifier) {
    Row(
        modifier.padding(horizontal = 16.dp).background(Color.Black.copy(alpha = 0.75f), CircleShape).padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = Theme.accent, modifier = Modifier.size(16.dp))
        Text(text, fontSize = 13.sp, color = Color.White, maxLines = 2, modifier = Modifier.padding(start = 8.dp))
    }
}

/** What the session is doing, before the picture or once it is over. */
@Composable
private fun StatusCard(text: String, busy: Boolean, over: Boolean, onClose: () -> Unit) {
    Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.Center) {
        Column(
            Modifier.widthIn(max = 380.dp).background(glass, RoundedCornerShape(20.dp)).padding(28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(18.dp),
        ) {
            if (busy) CircularProgressIndicator(color = Color.White, modifier = Modifier.size(36.dp))
            Text(text, color = Color.White, fontSize = 17.sp, textAlign = TextAlign.Center)
            if (over) {
                Button(onClick = onClose, colors = ButtonDefaults.buttonColors(containerColor = Theme.accent, contentColor = Theme.onAccent)) {
                    Text(stringResource(R.string.back))
                }
            } else {
                OutlinedButton(onClick = onClose) { Text(stringResource(R.string.cancel), color = Color.White) }
            }
        }
    }
}

/** The details: frames a second, size, bitrate, loss, round trip and path. */
@Composable
private fun StatsPanel(stats: LiveStats, modifier: Modifier) {
    Column(modifier.background(Color.Black.copy(alpha = 0.6f), RoundedCornerShape(10.dp)).padding(10.dp)) {
        @Composable
        fun row(k: String, v: String) = Row {
            Text(k, color = Color.White.copy(alpha = 0.6f), fontSize = 12.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.width(48.dp))
            Text(v, color = Color.White, fontSize = 12.sp, fontFamily = FontFamily.Monospace)
        }
        row("fps", "%.0f".format(stats.fps))
        row(stringResource(R.string.size), if (stats.width > 0) "${stats.width}×${stats.height}" else "–")
        row("kbps", "${stats.kbps}")
        row(stringResource(R.string.loss), "%.1f %%".format(stats.lossPct))
        row("rtt", stats.rttMs?.let { "$it ms" } ?: "–")
        row(stringResource(R.string.path), stats.path?.let { stringResource(if (it == "relay") R.string.relay else R.string.direct) } ?: "–")
    }
}

/** The unattended password the linked computer asks for. */
@Composable
private fun PasswordCard(prompt: PasswordPrompt, onSend: (String) -> Unit, onCancel: () -> Unit) {
    var password by remember { mutableStateOf("") }
    val focus = remember { FocusRequester() }
    val go = { if (password.isNotEmpty() && !prompt.checking) onSend(password) }
    LaunchedEffect(prompt.wrong, prompt.left) { if (prompt.wrong) password = "" }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    Box(Modifier.fillMaxSize().imePadding().padding(16.dp), contentAlignment = Alignment.Center) {
        Column(
            Modifier.widthIn(max = 380.dp).background(glass, RoundedCornerShape(20.dp)).padding(28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Icon(Icons.Outlined.Security, null, tint = Theme.accent, modifier = Modifier.size(40.dp))
            Text(stringResource(R.string.password_asked), color = Color.White, textAlign = TextAlign.Center)
            TextField(
                value = password,
                onValueChange = { password = it },
                placeholder = { Text(stringResource(R.string.password)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                keyboardActions = KeyboardActions(onGo = { go() }),
                colors = TextFieldDefaults.colors(
                    focusedContainerColor = Color.White.copy(alpha = 0.12f),
                    unfocusedContainerColor = Color.White.copy(alpha = 0.12f),
                    focusedIndicatorColor = Color.Transparent,
                    unfocusedIndicatorColor = Color.Transparent,
                    cursorColor = Theme.accent,
                ),
                shape = RoundedCornerShape(10.dp),
                modifier = Modifier.fillMaxWidth().focusRequester(focus),
            )
            if (prompt.wrong) Text(stringResource(R.string.wrong_password, prompt.left), color = Color(0xFFFF9F0A), fontSize = 13.sp)
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedButton(onClick = onCancel) { Text(stringResource(R.string.cancel), color = Color.White) }
                Button(
                    onClick = { go() },
                    enabled = password.isNotEmpty() && !prompt.checking,
                    colors = ButtonDefaults.buttonColors(containerColor = Theme.accent, contentColor = Theme.onAccent),
                ) {
                    if (prompt.checking) CircularProgressIndicator(Modifier.size(18.dp), color = Theme.onAccent, strokeWidth = 2.dp) else Text(stringResource(R.string.unlock))
                }
            }
        }
    }
}

/** The keys the on-screen keyboard lacks, over it. Ctrl, Alt and Cmd latch for the next key. */
@Composable
private fun KeyBar(mac: Boolean, latched: Int, onKey: (String) -> Unit, onLatch: (Int) -> Unit) {
    Row(
        Modifier.fillMaxWidth().background(Color(0xFF2C2C2E)).horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp, vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        @Composable
        fun key(label: String, on: Boolean = false, onClick: () -> Unit) {
            Text(
                label,
                color = if (on) Theme.onAccent else Color.White,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.height(36.dp).background(if (on) Theme.accent else Color.White.copy(alpha = 0.14f), RoundedCornerShape(8.dp))
                    .clickable(onClick = onClick).padding(horizontal = 12.dp, vertical = 8.dp),
            )
        }
        key("esc") { onKey("Escape") }
        key("tab") { onKey("Tab") }
        key("ctrl", latched and InputPacket.MOD_CTRL != 0) { onLatch(InputPacket.MOD_CTRL) }
        key(if (mac) "⌥" else "alt", latched and InputPacket.MOD_ALT != 0) { onLatch(InputPacket.MOD_ALT) }
        key(if (mac) "⌘" else "win", latched and InputPacket.MOD_META != 0) { onLatch(InputPacket.MOD_META) }
        key("⇧", latched and InputPacket.MOD_SHIFT != 0) { onLatch(InputPacket.MOD_SHIFT) }
        key("←") { onKey("ArrowLeft") }
        key("↑") { onKey("ArrowUp") }
        key("↓") { onKey("ArrowDown") }
        key("→") { onKey("ArrowRight") }
        key(if (mac) "⌦" else "del") { onKey("Delete") }
        key("home") { onKey("Home") }
        key("end") { onKey("End") }
        key("pg↑") { onKey("PageUp") }
        key("pg↓") { onKey("PageDown") }
        for (i in 1..12) key("F$i") { onKey("F$i") }
    }
}
