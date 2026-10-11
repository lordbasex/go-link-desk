// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull

// Defensive readers for JSON that comes from the network: the device and
// the signaling server are trusted to speak the protocol, but names and
// chat are typed by the person at the computer, and a custom signaling server could send
// anything. Every reader returns a safe default instead of throwing, like
// the web's str/num/obj/arr helpers (frontend/packages/shared).

internal val json = Json { ignoreUnknownKeys = true }

/** Parses text as a JSON object, or null for anything else. */
fun parseObject(text: String): JsonObject? =
    try {
        json.parseToJsonElement(text) as? JsonObject
    } catch (_: Exception) {
        null
    }

internal fun JsonElement?.obj(): JsonObject = this as? JsonObject ?: JsonObject(emptyMap())

internal fun JsonElement?.arr(): List<JsonElement> = (this as? JsonArray)?.toList() ?: emptyList()

/** A string field, cut to max characters; "" when it is not a string. */
internal fun JsonElement?.str(max: Int = 300): String {
    val p = this as? JsonPrimitive ?: return ""
    if (!p.isString) return ""
    return p.content.take(max)
}

/** A string field or null when absent or not a string. */
internal fun JsonElement?.strOrNull(): String? {
    val p = this as? JsonPrimitive ?: return null
    return if (p.isString) p.content else null
}

/** A finite number field; 0 otherwise. */
internal fun JsonElement?.num(): Double {
    val p = this as? JsonPrimitive ?: return 0.0
    if (p.isString || this is JsonNull) return 0.0
    val d = p.doubleOrNull ?: return 0.0
    return if (d.isFinite()) d else 0.0
}

internal fun JsonElement?.isNumber(): Boolean {
    val p = this as? JsonPrimitive ?: return false
    return !p.isString && this !is JsonNull && p.doubleOrNull?.isFinite() == true
}

internal fun JsonElement?.isTrue(): Boolean {
    val p = this as? JsonPrimitive ?: return false
    return !p.isString && p.booleanOrNull == true
}

internal fun JsonElement?.isFalse(): Boolean {
    val p = this as? JsonPrimitive ?: return false
    return !p.isString && p.booleanOrNull == false
}

/** JavaScript's Math.round (halves go up), so numbers match the web. */
internal fun jsRound(v: Double): Long = kotlin.math.floor(v + 0.5).toLong()
