// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties

/**
 * The app's colors: go-link Desk's amber, on the grounds the iOS app uses
 * (the system's grouped background, its cards and fields), light or dark
 * as the system is. A session is always dark.
 */
data class Palette(
    val background: Color,
    val card: Color,
    val field: Color,
    val text: Color,
    val secondary: Color,
    val tertiary: Color,
    val separator: Color,
)

object Theme {
    val accent = Color(0xFFF2A33A) // --color-accent
    val onAccent = Color(0xFF000000)
    val danger = Color(0xFFFF453A)

    val light = Palette(
        background = Color(0xFFF2F2F7),
        card = Color(0xFFFFFFFF),
        field = Color(0x1F767680),
        text = Color(0xFF000000),
        secondary = Color(0x993C3C43),
        tertiary = Color(0x4D3C3C43),
        separator = Color(0x4A3C3C43),
    )
    val dark = Palette(
        background = Color(0xFF000000),
        card = Color(0xFF1C1C1E),
        field = Color(0x3D767680),
        text = Color(0xFFFFFFFF),
        secondary = Color(0x99EBEBF5),
        tertiary = Color(0x4DEBEBF5),
        separator = Color(0x99545458),
    )
}

val LocalPalette = staticCompositionLocalOf { Theme.light }

/** The palette now. */
val colors: Palette @Composable get() = LocalPalette.current

@Composable
fun DeskTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val p = if (dark) Theme.dark else Theme.light
    val scheme = if (dark) {
        darkColorScheme(
            primary = Theme.accent, onPrimary = Theme.onAccent, background = p.background, onBackground = p.text,
            surface = p.card, onSurface = p.text, surfaceVariant = p.field, onSurfaceVariant = p.secondary,
            surfaceContainer = p.card, surfaceContainerHigh = p.card, surfaceContainerHighest = Color(0xFF2C2C2E),
            surfaceContainerLow = p.card, outline = p.separator, outlineVariant = p.separator, error = Theme.danger,
        )
    } else {
        lightColorScheme(
            primary = Theme.accent, onPrimary = Theme.onAccent, background = p.background, onBackground = p.text,
            surface = p.card, onSurface = p.text, surfaceVariant = p.field, onSurfaceVariant = p.secondary,
            surfaceContainer = p.card, surfaceContainerHigh = p.card, surfaceContainerHighest = Color(0xFFE5E5EA),
            surfaceContainerLow = p.card, outline = p.separator, outlineVariant = p.separator, error = Theme.danger,
        )
    }
    CompositionLocalProvider(LocalPalette provides p) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}

/** A card of rows or text, as iOS's inset grouped lists. */
@Composable
fun Card(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(modifier.fillMaxWidth().background(colors.card, RoundedCornerShape(16.dp)), content = content)
}

/** A section of a sheet: a small title, a card, and a note under it. */
@Composable
fun Section(header: String? = null, footer: String? = null, content: @Composable ColumnScope.() -> Unit) {
    Column(Modifier.fillMaxWidth().padding(bottom = 22.dp)) {
        if (header != null) {
            Text(
                header.uppercase(),
                fontSize = 13.sp,
                color = colors.secondary,
                modifier = Modifier.padding(start = 16.dp, bottom = 6.dp),
            )
        }
        Card { content() }
        if (footer != null) {
            Text(footer, fontSize = 13.sp, color = colors.secondary, modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 6.dp))
        }
    }
}

/** A thin line between the rows of a card. */
@Composable
fun RowDivider(start: Int = 16) {
    HorizontalDivider(Modifier.padding(start = start.dp), thickness = 0.5.dp, color = colors.separator)
}

/**
 * A sheet over the screen, as the iOS app's: a title with Cancel and Done
 * (or other) actions, and its content; at most 640 dp wide on a tablet.
 */
@Composable
fun Sheet(
    title: String,
    onDismiss: () -> Unit,
    leading: (@Composable () -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Box(Modifier.fillMaxSize().statusBarsPadding().padding(top = 8.dp), contentAlignment = Alignment.TopCenter) {
            Column(
                Modifier.widthIn(max = 640.dp).fillMaxSize()
                    .background(colors.background, RoundedCornerShape(topStart = 14.dp, topEnd = 14.dp)),
            ) {
                Row(Modifier.fillMaxWidth().height(52.dp).padding(horizontal = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.weight(1f), contentAlignment = Alignment.CenterStart) { leading?.invoke() }
                    Text(title, fontWeight = FontWeight.SemiBold, fontSize = 17.sp, textAlign = TextAlign.Center, maxLines = 1, color = colors.text)
                    Box(Modifier.weight(1f), contentAlignment = Alignment.CenterEnd) { trailing?.invoke() }
                }
                content()
            }
        }
    }
}

/** A sheet's plain text action (Cancel, Done…), in the accent; bold for the main one. */
@Composable
fun SheetAction(text: String, bold: Boolean = false, enabled: Boolean = true, onClick: () -> Unit) {
    TextButton(onClick = onClick, enabled = enabled) {
        Text(text, color = if (enabled) Theme.accent else colors.tertiary, fontWeight = if (bold) FontWeight.SemiBold else FontWeight.Normal, fontSize = 17.sp)
    }
}

@Composable
fun Gap(h: Int) = Spacer(Modifier.height(h.dp))

val spacedTen = Arrangement.spacedBy(10.dp)
