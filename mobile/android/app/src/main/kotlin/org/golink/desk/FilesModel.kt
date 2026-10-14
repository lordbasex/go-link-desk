// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.content.ContentValues
import android.content.Context
import android.net.Uri
import android.os.Environment
import android.provider.MediaStore
import android.provider.OpenableColumns
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import org.golink.desk.core.DownloadSink
import org.golink.desk.core.FileEntry
import org.golink.desk.core.FilesClient
import org.golink.desk.core.FilesError
import org.golink.desk.core.FilesResult
import org.golink.desk.core.Transfer
import org.golink.desk.core.UploadSource
import java.io.File
import java.io.InputStream
import java.io.RandomAccessFile

/**
 * The computer's files as the Files sheet shows them: a folder's entries,
 * the transfer running (at most one download and one upload), and what
 * came of the last one. Downloads go to Downloads › go-link-desk, where
 * the Files app and every other app see them.
 */
class FilesModel(private val context: Context, private val client: FilesClient) {
    data class Progress(val name: String, val upload: Boolean, val done: Long, val size: Long) {
        val fraction: Float get() = if (size > 0) done.toFloat() / size else 0f
    }

    sealed interface Outcome {
        data class Downloaded(val name: String, val uri: Uri, val mime: String) : Outcome

        data class Uploaded(val name: String) : Outcome

        data class Failed(val why: String) : Outcome
    }

    var progress by mutableStateOf<Progress?>(null)
        private set
    var outcome by mutableStateOf<Outcome?>(null)
    private var transfer: Transfer? = null

    fun list(path: String, done: (FilesResult<List<FileEntry>>) -> Unit) = client.list(path, done)

    fun download(path: String) {
        if (progress != null) return
        val tmp = File(context.cacheDir, "download.part")
        tmp.delete()
        var file: RandomAccessFile? = null
        progress = Progress(path.substringAfterLast('/'), false, 0, 0)
        transfer = client.download(
            path,
            ready = { _, _ ->
                val f = RandomAccessFile(tmp, "rw").also { file = it }
                DownloadSink { data, offset ->
                    f.seek(offset)
                    f.write(data)
                }
            },
            progress = { got, size -> progress = progress?.copy(done = got, size = size) },
            done = { r ->
                file?.close()
                progress = null
                transfer = null
                when (r) {
                    is FilesResult.Ok -> outcome = runCatching { publish(tmp, r.value) }.getOrElse { Outcome.Failed(it.message ?: it.javaClass.simpleName) }
                    is FilesResult.Failed -> if (r.error != FilesError.STOPPED) outcome = Outcome.Failed(r.error.message)
                }
                tmp.delete()
            },
        )
    }

    /** Copies a finished download into Downloads › go-link-desk (the system picks a free name). */
    private fun publish(tmp: File, name: String): Outcome {
        val mime = android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(name.substringAfterLast('.', "").lowercase()) ?: "application/octet-stream"
        val values = ContentValues().apply {
            put(MediaStore.Downloads.DISPLAY_NAME, name)
            put(MediaStore.Downloads.MIME_TYPE, mime)
            put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/go-link-desk")
            put(MediaStore.Downloads.IS_PENDING, 1)
        }
        val resolver = context.contentResolver
        val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values) ?: error("cannot save in Downloads")
        try {
            resolver.openOutputStream(uri)!!.use { out -> tmp.inputStream().use { it.copyTo(out) } }
            resolver.update(uri, ContentValues().apply { put(MediaStore.Downloads.IS_PENDING, 0) }, null, null)
        } catch (e: Exception) {
            resolver.delete(uri, null, null)
            throw e
        }
        val saved = resolver.query(uri, arrayOf(MediaStore.Downloads.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) c.getString(0) else null
        } ?: name
        return Outcome.Downloaded(saved, uri, mime)
    }

    /** Sends a file of this device (from the Files app or the photos). */
    fun upload(uri: Uri) {
        if (progress != null) return
        val resolver = context.contentResolver
        var name = "file"
        var size = 0L
        resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { c ->
            if (c.moveToFirst()) {
                name = c.getString(0) ?: name
                if (!c.isNull(1)) size = c.getLong(1)
            }
        }
        val input: InputStream = runCatching { resolver.openInputStream(uri) }.getOrNull() ?: run {
            outcome = Outcome.Failed(context.getString(R.string.file_unreadable))
            return
        }
        progress = Progress(name, true, 0, size)
        val buffer = ByteArray(FilesClient.UPLOAD_CHUNK)
        transfer = client.upload(
            name,
            size,
            UploadSource { max ->
                val n = input.read(buffer, 0, minOf(max, buffer.size))
                if (n <= 0) ByteArray(0) else buffer.copyOf(n)
            },
            progress = { sent, total -> progress = progress?.copy(done = sent, size = total) },
            done = { r ->
                input.close()
                progress = null
                transfer = null
                when (r) {
                    is FilesResult.Ok -> outcome = Outcome.Uploaded(r.value)
                    is FilesResult.Failed -> if (r.error != FilesError.STOPPED) outcome = Outcome.Failed(r.error.message)
                }
            },
        )
    }

    fun cancel() {
        transfer?.cancel()
    }
}
