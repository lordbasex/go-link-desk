// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import java.net.HttpURLConnection
import java.net.URL
import org.golink.desk.core.Recents
import org.json.JSONObject

/** The demonstration computer's current code (desk.go-link.org/demo). */
object DemoCode {
    private const val URL_DEMO = "https://desk.go-link.org/demo.json"

    /** Reads it (blocking: call off the main thread); throws when it is not online. */
    fun fetch(): String {
        val c = URL(URL_DEMO).openConnection() as HttpURLConnection
        try {
            c.connectTimeout = 10_000
            c.readTimeout = 10_000
            c.useCaches = false
            c.setRequestProperty("Accept", "application/json")
            if (c.responseCode != 200) throw IllegalStateException("demo: HTTP ${c.responseCode}")
            val f = JSONObject(c.inputStream.bufferedReader().use { it.readText() })
            val code = f.optString("code")
            if (!f.optBoolean("online") || Recents.digits(code) == null) throw IllegalStateException("demo: offline")
            return code
        } finally {
            c.disconnect()
        }
    }
}
