// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

// The files channel (docs/protocol.md, "Files"): browse the computer's home
// folder, download a file, send one. A port of the iOS app's client
// (mobile/ios/DeskCore), itself a port of the Mac app's
// (backend-device/internal/viewer/files.go) and the website's (files.ts),
// on the scope's thread with callbacks. One upload and one download at a time.

/** The "files" data channel, as the client needs it (called on the scope's thread). */
interface FilesChannel {
    fun sendText(text: String): Boolean

    fun sendData(bytes: ByteArray): Boolean

    /** Bytes queued in the channel and not sent yet. */
    val bufferedAmount: Long
    val isOpen: Boolean
}

/** One entry of a folder: folders come first. mtime: seconds since 1970. */
data class FileEntry(val name: String, val dir: Boolean, val size: Long, val mtime: Long)

data class FilesError(val message: String) {
    companion object {
        val CLOSED = FilesError("the connection closed")
        val NO_ANSWER = FilesError("the computer did not answer")
        val STOPPED = FilesError("stopped")
        val BUSY = FilesError("another transfer is running")
    }
}

/** A result: the value, or why not. */
sealed interface FilesResult<out T> {
    data class Ok<T>(val value: T) : FilesResult<T>

    data class Failed(val error: FilesError) : FilesResult<Nothing>
}

/** Where a download's pieces go (a file). */
fun interface DownloadSink {
    fun write(data: ByteArray, offset: Long)
}

/** Where an upload's bytes come from (a file). */
fun interface UploadSource {
    /** Up to max bytes, or an empty array at the end. */
    fun read(max: Int): ByteArray
}

/** Something running that can be stopped. */
class Transfer {
    internal var stop: (() -> Unit)? = null

    fun cancel() {
        stop?.invoke()
    }
}

class FilesClient(private val channel: FilesChannel, private val scope: CoroutineScope) {
    private var seq = 0
    private val waiting = HashMap<String, (JsonObject) -> Unit>()
    private var binary: ((ByteArray) -> Unit)? = null
    private var drained: (() -> Unit)? = null
    private var uploading = false
    private var downloading = false
    private var closed = false
    private val closers = HashMap<String, () -> Unit>()

    // From the channel

    fun onText(text: String) {
        val m = parseObject(text) ?: return
        waiting[m["id"].str(40)]?.invoke(m)
    }

    fun onBinary(data: ByteArray) {
        binary?.invoke(data)
    }

    /** The channel's queue went down: an upload may go on. */
    fun onBufferedLow() {
        val d = drained
        drained = null
        d?.invoke()
    }

    fun onClosed() {
        if (closed) return
        closed = true
        for (close in closers.values.toList()) close()
        closers.clear()
        waiting.clear()
        binary = null
    }

    // Browsing

    /** The entries of a folder, relative to the computer's home folder ("" is the home folder). */
    fun list(path: String, done: (FilesResult<List<FileEntry>>) -> Unit) {
        val id = open("l")
        var timer: Job? = null
        val finish: (FilesResult<List<FileEntry>>) -> Unit = { r ->
            timer?.cancel()
            forget(id)
            done(r)
        }
        closers[id] = { finish(FilesResult.Failed(FilesError.CLOSED)) }
        waiting[id] = { m ->
            when (m["type"].str(30)) {
                "listing" -> {
                    val entries = m["entries"].arr().take(MAX_ENTRIES).mapNotNull { e ->
                        val o = e.obj()
                        val name = o["name"].str(255)
                        if (name.isEmpty()) null else FileEntry(name, o["dir"].isTrue(), o["size"].num().coerceAtLeast(0.0).toLong(), o["mtime"].num().toLong())
                    }
                    finish(FilesResult.Ok(entries))
                }
                "files_error" -> finish(FilesResult.Failed(FilesError(m["error"].str(200))))
            }
        }
        timer = after(REPLY_WAIT_MS) { finish(FilesResult.Failed(FilesError.NO_ANSWER)) }
        if (!send(msg("list", id) { put("path", path) })) finish(FilesResult.Failed(FilesError.CLOSED))
    }

    // Downloads

    /**
     * Downloads path into a sink: ready gets the file's name and size first
     * (to make the file), then progress, then done with the name.
     */
    fun download(
        path: String,
        ready: (name: String, size: Long) -> DownloadSink,
        progress: (got: Long, size: Long) -> Unit,
        done: (FilesResult<String>) -> Unit,
    ): Transfer {
        val t = Transfer()
        if (downloading) {
            done(FilesResult.Failed(FilesError.BUSY))
            return t
        }
        downloading = true
        val id = open("d")
        var timer: Job? = null
        var finished = false
        var sink: DownloadSink? = null
        var name = ""
        var size = 0L
        var next = 0L
        var got = 0L
        var inFlight = 0
        val finish: (FilesResult<String>) -> Unit = { r ->
            if (!finished) {
                finished = true
                timer?.cancel()
                binary = null
                downloading = false
                forget(id)
                done(r)
            }
        }
        val rearm = {
            timer?.cancel()
            timer = after(REPLY_WAIT_MS) { finish(FilesResult.Failed(FilesError.NO_ANSWER)) }
        }
        val ask = {
            while (inFlight < DOWNLOAD_IN_FLIGHT && next < size) {
                val n = minOf(DOWNLOAD_CHUNK, size - next)
                send(
                    msg("read", id) {
                        put("offset", next)
                        put("length", n)
                    },
                )
                next += n
                inFlight++
            }
        }
        val cancel = { send(msg("cancel", id)) }
        t.stop = {
            cancel()
            finish(FilesResult.Failed(FilesError.STOPPED))
        }
        closers[id] = { finish(FilesResult.Failed(FilesError.CLOSED)) }
        waiting[id] = { m ->
            when (m["type"].str(30)) {
                "download_ready" -> {
                    name = safeName(m["name"].str(255)).ifEmpty { safeName(path.substringAfterLast('/')) }
                    size = m["size"].num().coerceAtLeast(0.0).toLong()
                    val s = runCatching { ready(name, size) }
                    if (s.isFailure) {
                        cancel()
                        finish(FilesResult.Failed(FilesError(s.exceptionOrNull()?.message ?: "cannot write the file")))
                    } else {
                        sink = s.getOrThrow()
                        if (size == 0L) {
                            send(msg("done", id))
                            finish(FilesResult.Ok(name))
                        } else {
                            rearm()
                            ask()
                        }
                    }
                }
                "download_error", "files_error" -> finish(FilesResult.Failed(FilesError(m["error"].str(200))))
            }
        }
        binary = binary@{ data ->
            val out = sink ?: return@binary
            if (data.size < 8) return@binary
            var off = 0L
            for (i in 0 until 8) off = (off shl 8) or (data[i].toLong() and 0xFF)
            val piece = data.copyOfRange(8, data.size)
            if (off < 0 || off + piece.size > size) {
                cancel()
                finish(FilesResult.Failed(FilesError("the computer sent a bad piece")))
                return@binary
            }
            val w = runCatching { out.write(piece, off) }
            if (w.isFailure) {
                cancel()
                finish(FilesResult.Failed(FilesError(w.exceptionOrNull()?.message ?: "cannot write the file")))
                return@binary
            }
            got += piece.size
            inFlight--
            progress(got, size)
            if (got >= size) {
                send(msg("done", id))
                finish(FilesResult.Ok(name))
                return@binary
            }
            rearm()
            ask()
        }
        timer = after(REPLY_WAIT_MS) { finish(FilesResult.Failed(FilesError.NO_ANSWER)) }
        if (!send(msg("download", id) { put("path", path) })) finish(FilesResult.Failed(FilesError.CLOSED))
        return t
    }

    // Uploads

    /** Sends a file: done gets the name the computer saved it as (in Downloads/go-link Desk). */
    fun upload(
        name: String,
        size: Long,
        source: UploadSource,
        progress: (sent: Long, size: Long) -> Unit,
        done: (FilesResult<String>) -> Unit,
    ): Transfer {
        val t = Transfer()
        if (uploading) {
            done(FilesResult.Failed(FilesError.BUSY))
            return t
        }
        uploading = true
        val id = open("u")
        var timer: Job? = null
        var finished = false
        var sent = 0L
        var ended = false
        val finish: (FilesResult<String>) -> Unit = { r ->
            if (!finished) {
                finished = true
                timer?.cancel()
                drained = null
                uploading = false
                forget(id)
                done(r)
            }
        }
        t.stop = {
            send(msg("abort", id))
            finish(FilesResult.Failed(FilesError.STOPPED))
        }
        closers[id] = { finish(FilesResult.Failed(FilesError.CLOSED)) }
        waiting[id] = { m ->
            val type = m["type"].str(30)
            if (type == "upload_result" || type == "files_error") {
                if (m["ok"].isTrue()) {
                    finish(FilesResult.Ok(m["name"].str(255).ifEmpty { name }))
                } else {
                    finish(FilesResult.Failed(FilesError(m["error"].str(200))))
                }
            }
        }
        // Sends while the channel's queue is under the window, then waits for it to drain.
        lateinit var pump: () -> Unit
        pump = pump@{
            if (finished) return@pump
            while (!ended) {
                if (!channel.isOpen) {
                    finish(FilesResult.Failed(FilesError.CLOSED))
                    return@pump
                }
                if (channel.bufferedAmount > UPLOAD_WINDOW) {
                    drained = pump
                    // In case the low mark is never reported.
                    after(1000) {
                        val d = drained
                        if (d != null && !finished) {
                            drained = null
                            d()
                        }
                    }
                    return@pump
                }
                if (sent >= size) {
                    ended = true
                    break
                }
                val chunk = runCatching { source.read(UPLOAD_CHUNK) }
                if (chunk.isFailure) {
                    send(msg("abort", id))
                    finish(FilesResult.Failed(FilesError(chunk.exceptionOrNull()?.message ?: "cannot read the file")))
                    return@pump
                }
                val bytes = chunk.getOrThrow()
                if (bytes.isEmpty()) {
                    ended = true
                    break
                }
                if (!channel.sendData(bytes)) {
                    finish(FilesResult.Failed(FilesError.CLOSED))
                    return@pump
                }
                sent += bytes.size
                progress(sent, size)
            }
            send(msg("end", id))
            timer = after(REPLY_WAIT_MS) { finish(FilesResult.Failed(FilesError.NO_ANSWER)) }
        }
        val safe = safeName(name).ifEmpty { "file" }
        val begin = msg("begin", id) {
            put("name", safe)
            put("size", size)
        }
        if (!send(begin)) {
            finish(FilesResult.Failed(FilesError.CLOSED))
            return t
        }
        pump()
        return t
    }

    // Helpers

    private fun open(prefix: String): String {
        seq++
        return "$prefix$seq"
    }

    private fun forget(id: String) {
        waiting.remove(id)
        closers.remove(id)
    }

    private fun after(ms: Long, fn: () -> Unit): Job = scope.launch {
        delay(ms)
        fn()
    }

    private fun msg(type: String, id: String, more: kotlinx.serialization.json.JsonObjectBuilder.() -> Unit = {}): JsonObject = buildJsonObject {
        put("type", type)
        put("id", id)
        more()
    }

    private fun send(m: JsonObject): Boolean {
        if (closed || !channel.isOpen) return false
        return channel.sendText(m.toString())
    }

    companion object {
        const val UPLOAD_CHUNK = 16 shl 10
        const val UPLOAD_WINDOW = 1L shl 20
        const val DOWNLOAD_CHUNK = 60L shl 10
        const val DOWNLOAD_IN_FLIGHT = 16
        const val REPLY_WAIT_MS = 30_000L
        const val MAX_ENTRIES = 5000

        /** A name without any folder, control characters or leading dots ("" when nothing is left). */
        fun safeName(name: String): String {
            val last = name.replace('\\', '/').split('/').lastOrNull { it.isNotEmpty() } ?: ""
            val clean = last.filter { it.code >= 0x20 && it.code != 0x7F && it != ':' }
            return clean.trimStart('.')
        }
    }
}
