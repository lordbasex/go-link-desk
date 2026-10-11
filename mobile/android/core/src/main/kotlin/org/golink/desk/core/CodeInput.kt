// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

/**
 * The 9-digit session code shown as "915 355 636" while it is typed. The
 * spaces are only visual: the code is the digits. The same rules as the
 * website's and the iOS app's helpers.
 */
object CodeInput {
    const val DIGITS = 9
    const val MAX_TEXT = 120

    /** The displayed text and where the caret goes. */
    data class Edit(val text: String, val caret: Int)

    /** "915355636" -> "915 355 636" (groups of 3, no trailing space). */
    fun group(digits: String): String = digits.chunked(3).joinToString(" ")

    /**
     * Replaces [start, end) of the displayed [text] with [insert]. A link
     * (any character but digits, spaces, '.' or '-', when [allowLinks]) is
     * left as typed; otherwise the digits are kept, up to 9, and grouped.
     * Deleting only a space (backspace over a separator) also deletes the
     * digit before it.
     */
    fun edit(text: String, start: Int, end: Int, insert: String, allowLinks: Boolean = true): Edit {
        var s = start.coerceIn(0, text.length)
        val e = end.coerceIn(s, text.length)
        val raw = text.substring(0, s) + insert + text.substring(e)
        if (allowLinks && raw.any { !(it.isDigit() || it.isWhitespace() || it == '.' || it == '-') }) {
            val capped = raw.take(MAX_TEXT)
            return Edit(capped, (s + insert.length).coerceAtMost(capped.length))
        }
        if (insert.isEmpty() && e > s && text.substring(s, e).all { it.isWhitespace() } && s > 0) {
            s = text.lastIndexOfAny(('0'..'9').toList().toCharArray(), s - 1).coerceAtLeast(0)
        }
        val before = digitsOf(text.substring(0, s))
        val inserted = digitsOf(insert)
        val digits = (before + inserted + digitsOf(text.substring(e))).take(DIGITS)
        val caretDigits = minOf(DIGITS, before.length + inserted.length, digits.length)
        return Edit(group(digits), caretAfter(caretDigits))
    }

    /**
     * An edit from the old text and the new text with its caret, as a text
     * field reports it: the common prefix (up to the caret) and suffix.
     */
    fun fromChange(prev: String, next: String, caret: Int, allowLinks: Boolean = true): Edit {
        val c = caret.coerceIn(0, next.length)
        var prefix = 0
        while (prefix < c && prefix < prev.length && prev[prefix] == next[prefix]) prefix++
        var suffix = 0
        while (suffix < next.length - c && suffix < prev.length - prefix &&
            prev[prev.length - 1 - suffix] == next[next.length - 1 - suffix]
        ) suffix++
        return edit(prev, prefix, prev.length - suffix, next.substring(prefix, next.length - suffix), allowLinks)
    }

    /** The offset right after [n] digits in the grouped text. */
    fun caretAfter(n: Int): Int = if (n <= 0) 0 else n + (n - 1) / 3

    private fun digitsOf(text: String): String = text.filter { it in '0'..'9' }
}
