// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.serialization.Serializable
import kotlin.math.max

/** A computer this app connected to with its code, for Home's list. at: milliseconds since 1970. */
@Serializable
data class Recent(val code: String, val name: String, val os: String, val at: Long)

object Recents {
    const val MAX = 12

    /** Puts r first, without its older entry, keeping at most MAX. */
    fun add(r: Recent, list: List<Recent>): List<Recent> = (listOf(r) + list.filter { it.code != r.code }).take(MAX)

    /** The 9 digits of what was typed, or null. */
    fun digits(typed: String): String? {
        val d = typed.filter { it in '0'..'9' }
        val rest = typed.filter { it !in '0'..'9' && !it.isWhitespace() && it != '-' && it != '.' }
        return if (d.length == CodeInput.DIGITS && rest.isEmpty()) d else null
    }

    /** The system's name, as Home shows it. */
    fun systemName(os: String): String = when (os) {
        "darwin" -> "macOS"
        "windows" -> "Windows"
        "linux" -> "Linux"
        else -> os
    }

    fun encode(list: List<Recent>): String = json.encodeToString(kotlinx.serialization.builtins.ListSerializer(Recent.serializer()), list)

    fun decode(text: String?): List<Recent> =
        text?.let { runCatching { json.decodeFromString(kotlinx.serialization.builtins.ListSerializer(Recent.serializer()), it) }.getOrNull() } ?: emptyList()
}

/** The numbers the details panel shows and the loss the computer is told about. */
data class LiveStats(
    val fps: Double = 0.0,
    val width: Int = 0,
    val height: Int = 0,
    val kbps: Int = 0,
    val lossPct: Double = 0.0,
    val rttMs: Int? = null,
    /** "direct" or "relay", once known. */
    val path: String? = null,
)

/** Turns successive cumulative counters into rates. */
class StatsMeter {
    data class Sample(
        val atMs: Double,
        val framesDecoded: Long? = null,
        val bytesReceived: Long? = null,
        val packetsReceived: Long? = null,
        val packetsLost: Long? = null,
        val width: Int? = null,
        val height: Int? = null,
        val rttSeconds: Double? = null,
        val localType: String? = null,
        val remoteType: String? = null,
    )

    private var last: Sample? = null

    fun next(s: Sample): LiveStats {
        var fps = 0.0
        var kbps = 0
        var loss = 0.0
        val p = last
        if (p != null && s.atMs > p.atMs) {
            val dt = (s.atMs - p.atMs) / 1000
            val a = s.framesDecoded
            val b = p.framesDecoded
            if (a != null && b != null && a >= b) fps = (a - b) / dt
            val ba = s.bytesReceived
            val bb = p.bytesReceived
            if (ba != null && bb != null && ba >= bb) kbps = ((ba - bb) * 8 / 1000.0 / dt).toInt()
            val rx = s.packetsReceived
            val prx = p.packetsReceived
            val lost = s.packetsLost
            val plost = p.packetsLost
            if (rx != null && prx != null && lost != null && plost != null) {
                val got = max(0L, rx - prx)
                val missed = max(0L, lost - plost)
                if (got + missed > 0) loss = missed * 100.0 / (got + missed)
            }
        }
        last = s
        val path = if (s.localType != null && s.remoteType != null) {
            if (s.localType == "relay" || s.remoteType == "relay") "relay" else "direct"
        } else {
            null
        }
        return LiveStats(fps, s.width ?: 0, s.height ?: 0, kbps, loss, s.rttSeconds?.let { Math.round(it * 1000).toInt() }, path)
    }
}
