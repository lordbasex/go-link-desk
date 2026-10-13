// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import org.golink.desk.AppModel
import org.golink.desk.BuildConfig
import org.golink.desk.NetworkProbe
import org.golink.desk.R
import org.golink.desk.core.Protocol
import org.golink.desk.core.SignalClient
import org.golink.desk.core.SignalUrlCheck
import org.golink.desk.core.SignalUrls
import org.golink.desk.net.OkHttpSockets

/** Settings: your name, the fingers, the message sound, the signaling server, and the app's version. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(app: AppModel, onDismiss: () -> Unit) {
    var server by remember { mutableStateOf(app.customSignal) }
    var serverError by remember { mutableStateOf("") }
    var testing by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val notAServer = stringResource(R.string.server_not_address)
    val noAnswer = stringResource(R.string.server_no_answer)

    fun save() {
        val text = server.trim()
        if (text.isEmpty()) {
            app.updateSignal("")
            serverError = ""
            return
        }
        val c = SignalUrls.check(text, allowDevHosts = BuildConfig.DEBUG)
        if (c !is SignalUrlCheck.Ok) {
            serverError = notAServer
            return
        }
        testing = true
        scope.launch {
            val problem = SignalClient.test(c.url, OkHttpSockets())
            testing = false
            if (problem == null) {
                app.updateSignal(c.url)
                server = c.url
                serverError = ""
            } else {
                serverError = noAnswer
            }
        }
    }

    Sheet(stringResource(R.string.settings), onDismiss, trailing = { SheetAction(stringResource(R.string.done), bold = true, onClick = onDismiss) }) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp)) {
            Section(stringResource(R.string.your_name), stringResource(R.string.your_name_footer)) {
                Field(app.name, { app.updateName(it) }, app.deviceName, capitalize = true)
            }
            Section(stringResource(R.string.fingers)) {
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth().padding(12.dp)) {
                    listOf(false to R.string.trackpad, true to R.string.touch).forEachIndexed { i, (touch, label) ->
                        SegmentedButton(
                            selected = app.touchMode == touch,
                            onClick = { app.updateTouchMode(touch) },
                            shape = SegmentedButtonDefaults.itemShape(i, 2),
                            colors = SegmentedButtonDefaults.colors(activeContainerColor = Theme.accent, activeContentColor = Theme.onAccent),
                        ) { Text(stringResource(label)) }
                    }
                }
                Text(
                    stringResource(if (app.touchMode) R.string.touch_explained else R.string.trackpad_explained),
                    fontSize = 13.sp,
                    color = colors.secondary,
                    modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 12.dp),
                )
            }
            Section(footer = stringResource(R.string.message_sound_footer)) {
                ToggleRow(stringResource(R.string.message_sound), app.chatSound) { app.updateChatSound(it) }
            }
            Section(stringResource(R.string.signaling_server), stringResource(R.string.signaling_footer)) {
                Field(server, { server = it }, Protocol.OFFICIAL_SIGNAL_URL, uri = true)
                if (serverError.isNotEmpty()) {
                    Text(serverError, color = Theme.danger, fontSize = 13.sp, modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 8.dp))
                }
                RowDivider()
                ActionRow(stringResource(if (testing) R.string.checking else R.string.save), enabled = !testing) { save() }
                if (app.customSignal.isNotEmpty()) {
                    RowDivider()
                    ActionRow(stringResource(R.string.use_official_server), color = Theme.danger) {
                        app.updateSignal("")
                        server = ""
                        serverError = ""
                    }
                }
            }
            Section(stringResource(R.string.connection), stringResource(R.string.connection_footer)) {
                ActionRow(stringResource(R.string.test_connection)) {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(NetworkProbe.url(context))))
                }
            }
            Section(footer = stringResource(R.string.about_footer)) {
                Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(stringResource(R.string.version), color = colors.text, modifier = Modifier.weight(1f))
                    Text(app.version, color = colors.secondary)
                }
                RowDivider()
                ActionRow("desk.go-link.org") {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://desk.go-link.org")))
                }
            }
        }
    }
}

/** A text field filling a card's row. */
@Composable
fun Field(value: String, onChange: (String) -> Unit, placeholder: String, capitalize: Boolean = false, uri: Boolean = false) {
    BasicTextField(
        value = value,
        onValueChange = onChange,
        singleLine = true,
        textStyle = TextStyle(fontSize = 17.sp, color = colors.text),
        cursorBrush = SolidColor(Theme.accent),
        keyboardOptions = KeyboardOptions(
            capitalization = if (capitalize) KeyboardCapitalization.Words else KeyboardCapitalization.None,
            keyboardType = if (uri) KeyboardType.Uri else KeyboardType.Text,
            autoCorrectEnabled = !uri,
        ),
        modifier = Modifier.fillMaxWidth().padding(16.dp),
        decorationBox = { inner ->
            if (value.isEmpty()) Text(placeholder, fontSize = 17.sp, color = colors.tertiary, maxLines = 1)
            inner()
        },
    )
}

@Composable
fun ToggleRow(title: String, on: Boolean, onChange: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth().clickable { onChange(!on) }.padding(horizontal = 16.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(title, fontSize = 17.sp, color = colors.text, modifier = Modifier.weight(1f))
        Switch(on, onChange, colors = SwitchDefaults.colors(checkedTrackColor = Color(0xFF34C759), checkedThumbColor = Color.White, checkedBorderColor = Color.Transparent))
    }
}

@Composable
fun ActionRow(title: String, color: Color = Theme.accent, enabled: Boolean = true, onClick: () -> Unit) {
    Text(
        title,
        fontSize = 17.sp,
        color = if (enabled) color else colors.tertiary,
        modifier = Modifier.fillMaxWidth().clickable(enabled = enabled, onClick = onClick).padding(16.dp),
    )
}
