// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.content.ContentValues
import android.content.Context
import android.graphics.Bitmap
import android.media.MediaCodec
import android.media.MediaCodecInfo
import android.media.MediaFormat
import android.media.MediaMuxer
import android.os.Handler
import android.os.HandlerThread
import android.os.SystemClock
import android.provider.MediaStore
import org.webrtc.AudioTrackSink
import org.webrtc.VideoFrame
import org.webrtc.VideoSink
import java.io.File
import java.nio.ByteBuffer

/*
 * Screenshots and recordings of the computer's screen, made here from the
 * picture this device already gets (docs/protocol.md: the computer is asked
 * first, and shows it). They go to the gallery: Pictures and Movies ›
 * go-link Desk.
 */

/** Keeps the latest picture for a screenshot, and feeds a recording. */
class FrameTap : VideoSink {
    private val lock = Any()
    private var last: VideoFrame? = null

    @Volatile var recorder: SessionRecorder? = null

    /** libwebrtc's decoding thread. */
    override fun onFrame(frame: VideoFrame) {
        frame.retain()
        synchronized(lock) {
            last?.release()
            last = frame
        }
        recorder?.video(frame)
    }

    /** The picture now, as a bitmap (BT.601, video range: what the hosts send). */
    fun snapshot(): Bitmap? {
        val f = synchronized(lock) { last?.also { it.retain() } } ?: return null
        try {
            val i = f.buffer.toI420() ?: return null
            try {
                val w = i.width
                val h = i.height
                val px = IntArray(w * h)
                val y = i.dataY
                val u = i.dataU
                val v = i.dataV
                for (r in 0 until h) {
                    for (c in 0 until w) {
                        val yy = (y.get(r * i.strideY + c).toInt() and 0xff) - 16
                        val uu = (u.get((r / 2) * i.strideU + c / 2).toInt() and 0xff) - 128
                        val vv = (v.get((r / 2) * i.strideV + c / 2).toInt() and 0xff) - 128
                        val yc = 1.164f * yy
                        val red = (yc + 1.596f * vv).toInt().coerceIn(0, 255)
                        val green = (yc - 0.392f * uu - 0.813f * vv).toInt().coerceIn(0, 255)
                        val blue = (yc + 2.017f * uu).toInt().coerceIn(0, 255)
                        px[r * w + c] = (0xff shl 24) or (red shl 16) or (green shl 8) or blue
                    }
                }
                return Bitmap.createBitmap(px, w, h, Bitmap.Config.ARGB_8888)
            } finally {
                i.release()
            }
        } finally {
            f.release()
        }
    }

    fun clear() {
        synchronized(lock) {
            last?.release()
            last = null
        }
    }
}

/**
 * Writes the pictures (H.264) and the computer's sound (AAC) to an MPEG-4
 * file with the system's encoders, on a thread of its own.
 */
class SessionRecorder(val file: File, private val withAudio: Boolean) : AudioTrackSink {
    private val thread = HandlerThread("go-link-desk-recorder").apply { start() }
    private val h = Handler(thread.looper)
    private var muxer: MediaMuxer? = null
    private var videoCodec: MediaCodec? = null
    private var audioCodec: MediaCodec? = null
    private var videoTrack = -1
    private var audioTrack = -1
    private var started = false
    private var width = 0
    private var height = 0
    private var startUs = -1L
    private var lastVideoUs = -1L
    private var videoSamples = 0
    private var audioAnchorUs = -1L
    private var audioFrames = 0L
    private var audioRate = 0
    private var openedAt = 0L
    private var finishing = false
    private val info = MediaCodec.BufferInfo()

    private fun nowUs() = SystemClock.elapsedRealtimeNanos() / 1000

    fun video(frame: VideoFrame) {
        frame.retain()
        val at = nowUs()
        h.post {
            try {
                writeVideo(frame, at)
            } catch (_: Exception) {
            } finally {
                frame.release()
            }
        }
    }

    private fun openVideo(w0: Int, h0: Int): Boolean {
        if (videoCodec != null) return true
        var w = w0
        var hh = h0
        if (w > MAX_WIDTH) {
            hh = hh * MAX_WIDTH / w
            w = MAX_WIDTH
        }
        w = w and 1.inv()
        hh = hh and 1.inv()
        if (w <= 0 || hh <= 0) return false
        val fmt = MediaFormat.createVideoFormat(MediaFormat.MIMETYPE_VIDEO_AVC, w, hh).apply {
            setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Flexible)
            setInteger(MediaFormat.KEY_BIT_RATE, 6_000_000)
            setInteger(MediaFormat.KEY_FRAME_RATE, 30)
            setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, 2)
        }
        val c = MediaCodec.createEncoderByType(MediaFormat.MIMETYPE_VIDEO_AVC)
        c.configure(fmt, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
        c.start()
        videoCodec = c
        width = w
        height = hh
        muxer = MediaMuxer(file.path, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)
        startUs = nowUs()
        openedAt = SystemClock.elapsedRealtime()
        return true
    }

    private fun writeVideo(frame: VideoFrame, at: Long) {
        if (finishing || !openVideo(frame.buffer.width, frame.buffer.height)) return
        val c = videoCodec ?: return
        val pts = at - startUs
        if (pts <= lastVideoUs) return
        val index = c.dequeueInputBuffer(0)
        if (index < 0) return drain(c, isVideo = true)
        val scaled = if (frame.buffer.width == width && frame.buffer.height == height) {
            frame.buffer.also { it.retain() }
        } else {
            frame.buffer.cropAndScale(0, 0, frame.buffer.width, frame.buffer.height, width, height)
        }
        val i = scaled.toI420()
        scaled.release()
        if (i == null) {
            c.queueInputBuffer(index, 0, 0, pts, 0)
            return
        }
        try {
            val image = c.getInputImage(index) ?: return
            val planes = image.planes
            copyPlane(i.dataY, i.strideY, width, height, planes[0].buffer, planes[0].rowStride, planes[0].pixelStride)
            copyPlane(i.dataU, i.strideU, width / 2, height / 2, planes[1].buffer, planes[1].rowStride, planes[1].pixelStride)
            copyPlane(i.dataV, i.strideV, width / 2, height / 2, planes[2].buffer, planes[2].rowStride, planes[2].pixelStride)
            c.queueInputBuffer(index, 0, width * height * 3 / 2, pts, 0)
            lastVideoUs = pts
        } finally {
            i.release()
        }
        drain(c, isVideo = true)
        maybeStart() // the sound did not come in time: the picture alone
    }

    private fun copyPlane(src: ByteBuffer, srcStride: Int, w: Int, hh: Int, dst: ByteBuffer, dstStride: Int, pixelStride: Int) {
        for (r in 0 until hh) {
            if (pixelStride == 1) {
                val s = src.duplicate().apply { position(r * srcStride); limit(r * srcStride + w) }
                dst.position(r * dstStride)
                dst.put(s)
            } else {
                for (x in 0 until w) dst.put(r * dstStride + x * pixelStride, src.get(r * srcStride + x))
            }
        }
    }

    /** libwebrtc's audio thread: the computer's sound as played. */
    override fun onData(audioData: ByteBuffer, bitsPerSample: Int, sampleRate: Int, channels: Int, frames: Int, timestamp: Long) {
        if (!withAudio || bitsPerSample != 16 || frames <= 0) return
        val bytes = ByteArray(frames * channels * 2)
        audioData.duplicate().get(bytes, 0, minOf(bytes.size, audioData.remaining()))
        val at = nowUs()
        h.post {
            try {
                writeAudio(bytes, sampleRate, channels, frames, at)
            } catch (_: Exception) {
            }
        }
    }

    private fun writeAudio(bytes: ByteArray, rate: Int, channels: Int, frames: Int, at: Long) {
        if (finishing || startUs < 0 || at < startUs) return
        if (audioCodec == null) {
            if (started) return // the file began without sound
            val fmt = MediaFormat.createAudioFormat(MediaFormat.MIMETYPE_AUDIO_AAC, rate, channels).apply {
                setInteger(MediaFormat.KEY_AAC_PROFILE, MediaCodecInfo.CodecProfileLevel.AACObjectLC)
                setInteger(MediaFormat.KEY_BIT_RATE, 128_000)
                setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, 64 * 1024)
            }
            val c = MediaCodec.createEncoderByType(MediaFormat.MIMETYPE_AUDIO_AAC)
            c.configure(fmt, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE)
            c.start()
            audioCodec = c
            audioRate = rate
        }
        val c = audioCodec ?: return
        var pts = if (audioAnchorUs >= 0) audioAnchorUs + audioFrames * 1_000_000 / audioRate else at - startUs
        if (audioAnchorUs < 0 || kotlin.math.abs((at - startUs) - pts) > 250_000) {
            audioAnchorUs = at - startUs
            audioFrames = 0
            pts = audioAnchorUs
        }
        audioFrames += frames
        val index = c.dequeueInputBuffer(0)
        if (index >= 0) {
            val buf = c.getInputBuffer(index) ?: return
            buf.clear()
            val n = minOf(bytes.size, buf.remaining())
            buf.put(bytes, 0, n)
            c.queueInputBuffer(index, 0, n, pts, 0)
        }
        drain(c, isVideo = false)
    }

    /** Takes what an encoder made; the file starts once its tracks are known. */
    private fun drain(c: MediaCodec, isVideo: Boolean) {
        val m = muxer ?: return
        while (true) {
            val index = c.dequeueOutputBuffer(info, 0)
            when {
                index == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED -> {
                    if (started) continue
                    if (isVideo) videoTrack = m.addTrack(c.outputFormat) else audioTrack = m.addTrack(c.outputFormat)
                    maybeStart()
                }
                index < 0 -> return
                else -> {
                    val out = c.getOutputBuffer(index)
                    val config = info.flags and MediaCodec.BUFFER_FLAG_CODEC_CONFIG != 0
                    if (out != null && started && !config && info.size > 0) {
                        m.writeSampleData(if (isVideo) videoTrack else audioTrack, out, info)
                        if (isVideo) videoSamples++
                    }
                    c.releaseOutputBuffer(index, false)
                }
            }
        }
    }

    /** The file starts with the picture's track, and the sound's when it comes in time. */
    private fun maybeStart() {
        if (started || videoTrack < 0) return
        val waitedForSound = SystemClock.elapsedRealtime() - openedAt > 1000
        if (withAudio && audioTrack < 0 && !waitedForSound) return
        muxer?.start()
        started = true
        // The first picture written is a whole one.
        videoCodec?.setParameters(android.os.Bundle().apply { putInt(MediaCodec.PARAMETER_KEY_REQUEST_SYNC_FRAME, 0) })
    }

    /** Closes the file; done (on the recorder's thread) says whether it holds a picture. */
    fun finish(done: (Boolean) -> Unit) {
        h.post {
            finishing = true
            var ok = false
            try {
                videoCodec?.let { drain(it, true) }
                audioCodec?.let { drain(it, false) }
                if (started) muxer?.stop()
                ok = started && videoSamples > 0
            } catch (_: Exception) {
                ok = false
            }
            runCatching { videoCodec?.stop() }
            runCatching { videoCodec?.release() }
            runCatching { audioCodec?.stop() }
            runCatching { audioCodec?.release() }
            runCatching { muxer?.release() }
            if (!ok) file.delete()
            done(ok)
            thread.quitSafely()
        }
    }

    companion object {
        /** Wider pictures are scaled down: what every phone's encoder takes. */
        const val MAX_WIDTH = 1920
    }
}

/** The gallery: Pictures and Movies › go-link-desk, with no permission (MediaStore). */
object MediaSaver {
    private const val FOLDER = "go-link-desk"

    fun saveImage(context: Context, bitmap: Bitmap, name: String): Boolean = runCatching {
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, name)
            put(MediaStore.Images.Media.MIME_TYPE, "image/png")
            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/$FOLDER")
        }
        val uri = context.contentResolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values) ?: return false
        context.contentResolver.openOutputStream(uri)?.use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) } ?: return false
        true
    }.getOrDefault(false)

    fun saveVideo(context: Context, file: File, name: String): Boolean = runCatching {
        val values = ContentValues().apply {
            put(MediaStore.Video.Media.DISPLAY_NAME, name)
            put(MediaStore.Video.Media.MIME_TYPE, "video/mp4")
            put(MediaStore.Video.Media.RELATIVE_PATH, "Movies/$FOLDER")
        }
        val uri = context.contentResolver.insert(MediaStore.Video.Media.EXTERNAL_CONTENT_URI, values) ?: return false
        context.contentResolver.openOutputStream(uri)?.use { out -> file.inputStream().use { it.copyTo(out) } } ?: return false
        true
    }.getOrDefault(false).also { file.delete() }

    /** "go-link Desk - Office Mac - 2026-10-14 15.04.05.png" */
    fun name(host: String, ext: String): String {
        val clean = host.map { if (it in "/\\:*?\"<>|" || it.isISOControl()) ' ' else it }.joinToString("").trim().split(Regex("\\s+")).joinToString(" ").take(40).trim()
        val stamp = java.text.SimpleDateFormat("yyyy-MM-dd HH.mm.ss", java.util.Locale.ROOT).format(java.util.Date())
        return "go-link Desk - " + (if (clean.isEmpty()) "" else "$clean - ") + "$stamp.$ext"
    }
}
