// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

/** A STUN or TURN server from signalhub's hello (same shape as RTCIceServer). */
data class IceServer(
    val urls: List<String>,
    val username: String? = null,
    val credential: String? = null,
)

object IceServers {
    private val SCHEME = Regex("^(stun|turns?):", RegexOption.IGNORE_CASE)

    /**
     * The STUN/TURN servers of a hello, checked: at most 10, each with
     * stun:/turn:/turns: URLs and string credentials. Anything else is
     * dropped. They live in memory only and are taken again on every
     * reconnect (TURN credentials expire).
     */
    fun parse(v: JsonElement?): List<IceServer> = v.arr().take(10).mapNotNull { s ->
        val o = s as? kotlinx.serialization.json.JsonObject ?: return@mapNotNull null
        val raw = o["urls"]
        val list = when {
            raw is JsonPrimitive && raw.isString -> listOf(raw.content)
            else -> raw.arr().mapNotNull { it.strOrNull() }
        }
        val urls = list.filter { SCHEME.containsMatchIn(it) && it.length <= 256 }.take(10)
        if (urls.isEmpty()) return@mapNotNull null
        IceServer(
            urls = urls,
            username = o["username"].strOrNull()?.take(256),
            credential = o["credential"].strOrNull()?.take(256),
        )
    }
}
