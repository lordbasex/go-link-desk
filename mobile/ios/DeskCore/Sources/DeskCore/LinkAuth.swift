// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import CryptoKit
import Foundation

/**
 * Asymmetric link authentication, the viewer's half. Each link has an ECDSA
 * P-256 key; its private half is kept non-extractable (the app's Secure
 * Enclave), so it can sign but never be read. The viewer also pins the
 * computer's device public key and checks every proof the computer signs
 * with it.
 *
 * A public key is the 65-byte uncompressed point (x9.63), a signature is the
 * 64-byte P1363 pair (R||S, CryptoKit's rawRepresentation), both base64url.
 * Signing a message hashes it with SHA-256 (ES256), the same as the host and
 * the web and Android viewers.
 */

/** Signs a link's proof. The private key never leaves the signer. */
public protocol LinkSigner: Sendable {
    /** Signs a message; returns the 64-byte R||S, base64url. */
    func sign(_ message: Data) throws -> String
}

/** A key just made for a new link (claim), and how to keep it under its id. */
public struct MadeKey: Sendable {
    public let publicKey: String
    public let signer: LinkSigner
    /** Stores the key under the link id the computer returned (auth_ok). */
    public let persist: @Sendable (String) throws -> Void

    public init(publicKey: String, signer: LinkSigner, persist: @escaping @Sendable (String) throws -> Void) {
        self.publicKey = publicKey
        self.signer = signer
        self.persist = persist
    }
}

/** Where the viewer makes and finds its link keys. */
public protocol LinkKeys: Sendable {
    /** Makes a fresh link key (claim). */
    func make() throws -> MadeKey
    /** The signer for a link coming back (reach), or nil if it is gone. */
    func signer(linkId: String) -> LinkSigner?
}

/** A viewer with no key store: a code-only session that never links. */
public struct NoLinkKeys: LinkKeys {
    public init() {}
    public func make() throws -> MadeKey { throw LinkAuthError.noKeys }
    public func signer(linkId: String) -> LinkSigner? { nil }
}

public enum LinkAuthError: Error { case noKeys }

public enum LinkAuth {
    public static func hostProofMessage(nonce: String, host: String, viewer: String) -> Data {
        Data("go-link-desk host proof v1\n\(nonce)\n\(host)\n\(viewer)".utf8)
    }

    public static func linkProofMessage(nonce: String, host: String, viewer: String) -> Data {
        Data("go-link-desk link proof v1\n\(nonce)\n\(host)\n\(viewer)".utf8)
    }

    public static func hostUnlinkMessage(nonce: String, host: String, viewer: String) -> Data {
        Data("go-link-desk host unlink v1\n\(nonce)\n\(host)\n\(viewer)".utf8)
    }

    /** Verifies sig (64-byte R||S, base64url) against a public key. Never throws. */
    public static func verify(publicKey pub: String, message: Data, sig: String) -> Bool {
        guard let pubData = fromBase64url(pub), pubData.count == 65, pubData.first == 4,
            let sigData = fromBase64url(sig), sigData.count == 64,
            let key = try? P256.Signing.PublicKey(x963Representation: pubData),
            let signature = try? P256.Signing.ECDSASignature(rawRepresentation: sigData)
        else {
            return false
        }
        return key.isValidSignature(signature, for: message)
    }

    static func base64url(_ data: Data) -> String {
        data.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }

    static func fromBase64url(_ s: String) -> Data? {
        var b = s.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        while b.count % 4 != 0 { b += "=" }
        return Data(base64Encoded: b)
    }
}

/**
 * A link key held in memory as a plain CryptoKit key. The app uses the
 * Secure Enclave instead (non-extractable); this backs the portable tests
 * and any place without an enclave.
 */
public struct SoftwareLinkKey: LinkSigner {
    let key: P256.Signing.PrivateKey

    public init(_ key: P256.Signing.PrivateKey = P256.Signing.PrivateKey()) {
        self.key = key
    }

    public var publicKey: String { LinkAuth.base64url(key.publicKey.x963Representation) }

    public func sign(_ message: Data) throws -> String {
        LinkAuth.base64url(try key.signature(for: message).rawRepresentation)
    }
}
