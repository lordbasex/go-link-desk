// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Chat
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Gesture
import androidx.compose.material.icons.outlined.Keyboard
import androidx.compose.material.icons.outlined.KeyboardArrowUp
import androidx.compose.material.icons.outlined.KeyboardCommandKey
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Monitor
import androidx.compose.material.icons.outlined.Mouse
import androidx.compose.material.icons.outlined.PanTool
import androidx.compose.material.icons.outlined.Pinch
import androidx.compose.material.icons.outlined.SwipeVertical
import androidx.compose.material.icons.outlined.TouchApp
import androidx.compose.material.icons.outlined.Swipe
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.desk.R

/**
 * How to use the computer from here: the fingers (trackpad and touch),
 * the keyboard and its keys, the toolbar, a mouse, and the lock. It opens
 * by itself the first time a session starts, and from the toolbar's ?
 * button.
 */
@Composable
fun GuideSheet(touchMode: Boolean, onDismiss: () -> Unit) {
    Sheet(stringResource(R.string.how_to_use), onDismiss, trailing = { SheetAction(stringResource(R.string.done), bold = true, onClick = onDismiss) }) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(16.dp)) {
            Section {
                Text(
                    stringResource(if (touchMode) R.string.guide_in_touch else R.string.guide_in_trackpad),
                    fontSize = 15.sp,
                    color = colors.secondary,
                    modifier = Modifier.padding(16.dp),
                )
            }
            Section(stringResource(R.string.trackpad_mode)) {
                GuideRow(Icons.Outlined.Gesture, R.string.g_slide, R.string.g_slide_text)
                GuideRow(Icons.Outlined.TouchApp, R.string.g_tap, R.string.g_tap_text)
                GuideRow(Icons.Outlined.PanTool, R.string.g_hold, R.string.g_hold_text)
                GuideRow(Icons.Outlined.TouchApp, R.string.g_two_tap, R.string.g_two_tap_text, 2)
                GuideRow(Icons.Outlined.Pinch, R.string.g_pinch, R.string.g_pinch_text, 2)
                GuideRow(Icons.Outlined.SwipeVertical, R.string.g_three, R.string.g_three_text, 3, last = true)
            }
            Section(stringResource(R.string.touch_mode)) {
                GuideRow(Icons.Outlined.TouchApp, R.string.g_touch, R.string.g_touch_text)
                GuideRow(Icons.Outlined.PanTool, R.string.g_touch_hold, R.string.g_touch_hold_text)
                GuideRow(Icons.Outlined.TouchApp, R.string.g_two_tap, R.string.g_touch_right, 2)
                GuideRow(Icons.Outlined.Swipe, R.string.g_two_slide, R.string.g_two_slide_text, 2, last = true)
            }
            Section(stringResource(R.string.keyboard)) {
                GuideRow(Icons.Outlined.Keyboard, R.string.g_kb_button, R.string.g_kb_button_text)
                GuideRow(Icons.Outlined.KeyboardCommandKey, R.string.g_kb_bar, R.string.g_kb_bar_text)
                GuideRow(Icons.Outlined.Keyboard, R.string.g_kb_hw, R.string.g_kb_hw_text, last = true)
            }
            Section(stringResource(R.string.the_toolbar)) {
                GuideRow(Icons.Outlined.KeyboardCommandKey, R.string.special_keys, R.string.g_keys_text)
                GuideRow(Icons.AutoMirrored.Outlined.Chat, R.string.chat, R.string.g_chat_text)
                GuideRow(Icons.Outlined.Folder, R.string.files, R.string.g_files_text)
                GuideRow(Icons.Outlined.Monitor, R.string.screen, R.string.g_screen_text)
                GuideRow(Icons.Outlined.Lock, R.string.lock, R.string.g_lock_text)
                GuideRow(Icons.Outlined.KeyboardArrowUp, R.string.g_hide, R.string.g_hide_text, last = true)
            }
            Section(stringResource(R.string.g_mouse_title)) {
                GuideRow(Icons.Outlined.Mouse, R.string.g_mouse, R.string.g_mouse_text, last = true)
            }
        }
    }
}

/** One gesture: its symbol (and how many fingers), what to do and what it does. */
@Composable
private fun GuideRow(icon: ImageVector, title: Int, text: Int, fingers: Int = 0, last: Boolean = false) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.Top) {
        Box(Modifier.size(40.dp), contentAlignment = Alignment.Center) {
            Icon(icon, null, tint = Theme.accent, modifier = Modifier.size(28.dp))
            if (fingers > 0) {
                Text(
                    "$fingers",
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Bold,
                    color = Color.Black,
                    modifier = Modifier.align(Alignment.BottomEnd).size(16.dp).background(Theme.accent, CircleShape).padding(top = 0.dp),
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                )
            }
        }
        Column(Modifier.padding(start = 14.dp)) {
            Text(stringResource(title), fontSize = 17.sp, fontWeight = FontWeight.Medium, color = colors.text)
            Text(stringResource(text), fontSize = 15.sp, color = colors.secondary)
        }
    }
    if (!last) RowDivider(70)
}
