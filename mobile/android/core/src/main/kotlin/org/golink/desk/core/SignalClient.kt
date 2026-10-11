// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.random.Random

/** A WebSocket the platform opens (OkHttp on Android, URLSessionWebSocketTask on iOS). */
interface SignalSocket {
    fun send(text: String): Boolean

    fun close()
}

/** Socket events. They may arrive on any thread: the client moves them onto its scope. */
interface SignalSocketListener {
    fun onMessage(text: String)

    fun onClosed()
}

fun interface SignalSocketFactory {
    fun open(url: String, listener: SignalSocketListener): SignalSocket
}

enum class ConnectionState { CONNECTING, OPEN, CLOSED }

/** An error reply from signalhub (fromServer), or a local failure. */
class SignalException(message: String, val fromServer: Boolean) : Exception(message)

/**
 * Client for signalhub, a port of the web's SignalClient
 * (frontend/packages/shared/src/signal-client.ts). It reconnects with
 * jittered exponential backoff, keeps the ICE servers from hello in memory
 * only, and offers request() for messages that expect one reply. Replies
 * arrive in the order requests were sent (signalhub handles one
 * connection's messages in order), so pending requests form a FIFO queue.
 *
 * Every method must be called on scope's dispatcher (the main thread on
 * Android), and listeners run there too. Native clients send no Origin
 * header, which signalhub accepts.
 */
class SignalClient(
    val url: String,
    private val factory: SignalSocketFactory,
    private val scope: CoroutineScope,
    private val minBackoffMs: Long = 1_000,
    private val maxBackoffMs: Long = 30_000,
    private val requestTimeoutMs: Long = 10_000,
    private val random: Random = Random.Default,
) {
    private class Pending(val expect: Set<String>, val result: CompletableDeferred<Envelope>)

    private var socket: SignalSocket? = null
    private var generation = 0
    private var stopped = true
    private var backoff = minBackoffMs
    private var retryJob: Job? = null
    private val pending = ArrayDeque<Pending>()
    private val listeners = LinkedHashSet<(Envelope) -> Unit>()

    private val _state = MutableStateFlow(ConnectionState.CLOSED)
    val state: StateFlow<ConnectionState> = _state.asStateFlow()

    /** peer_id assigned in hello; empty while disconnected. */
    var peerId: String = ""
        private set

    /** STUN/TURN servers from hello, for the next peer connection. */
    var iceServers: List<IceServer> = emptyList()
        private set

    /** Starts connecting (and reconnecting) until close() is called. */
    fun connect() {
        if (!stopped) return
        stopped = false
        open()
    }

    /** Closes the link for good and fails pending requests. */
    fun close() {
        stopped = true
        retryJob?.cancel()
        retryJob = null
        val s = socket
        socket = null
        generation++
        s?.close()
        handleDown()
    }

    /**
     * Drops the connection and opens a new one right away. A signalhub
     * connection belongs to at most one session, so this is how the app
     * leaves a session.
     */
    fun reset() {
        close()
        backoff = minBackoffMs
        connect()
    }

    fun addListener(fn: (Envelope) -> Unit): () -> Unit {
        listeners.add(fn)
        return { listeners.remove(fn) }
    }

    /** Sends one message; false while disconnected. */
    fun send(env: JsonObject): Boolean {
        if (_state.value != ConnectionState.OPEN) return false
        return socket?.send(env.toString()) ?: false
    }

    /** Waits until the link is open (hello arrived); false on timeout. */
    suspend fun awaitOpen(timeoutMs: Long = requestTimeoutMs): Boolean =
        withTimeoutOrNull(timeoutMs) {
            state.collectUntilOpen()
            true
        } ?: false

    private suspend fun StateFlow<ConnectionState>.collectUntilOpen() {
        if (value == ConnectionState.OPEN) return
        val done = CompletableDeferred<Unit>()
        val job = scope.launch { collect { if (it == ConnectionState.OPEN) done.complete(Unit) } }
        try {
            done.await()
        } finally {
            job.cancel()
        }
    }

    /**
     * Sends env and returns the next reply whose type is in expect. An
     * "error" reply throws SignalException with the server's text.
     */
    suspend fun request(env: JsonObject, expect: Set<String>): Envelope {
        if (_state.value != ConnectionState.OPEN && !awaitOpen()) {
            throw SignalException("signaling server unreachable", false)
        }
        val entry = Pending(expect, CompletableDeferred())
        pending.addLast(entry)
        if (!send(env)) {
            pending.remove(entry)
            throw SignalException("not connected", false)
        }
        val reply = withTimeoutOrNull(requestTimeoutMs) { entry.result.await() }
        if (reply == null) {
            pending.remove(entry)
            throw SignalException("request timed out", false)
        }
        return reply
    }

    private fun open() {
        _state.value = ConnectionState.CONNECTING
        val gen = ++generation
        val listener = object : SignalSocketListener {
            override fun onMessage(text: String) {
                scope.launch { if (gen == generation) handleRaw(text) }
            }

            override fun onClosed() {
                scope.launch {
                    if (gen != generation) return@launch // a stale socket after reset()
                    socket = null
                    handleDown()
                    scheduleRetry()
                }
            }
        }
        socket = try {
            factory.open(endpoint(url), listener)
        } catch (_: Exception) {
            null
        }
        if (socket == null) scheduleRetry()
    }

    private fun handleRaw(text: String) {
        val env = Envelope.parse(text) ?: return
        if (env.type == "hello") {
            peerId = env.peerId
            iceServers = IceServers.parse(env.raw["ice_servers"])
            backoff = minBackoffMs
            _state.value = ConnectionState.OPEN
            return
        }
        val head = pending.firstOrNull()
        if (head != null && (env.type == "error" || env.type in head.expect)) {
            pending.removeFirst()
            if (env.type == "error") head.result.completeExceptionally(SignalException(env.error.ifEmpty { "unknown error" }, true))
            else head.result.complete(env)
        }
        for (fn in listeners.toList()) fn(env)
    }

    private fun handleDown() {
        peerId = ""
        iceServers = emptyList()
        val failed = pending.toList()
        pending.clear()
        for (p in failed) p.result.completeExceptionally(SignalException("disconnected", false))
        _state.value = if (stopped) ConnectionState.CLOSED else ConnectionState.CONNECTING
    }

    private fun scheduleRetry() {
        if (stopped) return
        // Full jitter: many clients do not reconnect at the same instant.
        val wait = (random.nextDouble() * backoff + minBackoffMs / 2.0).toLong()
        backoff = minOf(backoff * 2, maxBackoffMs)
        retryJob?.cancel()
        retryJob = scope.launch {
            delay(wait)
            retryJob = null
            if (!stopped) open()
        }
    }

    companion object {
        /**
         * Opens a throwaway connection and waits for hello. Used before
         * saving a custom signaling server. Returns null when it answered,
         * or a short reason.
         */
        suspend fun test(url: String, factory: SignalSocketFactory, timeoutMs: Long = 5_000): String? {
            // "" means it answered: withTimeoutOrNull's null is the timeout.
            val result = CompletableDeferred<String>()
            val socket = try {
                factory.open(
                    endpoint(url),
                    object : SignalSocketListener {
                        override fun onMessage(text: String) {
                            val type = parseObject(text)?.get("type")?.let { runCatching { it.jsonPrimitive.content }.getOrNull() }
                            if (type == "hello") result.complete("")
                        }

                        override fun onClosed() {
                            result.complete("could not connect")
                        }
                    },
                )
            } catch (_: Exception) {
                return "could not connect"
            }
            val outcome = withTimeoutOrNull(timeoutMs) { result.await() } ?: "no answer from the server"
            socket.close()
            return outcome.ifEmpty { null }
        }
    }
}
