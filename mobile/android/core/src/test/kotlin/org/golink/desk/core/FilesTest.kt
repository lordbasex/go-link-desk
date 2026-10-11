// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** The files channel's client against a fake channel that answers as the computer does. */
@OptIn(ExperimentalCoroutinesApi::class)
class FilesTest {
    class Channel : FilesChannel {
        val texts = mutableListOf<JsonObject>()
        val data = mutableListOf<ByteArray>()
        override var bufferedAmount = 0L
        override var isOpen = true

        override fun sendText(text: String): Boolean {
            texts += parseObject(text)!!
            return isOpen
        }

        override fun sendData(bytes: ByteArray): Boolean {
            data += bytes
            return isOpen
        }
    }

    class Sink : DownloadSink {
        var bytes = ByteArray(0)

        override fun write(data: ByteArray, offset: Long) {
            val end = offset.toInt() + data.size
            if (bytes.size < end) bytes = bytes.copyOf(end)
            data.copyInto(bytes, offset.toInt())
        }
    }

    class Source(private var rest: ByteArray) : UploadSource {
        override fun read(max: Int): ByteArray {
            val n = minOf(max, rest.size)
            val out = rest.copyOf(n)
            rest = rest.copyOfRange(n, rest.size)
            return out
        }
    }

    private fun type(o: JsonObject?) = o?.get("type").str()

    @Test
    fun list() = runTest {
        val ch = Channel()
        val f = FilesClient(ch, backgroundScope)
        var got = emptyList<FileEntry>()
        f.list("Documents") { if (it is FilesResult.Ok) got = it.value }
        assertEquals(obj("""{"type":"list","id":"l1","path":"Documents"}"""), ch.texts.last())
        f.onText("""{"type":"listing","id":"l1","path":"Documents","entries":[{"name":"a","dir":true,"size":0,"mtime":1},{"name":"b.txt","dir":false,"size":5,"mtime":2}]}""")
        assertEquals(listOf("a", "b.txt"), got.map { it.name })
        assertTrue(got[0].dir)

        var err: FilesError? = null
        f.list("..") { if (it is FilesResult.Failed) err = it.error }
        f.onText("""{"type":"files_error","id":"l2","error":"not allowed"}""")
        assertEquals(FilesError("not allowed"), err)

        var late: FilesError? = null
        f.list("x") { if (it is FilesResult.Failed) late = it.error }
        runCurrent()
        advanceTimeBy(FilesClient.REPLY_WAIT_MS + 1)
        assertEquals(FilesError.NO_ANSWER, late)
    }

    @Test
    fun downloadInPiecesOutOfOrder() = runTest {
        val ch = Channel()
        val f = FilesClient(ch, backgroundScope)
        val sink = Sink()
        var result: FilesResult<String>? = null
        val progress = mutableListOf<Long>()
        f.download("Documents/x.bin", { name, size ->
            assertEquals("x.bin", name)
            assertEquals(70_000L, size)
            sink
        }, { got, _ -> progress += got }, { result = it })
        assertEquals(obj("""{"type":"download","id":"d1","path":"Documents/x.bin"}"""), ch.texts.last())
        f.onText("""{"type":"download_ready","id":"d1","name":"../x.bin","size":70000}""")
        assertEquals(2, ch.texts.count { type(it) == "read" }) // 60 KiB and the rest
        fun piece(off: Int, n: Int): ByteArray {
            val head = ByteArray(8) { ((off.toLong() shr (8 * (7 - it))) and 0xFF).toByte() }
            return head + ByteArray(n) { ((off + it) % 251).toByte() }
        }
        f.onBinary(piece(61_440, 70_000 - 61_440))
        f.onBinary(piece(0, 61_440))
        assertEquals(FilesResult.Ok("x.bin"), result)
        assertEquals("done", type(ch.texts.last()))
        assertEquals(70_000, sink.bytes.size)
        assertEquals((61_440 % 251).toByte(), sink.bytes[61_440])
        assertEquals(70_000L, progress.last())

        // A piece outside the file stops it.
        var bad: FilesResult<String>? = null
        f.download("y", { _, _ -> Sink() }, { _, _ -> }, { bad = it })
        f.onText("""{"type":"download_ready","id":"d2","name":"y","size":10}""")
        f.onBinary(piece(5, 10))
        assertEquals(FilesResult.Failed(FilesError("the computer sent a bad piece")), bad)
        assertEquals("cancel", type(ch.texts.last()))
    }

    @Test
    fun uploadWaitsForTheChannelToDrain() = runTest {
        val ch = Channel()
        val f = FilesClient(ch, backgroundScope)
        val bytes = ByteArray(40_000) { 7 }
        var result: FilesResult<String>? = null
        ch.bufferedAmount = FilesClient.UPLOAD_WINDOW + 1
        f.upload("dir/.photo.jpg", bytes.size.toLong(), Source(bytes), { _, _ -> }, { result = it })
        assertEquals(obj("""{"type":"begin","id":"u1","name":"photo.jpg","size":40000}"""), ch.texts.first())
        assertTrue(ch.data.isEmpty()) // the queue is full
        ch.bufferedAmount = 0
        f.onBufferedLow()
        assertEquals(listOf(16_384, 16_384, 7232), ch.data.map { it.size })
        assertEquals(obj("""{"type":"end","id":"u1"}"""), ch.texts.last())
        f.onText("""{"type":"upload_result","id":"u1","ok":true,"name":"photo (2).jpg"}""")
        assertEquals(FilesResult.Ok("photo (2).jpg"), result)

        // One upload at a time; a closed channel fails the running one.
        var a: FilesResult<String>? = null
        var b: FilesResult<String>? = null
        ch.bufferedAmount = FilesClient.UPLOAD_WINDOW + 1
        f.upload("a", 3, Source(byteArrayOf(1, 2, 3)), { _, _ -> }, { a = it })
        f.upload("b", 3, Source(byteArrayOf(1, 2, 3)), { _, _ -> }, { b = it })
        assertEquals(FilesResult.Failed(FilesError.BUSY), b)
        f.onClosed()
        assertEquals(FilesResult.Failed(FilesError.CLOSED), a)
    }

    @Test
    fun safeNames() {
        assertEquals("passwd", FilesClient.safeName("../../etc/passwd"))
        assertEquals("a.txt", FilesClient.safeName("C:\\Users\\x\\a.txt"))
        assertEquals("hidden", FilesClient.safeName("...hidden"))
        assertEquals("abc", FilesClient.safeName("a\u0000b:c"))
        assertEquals("", FilesClient.safeName(".."))
    }
}
