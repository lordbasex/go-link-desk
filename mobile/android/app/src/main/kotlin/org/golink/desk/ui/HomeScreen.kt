// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import android.app.Activity
import android.text.format.DateUtils
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.DesktopMac
import androidx.compose.material.icons.outlined.DesktopWindows
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.Monitor
import androidx.compose.material.icons.outlined.PlayCircle
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.desk.AppModel
import org.golink.desk.LinkedComputer
import org.golink.desk.R
import org.golink.desk.core.CodeInput
import org.golink.desk.core.Recent
import org.golink.desk.core.Recents
import androidx.compose.runtime.LaunchedEffect

/** Home: the other computer's code, My computers and the recent computers. */
@Composable
fun HomeScreen(app: AppModel) {
    var code by remember { mutableStateOf(TextFieldValue("")) }
    var settings by remember { mutableStateOf(false) }
    var linking by remember { mutableStateOf(false) }
    val focus = LocalFocusManager.current
    val ready = Recents.digits(code.text) != null
    val connect = {
        if (ready) {
            focus.clearFocus()
            app.connect(code.text)
        }
    }

    Box(Modifier.fillMaxSize().background(colors.background).systemBarsPadding().imePadding()) {
        Column(
            Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Column(Modifier.widthIn(max = 560.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(28.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Image(painterResource(R.drawable.logo), null, Modifier.size(44.dp).clip(RoundedCornerShape(10.dp)))
                    Column(Modifier.padding(start = 12.dp).weight(1f)) {
                        Text("go-link Desk", fontSize = 22.sp, fontWeight = FontWeight.SemiBold, color = colors.text)
                        Text(stringResource(R.string.remote_desktop), fontSize = 15.sp, color = colors.secondary)
                    }
                    IconButton(onClick = { settings = true }) {
                        Icon(Icons.Outlined.Settings, stringResource(R.string.settings), tint = Theme.accent)
                    }
                }
                Column(
                    Modifier.fillMaxWidth().background(colors.card, RoundedCornerShape(20.dp)).padding(18.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp),
                ) {
                    Text(stringResource(R.string.connect_to_another), fontWeight = FontWeight.SemiBold, fontSize = 17.sp, color = colors.text)
                    CodeField(code, { code = it }, stringResource(R.string.its_code), onDone = connect)
                    Button(
                        onClick = connect,
                        enabled = ready,
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(12.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Theme.accent, contentColor = Theme.onAccent),
                    ) {
                        Text(stringResource(R.string.connect), fontWeight = FontWeight.SemiBold, fontSize = 17.sp, modifier = Modifier.padding(vertical = 6.dp))
                    }
                    // No other computer at hand: go-link Desk's demo, always on.
                    OutlinedButton(
                        onClick = { app.tryDemo() },
                        enabled = !app.demoLoading && app.session == null,
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(12.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = Theme.accent),
                    ) {
                        if (app.demoLoading) {
                            CircularProgressIndicator(Modifier.size(18.dp), color = Theme.accent, strokeWidth = 2.dp)
                        } else {
                            Icon(Icons.Outlined.PlayCircle, null, Modifier.size(20.dp))
                        }
                        Text(stringResource(R.string.try_demo), fontWeight = FontWeight.SemiBold, fontSize = 15.sp, modifier = Modifier.padding(start = 8.dp, top = 2.dp, bottom = 2.dp))
                    }
                }
                Computers(app) { linking = true }
                if (app.recents.isNotEmpty()) RecentList(app)
                Text(stringResource(R.string.home_note), fontSize = 13.sp, color = colors.secondary)
            }
        }
    }
    if (settings) SettingsSheet(app) { settings = false }
    if (linking) LinkSheet(app) { linking = false }
    app.linkError?.let { why ->
        AlertDialog(
            onDismissRequest = { app.linkError = null },
            confirmButton = { TextButton(onClick = { app.linkError = null }) { Text(stringResource(R.string.ok), color = Theme.accent) } },
            title = { Text(stringResource(R.string.cannot_open_it)) },
            text = { Text(why) },
        )
    }
}

/** My computers: the linked ones, opened with the fingerprint, face or screen lock, without anyone accepting. */
@Composable
private fun Computers(app: AppModel, onLink: () -> Unit) {
    val activity = LocalContext.current as Activity
    @Suppress("UNUSED_VARIABLE") val version = app.linksVersion // redraws when the links change
    val list = app.links.computers
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.my_computers), fontWeight = FontWeight.SemiBold, fontSize = 17.sp, color = colors.text, modifier = Modifier.weight(1f))
            TextButton(onClick = onLink) {
                Icon(Icons.Outlined.Add, null, tint = Theme.accent, modifier = Modifier.size(18.dp))
                Text(stringResource(R.string.link), color = Theme.accent, fontSize = 15.sp, modifier = Modifier.padding(start = 4.dp))
            }
        }
        if (list.isEmpty()) {
            Text(
                stringResource(R.string.computers_empty, app.unlockName),
                fontSize = 13.sp,
                color = colors.secondary,
                modifier = Modifier.fillMaxWidth().background(colors.card, RoundedCornerShape(16.dp)).padding(14.dp),
            )
        } else {
            Card(Modifier.clip(RoundedCornerShape(16.dp))) {
                list.forEachIndexed { i, c ->
                    ComputerRow(c, app.links.hasBiometrics, onOpen = { app.reach(activity, c) }, onForget = { app.forgetLink(c.deviceId) })
                    if (i < list.size - 1) RowDivider(56)
                }
            }
        }
    }
}

@Composable
private fun RecentList(app: AppModel) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.recent_sessions), fontWeight = FontWeight.SemiBold, fontSize = 17.sp, color = colors.text, modifier = Modifier.weight(1f))
            TextButton(onClick = { app.clearRecents() }) { Text(stringResource(R.string.clear), color = Theme.accent, fontSize = 15.sp) }
        }
        Card(Modifier.clip(RoundedCornerShape(16.dp))) {
            app.recents.forEachIndexed { i, r ->
                RecentRow(r, onOpen = { app.connect(r.code) }, onForget = { app.forget(r.code) })
                if (i < app.recents.size - 1) RowDivider(56)
            }
        }
    }
}

fun osIcon(os: String): ImageVector = when (os) {
    "darwin" -> Icons.Outlined.DesktopMac
    "windows" -> Icons.Outlined.DesktopWindows
    else -> Icons.Outlined.Monitor
}

fun dateText(ms: Long, context: android.content.Context): String =
    DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_DATE or DateUtils.FORMAT_ABBREV_MONTH or DateUtils.FORMAT_SHOW_TIME)

/** A row of a card list, with its long-press menu (as iOS's context menu). */
@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ListRow(icon: ImageVector, title: String, detail: String, trailing: ImageVector, forget: String, onOpen: () -> Unit, onForget: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier.fillMaxWidth().combinedClickable(onClick = onOpen, onLongClick = { menu = true }).padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, null, tint = Theme.accent, modifier = Modifier.size(32.dp).padding(3.dp))
            Column(Modifier.padding(start = 12.dp).weight(1f)) {
                Text(title, fontWeight = FontWeight.Medium, fontSize = 17.sp, color = colors.text, maxLines = 1)
                Text(detail, fontSize = 12.sp, color = colors.secondary, maxLines = 1)
            }
            Icon(trailing, null, tint = colors.tertiary, modifier = Modifier.size(20.dp))
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(
                text = { Text(forget, color = Theme.danger) },
                leadingIcon = { Icon(Icons.Outlined.Delete, null, tint = Theme.danger) },
                onClick = {
                    menu = false
                    onForget()
                },
            )
        }
    }
}

@Composable
private fun RecentRow(r: Recent, onOpen: () -> Unit, onForget: () -> Unit) {
    val ctx = LocalContext.current
    ListRow(
        osIcon(r.os),
        r.name.ifEmpty { CodeInput.group(r.code) },
        listOf(CodeInput.group(r.code), Recents.systemName(r.os), dateText(r.at, ctx)).filter { it.isNotEmpty() }.joinToString(" · "),
        Icons.AutoMirrored.Outlined.KeyboardArrowRight,
        stringResource(R.string.remove_from_list),
        onOpen,
        onForget,
    )
}

@Composable
private fun ComputerRow(c: LinkedComputer, biometrics: Boolean, onOpen: () -> Unit, onForget: () -> Unit) {
    val ctx = LocalContext.current
    ListRow(
        osIcon(c.os),
        c.name.ifEmpty { stringResource(R.string.linked_computer) },
        listOf(Recents.systemName(c.os), c.lastUsed?.let { dateText(it, ctx) } ?: "").filter { it.isNotEmpty() }.joinToString(" · "),
        if (biometrics) Icons.Outlined.Fingerprint else Icons.Outlined.Lock,
        stringResource(R.string.forget_computer),
        onOpen,
        onForget,
    )
}

/** Link a computer: its pairing code (Settings › Unattended access there), and the person there accepts once. */
@Composable
private fun LinkSheet(app: AppModel, onDismiss: () -> Unit) {
    var code by remember { mutableStateOf(TextFieldValue("")) }
    val focus = remember { FocusRequester() }
    val ok = Recents.digits(code.text) != null
    val go = {
        if (ok) {
            onDismiss()
            app.link(code.text)
        }
    }
    Sheet(
        stringResource(R.string.link_a_computer),
        onDismiss,
        leading = { SheetAction(stringResource(R.string.cancel), onClick = onDismiss) },
    ) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp)) {
            Section(header = stringResource(R.string.pairing_code), footer = stringResource(R.string.link_footer, app.unlockName)) {
                Box(Modifier.padding(12.dp)) { CodeField(code, { code = it }, stringResource(R.string.pairing_code), size = 28, focus = focus, onDone = go) }
            }
            Section {
                TextButton(onClick = go, enabled = ok, modifier = Modifier.fillMaxWidth()) {
                    Icon(Icons.Outlined.Link, null, tint = if (ok) Theme.accent else colors.tertiary)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.link_and_connect), color = if (ok) Theme.accent else colors.tertiary, fontSize = 17.sp)
                }
            }
        }
    }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
}
