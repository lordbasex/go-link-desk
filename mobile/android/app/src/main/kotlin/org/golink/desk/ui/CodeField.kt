// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.layout.Box
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.desk.core.CodeInput

/** The 9-digit code, grouped as it is typed or pasted ("915 355 636"), large and centered. */
@Composable
fun CodeField(
    value: TextFieldValue,
    onChange: (TextFieldValue) -> Unit,
    label: String,
    size: Int = 34,
    focus: FocusRequester? = null,
    onDone: () -> Unit = {},
) {
    val style = TextStyle(fontSize = size.sp, fontWeight = FontWeight.SemiBold, fontFamily = FontFamily.Monospace, textAlign = TextAlign.Center, color = colors.text)
    BasicTextField(
        value = value,
        onValueChange = { next ->
            val e = CodeInput.fromChange(value.text, next.text, next.selection.end, allowLinks = false)
            onChange(TextFieldValue(e.text, TextRange(e.caret)))
        },
        singleLine = true,
        textStyle = style,
        cursorBrush = SolidColor(Theme.accent),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Go),
        keyboardActions = KeyboardActions(onGo = { onDone() }),
        modifier = Modifier.fillMaxWidth().semantics { contentDescription = label }.let { if (focus != null) it.focusRequester(focus) else it },
        decorationBox = { inner ->
            Box(
                Modifier.fillMaxWidth().background(colors.field, RoundedCornerShape(14.dp)).padding(vertical = 12.dp),
                contentAlignment = Alignment.Center,
            ) {
                if (value.text.isEmpty()) Text("000 000 000", style = style.copy(color = colors.tertiary))
                inner()
            }
        },
    )
}
