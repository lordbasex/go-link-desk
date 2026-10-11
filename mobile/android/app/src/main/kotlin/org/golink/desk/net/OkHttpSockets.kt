// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.net

import android.util.Log
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.golink.desk.core.SignalSocket
import org.golink.desk.core.SignalSocketFactory
import org.golink.desk.core.SignalSocketListener
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/**
 * signalhub WebSockets through OkHttp. No Origin header is sent: signalhub
 * accepts native clients without one. Messages larger than signalhub's
 * 64 KB limit are never produced by the app.
 */
class OkHttpSockets(private val client: OkHttpClient = shared) : SignalSocketFactory {
    override fun open(url: String, listener: SignalSocketListener): SignalSocket {
        val closed = AtomicBoolean(false)
        fun down() {
            if (closed.compareAndSet(false, true)) listener.onClosed()
        }
        val ws = client.newWebSocket(
            Request.Builder().url(url).build(),
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, text: String) {
                    if (text.length <= MAX_MESSAGE) listener.onMessage(text)
                }

                override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                    webSocket.close(1000, null)
                    down()
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) = down()

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    Log.w("go-link-desk", "signaling socket failed: ${t.javaClass.simpleName}: ${t.message}${response?.let { " (HTTP ${it.code})" } ?: ""}")
                    down()
                }
            },
        )
        return object : SignalSocket {
            override fun send(text: String): Boolean = ws.send(text)

            override fun close() {
                ws.close(1000, null)
                // OkHttp reports the close asynchronously; the client ignores
                // events from a socket it already dropped.
            }
        }
    }

    companion object {
        private const val MAX_MESSAGE = 256 * 1024

        val shared: OkHttpClient by lazy {
            OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .pingInterval(20, TimeUnit.SECONDS)
                .build()
        }
    }
}
