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
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.spec.MGF1ParameterSpec
import java.security.spec.X509EncodedKeySpec
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.OAEPParameterSpec
import javax.crypto.spec.PSource
import javax.crypto.spec.SecretKeySpec
import kotlin.coroutines.resume

/** A linked computer as My computers shows it. Its token is not here: it is sealed apart. Dates in ms since 1970. */
@Serializable
data class LinkedComputer(
    val deviceId: String,
    val linkId: String,
    val name: String,
    val os: String,
    val linkedAt: Long,
    val lastUsed: Long? = null,
    /** The computer proved itself bound to the connection once (DeviceLink.bound). */
    val bound: Boolean = false,
)

/**
 * The computers this device is linked to, the Android counterpart of the
 * iOS app's LinkStore. What they are (name, system, dates) is in the app's
 * preferences; each link's token (and the computer's unattended password)
 * is sealed with a key of Android's keystore, kept in this device's secure
 * hardware and never backed up, that opens only after the owner's
 * fingerprint, face or screen lock. So a phone picked up unlocked cannot
 * reach a computer without its owner.
 *
 * Sealing needs no prompt (the key's public half seals a fresh AES key that
 * seals the secret); opening asks for the fingerprint, face or screen lock
 * (the private half is usable only right after it).
 */
class LinkStore(context: Context) {
    private val prefs = context.getSharedPreferences("links", Context.MODE_PRIVATE)
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

    /** What the keystore keeps for a link: its token, and the computer's unattended password once typed. */
    @Serializable
    private data class Secret(val token: String, val password: String? = null)

    /** Keeps a new link: its token sealed for the owner's fingerprint, face or screen lock. */
    fun save(link: DeviceLink, name: String, os: String) {
        store(link, null)
        val now = System.currentTimeMillis()
        computers = listOf(LinkedComputer(link.deviceId, link.linkId, name, os, now, now)) + computers.filter { it.deviceId != link.deviceId }
        persist()
    }

    /** Keeps (or forgets, with null) the computer's unattended password with its link, sealed too. */
    fun setPassword(link: DeviceLink, password: String?) {
        runCatching { store(link, password) }
    }

    private fun store(link: DeviceLink, password: String?) {
        if (!canProtect) throw LinkError(LinkError.Kind.NO_PASSCODE)
        try {
            val plain = format.encodeToString(Secret.serializer(), Secret(link.token, password)).toByteArray(Charsets.UTF_8)
            // A fresh AES key seals the secret; the keystore's public key seals that key.
            val aes = ByteArray(32).also { SecureRandom().nextBytes(it) }
            val iv = ByteArray(12).also { SecureRandom().nextBytes(it) }
            val gcm = Cipher.getInstance("AES/GCM/NoPadding")
            gcm.init(Cipher.ENCRYPT_MODE, SecretKeySpec(aes, "AES"), GCMParameterSpec(128, iv))
            val sealed = gcm.doFinal(plain)
            val rsa = Cipher.getInstance(RSA)
            rsa.init(Cipher.ENCRYPT_MODE, publicKey(), OAEP)
            val wrapped = rsa.doFinal(aes)
            val b64 = Base64.getEncoder()
            secrets.edit().putString(link.deviceId, listOf(wrapped, iv, sealed).joinToString(".") { b64.encodeToString(it) }).apply()
        } catch (e: LinkError) {
            throw e
        } catch (e: Exception) {
            throw LinkError(LinkError.Kind.KEYSTORE, e.javaClass.simpleName)
        }
    }

    /** The link of a computer, after the fingerprint, face or screen lock (the system's own prompt). */
    suspend fun unlock(activity: Activity, c: LinkedComputer, title: String, subtitle: String): Pair<DeviceLink, String?> {
        val parts = secrets.getString(c.deviceId, null)?.split(".")?.takeIf { it.size == 3 }
            ?: throw LinkError(LinkError.Kind.KEYSTORE, "missing")
        val b64 = Base64.getDecoder()
        val (wrapped, iv, sealed) = parts.map { b64.decode(it) }
        val rsa = try {
            Cipher.getInstance(RSA).apply { init(Cipher.DECRYPT_MODE, privateKey(), OAEP) }
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
                    BiometricPrompt.CryptoObject(rsa),
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
        try {
            val aes = rsa.doFinal(wrapped)
            val gcm = Cipher.getInstance("AES/GCM/NoPadding")
            gcm.init(Cipher.DECRYPT_MODE, SecretKeySpec(aes, "AES"), GCMParameterSpec(128, iv))
            val secret = format.decodeFromString(Secret.serializer(), String(gcm.doFinal(sealed), Charsets.UTF_8))
            if (secret.token.isEmpty()) throw LinkError(LinkError.Kind.KEYSTORE, "empty")
            return DeviceLink(c.deviceId, c.linkId, secret.token, c.bound) to secret.password
        } catch (e: LinkError) {
            throw e
        } catch (e: Exception) {
            throw LinkError(LinkError.Kind.KEYSTORE, e.javaClass.simpleName)
        }
    }

    fun update(deviceId: String, change: (LinkedComputer) -> LinkedComputer) {
        computers = computers.map { if (it.deviceId == deviceId) change(it) else it }
        persist()
    }

    /** Forgets a computer here: its sealed token goes too. */
    fun forget(deviceId: String) {
        secrets.edit().remove(deviceId).apply()
        computers = computers.filter { it.deviceId != deviceId }
        persist()
    }

    private fun persist() {
        prefs.edit().putString("links", format.encodeToString(ListSerializer(LinkedComputer.serializer()), computers)).apply()
    }

    // The keystore's key pair: made once; its private half needs the owner each time.

    private fun keyStore() = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    private fun ensureKey() {
        val ks = keyStore()
        if (ks.containsAlias(ALIAS)) return
        val gen = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_RSA, "AndroidKeyStore")
        gen.initialize(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setKeySize(2048)
                .setDigests(KeyProperties.DIGEST_SHA256, KeyProperties.DIGEST_SHA1)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_RSA_OAEP)
                .setUserAuthenticationRequired(true)
                .setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL)
                .setInvalidatedByBiometricEnrollment(false)
                .build(),
        )
        gen.generateKeyPair()
    }

    /** The public half as a plain key: sealing with it needs no prompt. */
    private fun publicKey(): java.security.PublicKey {
        ensureKey()
        val pub = keyStore().getCertificate(ALIAS).publicKey
        return KeyFactory.getInstance(pub.algorithm).generatePublic(X509EncodedKeySpec(pub.encoded))
    }

    private fun privateKey(): PrivateKey {
        ensureKey()
        return keyStore().getKey(ALIAS, null) as PrivateKey
    }

    companion object {
        private const val ALIAS = "desk-links"
        private const val RSA = "RSA/ECB/OAEPWithSHA-256AndMGF1Padding"

        /** OAEP with SHA-256, and MGF1 with SHA-1 (what the keystore's RSA supports). */
        private val OAEP = OAEPParameterSpec("SHA-256", "MGF1", MGF1ParameterSpec.SHA1, PSource.PSpecified.DEFAULT)
    }
}
