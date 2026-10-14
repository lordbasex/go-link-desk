// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

// What travels inside signalhub's "signal" payload between the app and the
// computer (docs/protocol.md). The app knocks; the person at the computer
// accepts or not; the computer then offers and the app answers. signalhub
// never reads it.

/** An ICE candidate as the web's RTCIceCandidateInit. */
data class IceCandidateInit(val candidate: String, val sdpMid: String?, val sdpMLineIndex: Int)

/** What the computer lets this viewer do. Seeing the screen always. */
data class Permissions(
    val control: Boolean = false,
    val audio: Boolean = false,
    val clipboard: Boolean = false,
    val files: Boolean = false,
    /** Screenshots and recordings, each asked of the computer first. */
    val record: Boolean = false,
) {
    companion object {
        fun parse(v: JsonElement?): Permissions {
            val o = v.obj()
            return Permissions(o["control"].isTrue(), o["audio"].isTrue(), o["clipboard"].isTrue(), o["files"].isTrue(), o["record"].isTrue())
        }
    }
}

/** What the computer says through signalhub. */
sealed interface HostSignal {
    data class Accepted(val permissions: Permissions) : HostSignal

    /** declined, timeout, busy, unavailable or off. */
    data class Rejected(val reason: String) : HostSignal

    data class Offer(val sdp: String) : HostSignal

    data class Candidate(val candidate: IceCandidateInit) : HostSignal
}

object RtcSignals {
    private val REJECT_REASONS = setOf("declined", "timeout", "busy", "unavailable", "off")

    fun parse(payload: JsonElement?): HostSignal? {
        val o = payload as? JsonObject ?: return null
        return when (o["kind"].str(20)) {
            "accepted" -> HostSignal.Accepted(Permissions.parse(o["permissions"]))
            "rejected" -> {
                val r = o["reason"].str(20)
                HostSignal.Rejected(if (r in REJECT_REASONS) r else "declined")
            }
            "offer" -> o["sdp"].strOrNull()?.takeIf { it.isNotEmpty() }?.let { HostSignal.Offer(it) }
            "candidate" -> {
                val c = o["candidate"] as? JsonObject ?: return null
                val text = c["candidate"].strOrNull() ?: return null
                HostSignal.Candidate(
                    IceCandidateInit(
                        candidate = text.take(1000),
                        sdpMid = c["sdpMid"].strOrNull()?.take(32),
                        sdpMLineIndex = if (c["sdpMLineIndex"].isNumber()) c["sdpMLineIndex"].num().toInt() else 0,
                    ),
                )
            }
            else -> null
        }
    }

    /**
     * The knock: who asks (a name the person at the computer sees, and the
     * app and system as "ua") and the video codecs this side decodes.
     * Cut as the host cuts them (40 and 60 characters).
     */
    fun knock(name: String, ua: String, codecs: List<String>): JsonObject = buildJsonObject {
        put("kind", "knock")
        put("name", name.trim().take(40))
        put("ua", ua.take(60))
        put("codecs", JsonArray(codecs.map { JsonPrimitive(it) }))
    }

    fun answer(sdp: String): JsonObject = buildJsonObject {
        put("kind", "answer")
        put("sdp", sdp)
    }

    fun candidate(c: IceCandidateInit): JsonObject = buildJsonObject {
        put("kind", "candidate")
        put(
            "candidate",
            buildJsonObject {
                put("candidate", c.candidate)
                if (c.sdpMid != null) put("sdpMid", c.sdpMid)
                put("sdpMLineIndex", c.sdpMLineIndex)
            },
        )
    }
}
