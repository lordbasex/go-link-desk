// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package org.golink.desk.core

import java.math.BigInteger
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.util.Base64
import java.security.interfaces.ECPublicKey

/**
 * Asymmetric link authentication, the viewer's half. Each link has an ECDSA
 * P-256 key; its private half is kept non-extractable (the app's Android
 * Keystore), so it can sign but never be read. The viewer also pins the
 * computer's device public key and checks every proof the computer signs
 * with it.
 *
 * A public key is the 65-byte uncompressed point (x9.63), a signature is the
 * 64-byte P1363 pair (R||S), both base64url. Java's "SHA256withECDSA" speaks
 * DER, so signatures are converted to and from R||S for the wire, which is
 * what the host and the web and iOS viewers use. Signing hashes with SHA-256.
 */

/** Signs a link's proof. The private key never leaves the signer. */
interface LinkSigner {
    /** Signs a message; returns the 64-byte R||S, base64url. */
    fun sign(message: ByteArray): String
}

/** A key just made for a new link (claim), and how to keep it under its id. */
class MadeKey(val publicKey: String, val signer: LinkSigner, val persist: (String) -> Unit)

/** Where the viewer makes and finds its link keys. */
interface LinkKeys {
    /** Makes a fresh link key (claim). */
    fun make(): MadeKey

    /** The signer for a link coming back (reach), or null if it is gone. */
    fun signer(linkId: String): LinkSigner?
}

/** A viewer with no key store: a code-only session that never links. */
object NoLinkKeys : LinkKeys {
    override fun make(): MadeKey = throw IllegalStateException("no key store")
    override fun signer(linkId: String): LinkSigner? = null
}

object LinkAuth {
    private val p256: ECParameterSpec by lazy {
        val p = AlgorithmParameters.getInstance("EC")
        p.init(ECGenParameterSpec("secp256r1"))
        p.getParameterSpec(ECParameterSpec::class.java)
    }

    fun hostProofMessage(nonce: String, host: String, viewer: String): ByteArray =
        "go-link-desk host proof v1\n$nonce\n$host\n$viewer".toByteArray(Charsets.UTF_8)

    fun linkProofMessage(nonce: String, host: String, viewer: String): ByteArray =
        "go-link-desk link proof v1\n$nonce\n$host\n$viewer".toByteArray(Charsets.UTF_8)

    fun hostUnlinkMessage(nonce: String, host: String, viewer: String): ByteArray =
        "go-link-desk host unlink v1\n$nonce\n$host\n$viewer".toByteArray(Charsets.UTF_8)

    /** Verifies sig (64-byte R||S, base64url) against a public key. Never throws. */
    fun verify(publicKey: String, message: ByteArray, sig: String): Boolean = runCatching {
        val raw = Base64.getUrlDecoder().decode(sig)
        if (raw.size != 64) return false
        val pub = importPublic(publicKey) ?: return false
        val s = Signature.getInstance("SHA256withECDSA")
        s.initVerify(pub)
        s.update(message)
        s.verify(rawToDer(raw))
    }.getOrDefault(false)

    /** Reads a public key from its 65-byte uncompressed point (base64url). */
    fun importPublic(pub: String): ECPublicKey? = runCatching {
        val raw = Base64.getUrlDecoder().decode(pub)
        if (raw.size != 65 || raw[0].toInt() != 4) return null
        val x = BigInteger(1, raw.copyOfRange(1, 33))
        val y = BigInteger(1, raw.copyOfRange(33, 65))
        KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(ECPoint(x, y), p256)) as ECPublicKey
    }.getOrNull()

    fun base64url(bytes: ByteArray): String = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)

    /** The 65-byte uncompressed point of a public key, base64url. */
    fun publicKeyString(pub: ECPublicKey): String {
        val out = ByteArray(65)
        out[0] = 4
        to32(pub.w.affineX).copyInto(out, 1)
        to32(pub.w.affineY).copyInto(out, 33)
        return base64url(out)
    }

    /** DER ECDSA signature to the 64-byte R||S pair. */
    private fun derToRaw(der: ByteArray): ByteArray {
        // SEQUENCE(0x30) len; INTEGER(0x02) rLen r; INTEGER(0x02) sLen s.
        var i = 2
        if (der[1].toInt() and 0x80 != 0) i += der[1].toInt() and 0x7f
        require(der[i].toInt() == 0x02)
        val rLen = der[i + 1].toInt()
        val r = der.copyOfRange(i + 2, i + 2 + rLen)
        var j = i + 2 + rLen
        require(der[j].toInt() == 0x02)
        val sLen = der[j + 1].toInt()
        val s = der.copyOfRange(j + 2, j + 2 + sLen)
        val out = ByteArray(64)
        to32(BigInteger(1, r)).copyInto(out, 0)
        to32(BigInteger(1, s)).copyInto(out, 32)
        return out
    }

    /** The 64-byte R||S pair to a DER ECDSA signature. */
    private fun rawToDer(raw: ByteArray): ByteArray {
        val r = derInt(raw.copyOfRange(0, 32))
        val s = derInt(raw.copyOfRange(32, 64))
        val body = r + s
        return byteArrayOf(0x30, body.size.toByte()) + body
    }

    private fun derInt(v: ByteArray): ByteArray {
        var b = BigInteger(1, v).toByteArray() // minimal, sign-safe
        return byteArrayOf(0x02, b.size.toByte()) + b
    }

    private fun to32(n: BigInteger): ByteArray {
        val b = n.toByteArray()
        val out = ByteArray(32)
        when {
            b.size == 32 -> b.copyInto(out)
            b.size < 32 -> b.copyInto(out, 32 - b.size)
            else -> b.copyInto(out, 0, b.size - 32, b.size) // drop a leading sign byte
        }
        return out
    }

    fun rawSignature(der: ByteArray): String = base64url(derToRaw(der))

    fun newKeyPair(): java.security.KeyPair {
        val g = KeyPairGenerator.getInstance("EC")
        g.initialize(ECGenParameterSpec("secp256r1"))
        return g.generateKeyPair()
    }
}

/**
 * A link key held as a plain Java key. The app uses the Android Keystore
 * instead (non-extractable); this backs the portable tests and any place
 * without a keystore.
 */
class SoftwareLinkKey(private val keyPair: java.security.KeyPair = LinkAuth.newKeyPair()) : LinkSigner {
    val publicKey: String get() = LinkAuth.publicKeyString(keyPair.public as ECPublicKey)

    override fun sign(message: ByteArray): String {
        val s = Signature.getInstance("SHA256withECDSA")
        s.initSign(keyPair.private)
        s.update(message)
        return LinkAuth.rawSignature(s.sign())
    }
}
