// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioManager
import android.media.AudioTrack
import kotlin.math.PI
import kotlin.math.exp
import kotlin.math.min
import kotlin.math.sin

/**
 * The sound of a chat message from the person at the computer: the
 * website's two short soft notes (880 and 1320 Hz, frontend's
 * chatSound.ts), made here (no file). Never with the phone on silent or
 * vibrate.
 */
object ChatSound {
    private const val RATE = 44_100

    private val pcm: ShortArray by lazy {
        val out = ShortArray((RATE * 0.29).toInt())
        for ((i, freq) in listOf(880.0, 1320.0).withIndex()) {
            val start = (RATE * 0.11 * i).toInt()
            val n = (RATE * 0.17).toInt()
            for (k in 0 until n) {
                val t = k.toDouble() / RATE
                // 10 ms up to 0.18, then down to nothing by 160 ms.
                val gain = if (t < 0.01) 0.18 * t / 0.01 else 0.18 * exp(-(t - 0.01) / 0.15 * 6.9)
                val v = out[start + k] + (sin(2 * PI * freq * t) * gain * Short.MAX_VALUE).toInt()
                out[start + k] = v.coerceIn(Short.MIN_VALUE.toInt(), Short.MAX_VALUE.toInt()).toShort()
            }
        }
        out
    }

    fun play(context: Context) {
        val audio = context.getSystemService(AudioManager::class.java)
        if (audio.ringerMode != AudioManager.RINGER_MODE_NORMAL) return
        runCatching {
            val data = pcm
            val track = AudioTrack.Builder()
                .setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_NOTIFICATION_EVENT)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                        .build(),
                )
                .setAudioFormat(
                    AudioFormat.Builder()
                        .setSampleRate(RATE)
                        .setEncoding(AudioFormat.ENCODING_PCM_16BIT)
                        .setChannelMask(AudioFormat.CHANNEL_OUT_MONO)
                        .build(),
                )
                .setTransferMode(AudioTrack.MODE_STATIC)
                .setBufferSizeInBytes(data.size * 2)
                .build()
            track.write(data, 0, data.size)
            track.setNotificationMarkerPosition(min(data.size, data.size - 1))
            track.setPlaybackPositionUpdateListener(object : AudioTrack.OnPlaybackPositionUpdateListener {
                override fun onMarkerReached(t: AudioTrack) = t.release()

                override fun onPeriodicNotification(t: AudioTrack) = Unit
            })
            track.play()
        }
    }
}
