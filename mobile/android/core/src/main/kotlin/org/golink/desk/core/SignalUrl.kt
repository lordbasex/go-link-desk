// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import java.net.URI

// Which signaling server the app uses: the official one, or one the person
// typed in Settings. It is only changed by hand (after a test that the
// server answers hello), never from a link or a QR code, so nobody can send
// people to a hostile server.
//
// Debug builds (and only they: the app passes allowDevHosts =
// BuildConfig.DEBUG) also accept ws:// to the development machine, like
// the website's loopback exception: localhost, 127.0.0.1 and 10.0.2.2
// (the Android emulator's address for the computer it runs on). The
// end-to-end test points the app at its own local signalhub this way.

sealed interface SignalUrlCheck {
    data class Ok(val url: String) : SignalUrlCheck

    data class Bad(val problem: Problem) : SignalUrlCheck

    enum class Problem { FORMAT, SCHEME }
}

object SignalUrls {
    /** Hosts a debug build may reach over plain ws:// (the developer's own machine). */
    val DEV_HOSTS = setOf("localhost", "127.0.0.1", "10.0.2.2")

    /**
     * Accepts only wss:// URLs with a host (the app talks to the internet:
     * always encrypted). With allowDevHosts (debug builds only), ws:// to
     * one of DEV_HOSTS is accepted too.
     */
    fun check(raw: String, allowDevHosts: Boolean = false): SignalUrlCheck {
        val text = raw.trim()
        if (text.isEmpty() || text.length > 300) return SignalUrlCheck.Bad(SignalUrlCheck.Problem.FORMAT)
        val uri = try {
            URI(text)
        } catch (_: Exception) {
            return SignalUrlCheck.Bad(SignalUrlCheck.Problem.FORMAT)
        }
        val scheme = uri.scheme?.lowercase()
        val devWs = allowDevHosts && scheme == "ws" && uri.host?.lowercase() in DEV_HOSTS
        if (scheme != "wss" && !devWs) return SignalUrlCheck.Bad(SignalUrlCheck.Problem.SCHEME)
        if (uri.host.isNullOrEmpty() || uri.userInfo != null) return SignalUrlCheck.Bad(SignalUrlCheck.Problem.FORMAT)
        val path = if (uri.rawPath.isNullOrEmpty()) "/" else uri.rawPath
        val port = if (uri.port > 0) ":${uri.port}" else ""
        val query = uri.rawQuery?.let { "?$it" } ?: ""
        return SignalUrlCheck.Ok("$scheme://${uri.host.lowercase()}$port$path$query")
    }

    /** The server to use: a valid stored custom one, else the official one. */
    fun resolve(stored: String?, allowDevHosts: Boolean = false): Choice {
        if (stored != null) {
            val c = check(stored, allowDevHosts)
            if (c is SignalUrlCheck.Ok && c.url != Protocol.OFFICIAL_SIGNAL_URL) return Choice(c.url, custom = true)
        }
        return Choice(Protocol.OFFICIAL_SIGNAL_URL, custom = false)
    }

    data class Choice(val url: String, val custom: Boolean)
}
