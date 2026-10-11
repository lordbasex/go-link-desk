// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** SignalClient.test: the check made before saving a custom server in Settings. */
class SignalTestTest {
    /** A socket that answers (or not) as soon as it opens. */
    private class Answering(private val reply: String?, private val closeInstead: Boolean = false) : SignalSocketFactory {
        var url = ""
        var closed = false

        override fun open(url: String, listener: SignalSocketListener): SignalSocket {
            this.url = url
            if (closeInstead) listener.onClosed() else reply?.let { listener.onMessage(it) }
            return object : SignalSocket {
                override fun send(text: String) = true

                override fun close() {
                    closed = true
                }
            }
        }
    }

    @Test
    fun aServerThatSaysHelloPasses() = runTest {
        val sockets = Answering("""{"type":"hello","peer_id":"p","ice_servers":[]}""")
        assertNull(SignalClient.test("ws://10.0.2.2:8191/ws", sockets))
        assertEquals("ws://10.0.2.2:8191/ws?v=1", sockets.url)
        assertTrue(sockets.closed)
    }

    @Test
    fun silenceOrAClosedSocketFails() = runTest {
        assertEquals("no answer from the server", SignalClient.test("wss://a.example/ws", Answering(null), timeoutMs = 100))
        assertEquals("no answer from the server", SignalClient.test("wss://a.example/ws", Answering("""{"type":"error"}"""), timeoutMs = 100))
        assertEquals("could not connect", SignalClient.test("wss://a.example/ws", Answering(null, closeInstead = true)))
    }
}
