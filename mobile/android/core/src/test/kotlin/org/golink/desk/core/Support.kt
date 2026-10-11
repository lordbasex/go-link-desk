// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlinx.serialization.json.JsonObject
import java.io.File

/** A signalhub stand-in: records what the client sends and lets tests answer. */
class FakeSockets : SignalSocketFactory {
    inner class Conn(val url: String, val listener: SignalSocketListener) : SignalSocket {
        val sent = mutableListOf<JsonObject>()
        val sentText = mutableListOf<String>()
        var closed = false

        override fun send(text: String): Boolean {
            sent += parseObject(text)!!
            sentText += text
            return !closed
        }

        override fun close() {
            closed = true
        }

        fun receive(text: String) = listener.onMessage(text)
    }

    val conns = mutableListOf<Conn>()
    val last get() = conns.last()

    override fun open(url: String, listener: SignalSocketListener): SignalSocket = Conn(url, listener).also { conns += it }
}

class FakePeer : RtcPeer {
    val control = mutableListOf<String>()
    val pointer = mutableListOf<ByteArray>()
    val input = mutableListOf<ByteArray>()
    val candidates = mutableListOf<IceCandidateInit>()
    val offers = mutableListOf<String>()
    var closed = false
    lateinit var events: RtcPeerEvents

    override suspend fun answer(offerSdp: String): String {
        offers += offerSdp
        return "answer-for-$offerSdp"
    }

    override fun addCandidate(candidate: IceCandidateInit) {
        candidates += candidate
    }

    override fun sendControl(text: String): Boolean {
        control += text
        return true
    }

    override fun sendPointer(packet: ByteArray): Boolean {
        pointer += packet
        return true
    }

    override fun sendInput(packet: ByteArray): Boolean {
        input += packet
        return true
    }

    override fun close() {
        closed = true
    }
}

class FakePeers : RtcPeerFactory {
    val created = mutableListOf<FakePeer>()
    val servers = mutableListOf<List<IceServer>>()

    override fun create(iceServers: List<IceServer>, events: RtcPeerEvents): RtcPeer {
        val p = FakePeer()
        p.events = events
        created += p
        servers += iceServers
        return p
    }
}

fun obj(text: String): JsonObject = parseObject(text)!!

/** A file of the repository, from the core module's directory (where Gradle runs the tests). */
fun repoFile(path: String): File {
    var dir: File? = File("").absoluteFile
    while (dir != null && !File(dir, "protocol").isDirectory) dir = dir.parentFile
    return File(requireNotNull(dir) { "repository root" }, path)
}

fun hex(b: ByteArray): String = b.joinToString("") { "%02x".format(it) }
