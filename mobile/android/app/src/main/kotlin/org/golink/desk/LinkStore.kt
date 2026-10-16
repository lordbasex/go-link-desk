// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk

import android.app.Activity
import android.app.KeyguardManager
import android.content.Context
import android.hardware.biometrics.BiometricManager
import android.hardware.biometrics.BiometricManager.Authenticators.BIOMETRIC_STRONG
import android.hardware.biometrics.BiometricManager.Authenticators.DEVICE_CREDENTIAL
import android.hardware.biometrics.BiometricPrompt
import android.os.CancellationSignal
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import org.golink.desk.core.DeviceLink
import org.golink.desk.core.LinkAuth
import org.golink.desk.core.LinkKeys
import org.golink.desk.core.LinkSigner
import org.golink.desk.core.MadeKey
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlin.coroutines.resume

/** A linked computer as My computers shows it. Its key is not here: it is in the keystore. Dates in ms since 1970. */
@Serializable
data class LinkedComputer(
    val deviceId: String,
    val linkId: String,
    val name: String,
    val os: String,
    val linkedAt: Long,
    val lastUsed: Long? = null,
    /** The computer's device public key, pinned when the link was made. */
    val hostKey: String = "",
)

/**
 * The computers this device is linked to, the Android counterpart of the iOS
 * app's LinkStore. What they are (name, system, dates, the pinned host key)
 * is in the app's preferences; each link's own private key is an ECDSA P-256
 * key of Android's keystore, kept in this device's secure hardware, never
 * backed up, and usable only right after the owner's fingerprint, face or
 * screen lock. So a phone picked up unlocked cannot reach a computer without
 * its owner, and the key can sign but never be read.
 */
class LinkStore(context: Context) : LinkKeys {
    private val prefs = context.getSharedPreferences("links", Context.MODE_PRIVATE)
    private val aliases = context.getSharedPreferences("link_keys", Context.MODE_PRIVATE)
    private val secrets = context.getSharedPreferences("link_secrets", Context.MODE_PRIVATE)
    private val keyguard = context.getSystemService(KeyguardManager::class.java)
    private val biometrics = context.getSystemService(BiometricManager::class.java)
    private val format = Json { ignoreUnknownKeys = true }

    var computers: List<LinkedComputer> =
        prefs.getString("links", null)?.let { runCatching { format.decodeFromString(ListSerializer(LinkedComputer.serializer()), it) }.getOrNull() } ?: emptyList()
        private set

    class LinkError(val kind: Kind, val detail: String = "") : Exception(detail) {
        enum class Kind { NO_PASSCODE, KEYSTORE, CANCELLED }
    }

    /** Whether this device has a screen lock, so a link can be protected. */
    val canProtect: Boolean get() = keyguard.isDeviceSecure

    /** Whether a fingerprint or face is set up (else the screen lock alone opens the links). */
    val hasBiometrics: Boolean get() = biometrics.canAuthenticate(BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS

    /**
     * Makes a new link key for a claim: an ECDSA P-256 key in the keystore,
     * usable only after the owner's authentication. The public half goes to
     * the computer; the key is remembered under the device id once it returns.
     */
    override fun make(): MadeKey {
        if (!canProtect) throw LinkError(LinkError.Kind.NO_PASSCODE)
        try {
            val alias = "desk-link-" + Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(12).also { SecureRandom().nextBytes(it) })
            val gen = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
            gen.initialize(
                KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setUserAuthenticationRequired(true)
                    .setUserAuthenticationParameters(AUTH_WINDOW_S, KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL)
                    .setInvalidatedByBiometricEnrollment(false)
                    .build(),
            )
            val pair = gen.generateKeyPair()
            val pub = LinkAuth.publicKeyString(pair.public as ECPublicKey)
            return MadeKey(pub, EcSigner(pair.private), persist = { deviceId -> aliases.edit().putString(deviceId, alias).apply() })
        } catch (e: Exception) {
            throw LinkError(LinkError.Kind.KEYSTORE, e.javaClass.simpleName)
        }
    }

    /** Reach uses a signer unlocked ahead of the session, not this. */
    override fun signer(linkId: String): LinkSigner? = null

    /** Keeps a link's name, system and pinned host key (its key is already in the keystore). */
    fun save(link: DeviceLink, name: String, os: String) {
        val now = System.currentTimeMillis()
        computers = listOf(LinkedComputer(link.deviceId, link.linkId, name, os, now, now, link.hostKey)) + computers.filter { it.deviceId != link.deviceId }
        persist()
    }

    /** Keeps (or forgets, with null) the computer's unattended password, sealed by a keystore key. */
    fun setPassword(link: DeviceLink, password: String?) {
        if (password == null) {
            secrets.edit().remove(link.deviceId).apply()
            return
        }
        runCatching {
            val cipher = Cipher.getInstance(AES)
            cipher.init(Cipher.ENCRYPT_MODE, passwordKey())
            val sealed = cipher.doFinal(password.toByteArray(Charsets.UTF_8))
            val b64 = Base64.getEncoder()
            secrets.edit().putString(link.deviceId, b64.encodeToString(cipher.iv) + "." + b64.encodeToString(sealed)).apply()
        }
    }

    /**
     * The link of a computer, after the fingerprint, face or screen lock (the
     * system's own prompt): its pinned host key, a signer that can sign once
     * with its keystore key, and the saved password.
     */
    suspend fun unlock(activity: Activity, c: LinkedComputer, title: String, subtitle: String): Triple<DeviceLink, String?, LinkSigner> {
        val alias = aliases.getString(c.deviceId, null) ?: throw LinkError(LinkError.Kind.KEYSTORE, "missing")
        val key = try {
            keyStore().getKey(alias, null) as PrivateKey
        } catch (e: Exception) {
            throw LinkError(LinkError.Kind.KEYSTORE, e.javaClass.simpleName)
        }
        val ok = suspendCancellableCoroutine { cont ->
            val cancel = CancellationSignal()
            cont.invokeOnCancellation { cancel.cancel() }
            BiometricPrompt.Builder(activity)
                .setTitle(title)
                .setSubtitle(subtitle)
                .setAllowedAuthenticators(BIOMETRIC_STRONG or DEVICE_CREDENTIAL)
                .build()
                .authenticate(
                    cancel,
                    activity.mainExecutor,
                    object : BiometricPrompt.AuthenticationCallback() {
                        override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                            if (cont.isActive) cont.resume(true)
                        }

                        override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                            if (cont.isActive) cont.resume(false)
                        }
                    },
                )
        }
        if (!ok) throw LinkError(LinkError.Kind.CANCELLED)
        // Authenticated now; the keystore key is usable for a short window.
        return Triple(DeviceLink(c.deviceId, c.linkId, c.hostKey), readPassword(c.deviceId), EcSigner(key))
    }

    private fun readPassword(deviceId: String): String? {
        val parts = secrets.getString(deviceId, null)?.split(".")?.takeIf { it.size == 2 } ?: return null
        return runCatching {
            val b64 = Base64.getDecoder()
            val cipher = Cipher.getInstance(AES)
            cipher.init(Cipher.DECRYPT_MODE, passwordKey(), GCMParameterSpec(128, b64.decode(parts[0])))
            String(cipher.doFinal(b64.decode(parts[1])), Charsets.UTF_8)
        }.getOrNull()
    }

    fun update(deviceId: String, change: (LinkedComputer) -> LinkedComputer) {
        computers = computers.map { if (it.deviceId == deviceId) change(it) else it }
        persist()
    }

    /** Forgets a computer here: its keystore key and sealed password go too. */
    fun forget(deviceId: String) {
        aliases.getString(deviceId, null)?.let { runCatching { keyStore().deleteEntry(it) } }
        aliases.edit().remove(deviceId).apply()
        secrets.edit().remove(deviceId).apply()
        computers = computers.filter { it.deviceId != deviceId }
        persist()
    }

    private fun persist() {
        prefs.edit().putString("links", format.encodeToString(ListSerializer(LinkedComputer.serializer()), computers)).apply()
    }

    private fun keyStore() = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    /** A keystore AES key that seals saved passwords, behind the same lock. */
    private fun passwordKey(): SecretKey {
        val ks = keyStore()
        (ks.getKey(PW_ALIAS, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(PW_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setUserAuthenticationRequired(true)
                .setUserAuthenticationParameters(AUTH_WINDOW_S, KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL)
                .setInvalidatedByBiometricEnrollment(false)
                .build(),
        )
        return gen.generateKey()
    }

    /** Signs a link's proof once with its keystore key, right after the owner's authentication. */
    private class EcSigner(private val key: PrivateKey) : LinkSigner {
        override fun sign(message: ByteArray): String {
            val s = Signature.getInstance("SHA256withECDSA")
            s.initSign(key)
            s.update(message)
            return LinkAuth.rawSignature(s.sign())
        }
    }

    companion object {
        private const val PW_ALIAS = "desk-link-passwords"
        private const val AES = "AES/GCM/NoPadding"
        private const val AUTH_WINDOW_S = 15
    }
}
