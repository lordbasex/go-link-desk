// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.desk.ChatLine
import org.golink.desk.R
import org.golink.desk.SessionModel
import kotlin.math.PI
import kotlin.math.max
import kotlin.math.sin

/** The session's chat with the person at the computer. */
@Composable
fun ChatSheet(model: SessionModel, onDismiss: () -> Unit) {
    var text by remember { mutableStateOf("") }
    val list = rememberLazyListState()
    val focus = remember { FocusRequester() }
    val host = model.hostName
    val send = { if (model.say(text)) text = "" }
    val ime = androidx.compose.ui.platform.LocalSoftwareKeyboardController.current
    val close = {
        ime?.hide()
        model.quiet()
        onDismiss()
    }

    LaunchedEffect(model.chat.size, model.hostTyping) {
        val n = model.chat.size + if (model.hostTyping) 1 else 0
        if (n > 0) list.animateScrollToItem(n - 1)
    }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }

    Sheet(stringResource(R.string.chat), close, trailing = { SheetAction(stringResource(R.string.done), bold = true, onClick = close) }) {
        Column(Modifier.imePadding()) {
            LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (model.chat.isEmpty()) {
                    item {
                        Text(
                            stringResource(R.string.chat_empty),
                            fontSize = 15.sp,
                            color = colors.secondary,
                            textAlign = TextAlign.Center,
                            modifier = Modifier.fillMaxWidth().padding(top = 40.dp, start = 24.dp, end = 24.dp),
                        )
                    }
                }
                items(model.chat, key = { it.id }) { Bubble(it, host) }
                if (model.hostTyping) {
                    item(key = "typing") {
                        AnimatedVisibility(true, enter = fadeIn(), exit = fadeOut()) {
                            TypingBubble(host.ifEmpty { stringResource(R.string.the_computer_cap) })
                        }
                    }
                }
            }
            Row(
                Modifier.fillMaxWidth().background(colors.card).navigationBarsPadding().padding(10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                BasicTextField(
                    value = text,
                    onValueChange = {
                        text = it
                        model.wrote(it)
                    },
                    maxLines = 5,
                    textStyle = TextStyle(fontSize = 17.sp, color = colors.text),
                    cursorBrush = SolidColor(Theme.accent),
                    modifier = Modifier.weight(1f).focusRequester(focus),
                    decorationBox = { inner ->
                        Box(Modifier.background(colors.field, RoundedCornerShape(18.dp)).padding(horizontal = 12.dp, vertical = 8.dp)) {
                            if (text.isEmpty()) Text(stringResource(R.string.message), fontSize = 17.sp, color = colors.tertiary)
                            inner()
                        }
                    },
                )
                val can = text.isNotBlank()
                IconButton(onClick = { send() }, enabled = can) {
                    Box(Modifier.size(30.dp).background(if (can) Theme.accent else colors.tertiary, CircleShape), contentAlignment = Alignment.Center) {
                        Icon(Icons.Filled.ArrowUpward, stringResource(R.string.send), tint = Color.Black, modifier = Modifier.size(20.dp))
                    }
                }
            }
        }
    }
}

@Composable
private fun Bubble(line: ChatLine, host: String) {
    val ctx = LocalContext.current
    val time = android.text.format.DateFormat.getTimeFormat(ctx).format(java.util.Date(line.at))
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (line.fromHost) Arrangement.Start else Arrangement.End) {
        Column(Modifier.widthIn(max = 520.dp).padding(start = if (line.fromHost) 0.dp else 40.dp, end = if (line.fromHost) 40.dp else 0.dp), horizontalAlignment = if (line.fromHost) Alignment.Start else Alignment.End) {
            SelectionContainer {
                Text(
                    line.text,
                    fontSize = 17.sp,
                    color = if (line.fromHost) colors.text else Color.Black,
                    modifier = Modifier.background(if (line.fromHost) colors.card else Theme.accent, RoundedCornerShape(16.dp)).padding(horizontal = 12.dp, vertical = 8.dp),
                )
            }
            Text((if (line.fromHost && host.isNotEmpty()) "$host · " else "") + time, fontSize = 11.sp, color = colors.secondary, modifier = Modifier.padding(top = 2.dp))
        }
    }
}

/** "Name is typing" with three dots that rise in turn, like a messaging app. */
@Composable
private fun TypingBubble(name: String) {
    val t by rememberInfiniteTransition(label = "typing").animateFloat(0f, 1f, infiniteRepeatable(tween(1200, easing = LinearEasing), RepeatMode.Restart), label = "dots")
    Column {
        Row(Modifier.background(colors.card, RoundedCornerShape(16.dp)).padding(horizontal = 14.dp, vertical = 13.dp), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            for (i in 0 until 3) {
                val wave = max(0.0, sin(t * 2 * PI - i * 0.6)).toFloat()
                Box(Modifier.offset(y = (-3 * wave).dp).size(7.dp).background(colors.secondary.copy(alpha = 0.4f + 0.6f * wave), CircleShape))
            }
        }
        Text(stringResource(R.string.is_typing, name), fontSize = 11.sp, color = colors.secondary, modifier = Modifier.padding(top = 2.dp))
    }
}
