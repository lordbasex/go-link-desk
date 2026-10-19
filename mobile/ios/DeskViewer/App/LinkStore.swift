// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import CryptoKit
import DeskCore
import Foundation
import LocalAuthentication
import Security

/** A linked computer as My computers shows it. The link's key is not here: it is in the keychain. */
struct LinkedComputer: Codable, Equatable, Identifiable {
    var deviceId: String
    var linkId: String
    var name: String
    var os: String
    var linkedAt: Date
    var lastUsed: Date?
    /** The computer's device public key, pinned when the link was made. */
    var hostKey: String

    var id: String { deviceId }
}

/**
 * A link's private key: in the Secure Enclave when the device has one (so it
 * can sign but never be read), else a software key. Either signs with ECDSA
 * P-256 and gives its public half as the 65-byte point.
 */
enum StoredKey: @unchecked Sendable {
    case enclave(SecureEnclave.P256.Signing.PrivateKey)
    case software(P256.Signing.PrivateKey)

    var publicKey: String {
        switch self {
        case let .enclave(k): return LinkAuth.base64url(k.publicKey.x963Representation)
        case let .software(k): return LinkAuth.base64url(k.publicKey.x963Representation)
        }
    }

    /** The persistable form: the enclave's encrypted blob, or the raw key. */
    var dataRepresentation: Data {
        switch self {
        case let .enclave(k): return k.dataRepresentation
        case let .software(k): return k.rawRepresentation
        }
    }

    func sign(_ message: Data) throws -> String {
        switch self {
        case let .enclave(k): return LinkAuth.base64url(try k.signature(for: message).rawRepresentation)
        case let .software(k): return LinkAuth.base64url(try k.signature(for: message).rawRepresentation)
        }
    }

    /** Makes a fresh key, in the enclave when there is one. */
    static func make(_ access: SecAccessControl) throws -> StoredKey {
        if SecureEnclave.isAvailable {
            return .enclave(try SecureEnclave.P256.Signing.PrivateKey(accessControl: access))
        }
        return .software(P256.Signing.PrivateKey())
    }

    /** Reads a key back from its stored form, under the owner's authentication. */
    static func load(_ data: Data, context: LAContext) throws -> StoredKey {
        if SecureEnclave.isAvailable, let k = try? SecureEnclave.P256.Signing.PrivateKey(dataRepresentation: data, authenticationContext: context) {
            return .enclave(k)
        }
        return .software(try P256.Signing.PrivateKey(rawRepresentation: data))
    }
}

/** Signs a link's proof with its stored key. */
struct StoredLinkSigner: LinkSigner {
    let key: StoredKey
    func sign(_ message: Data) throws -> String { try key.sign(message) }
}

/**
 * The computers this device is linked to. What they are (name, system,
 * dates) is in UserDefaults; each link's token is in the keychain, only on
 * this device (never in a backup or iCloud), and given back only after
 * Face ID, Touch ID or Optic ID, or the device's passcode. So a phone
 * picked up unlocked cannot reach a computer without its owner.
 */
@MainActor
final class LinkStore {
    private static let service = "org.go-link.desk.ios.links"
    private static let passwordService = "org.go-link.desk.ios.link-passwords"
    private let defaults = UserDefaults.standard

    private(set) var computers: [LinkedComputer] = []

    init() {
        if let data = defaults.data(forKey: "links"), let list = try? JSONDecoder().decode([LinkedComputer].self, from: data) {
            computers = list
        }
    }

    enum LinkError: LocalizedError {
        case noPasscode
        case keychain(OSStatus)
        case cancelled

        var errorDescription: String? {
            switch self {
            case .noPasscode: return String(localized: "Set a passcode on this device first: linked computers are protected with it and with Face ID.")
            case let .keychain(s): return String(localized: "The keychain refused it.") + " (\(s))"
            case .cancelled: return String(localized: "Cancelled.")
            }
        }
    }

    /** Whether this device has a passcode, so a link can be protected. */
    nonisolated static var canProtect: Bool {
        LAContext().canEvaluatePolicy(.deviceOwnerAuthentication, error: nil)
    }

    /** "Face ID", "Touch ID" or "Optic ID", or the passcode when there is no biometry. */
    static var biometryName: String {
        let c = LAContext()
        _ = c.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
        switch c.biometryType {
        case .faceID: return "Face ID"
        case .touchID: return "Touch ID"
        case .opticID: return "Optic ID"
        default: return String(localized: "the passcode")
        }
    }

    static var biometrySymbol: String {
        let c = LAContext()
        _ = c.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: nil)
        switch c.biometryType {
        case .faceID: return "faceid"
        case .touchID: return "touchid"
        case .opticID: return "opticid"
        default: return "lock"
        }
    }

    /** What the keychain keeps for a link: its private key, and the computer's unattended password once typed. */
    private struct Secret: Codable {
        var keyData: Data
        var password: String?
    }

    /**
     * Makes a new link key for a claim: it is generated now (in the Secure
     * Enclave when there is one) and kept in the keychain under the device id
     * once the computer returns it. The public half goes to the computer.
     */
    nonisolated func make() throws -> MadeKey {
        guard LinkStore.canProtect else { throw LinkError.noPasscode }
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .userPresence, nil) else {
            throw LinkError.keychain(errSecParam)
        }
        let key = try StoredKey.make(access)
        let data = key.dataRepresentation
        return MadeKey(publicKey: key.publicKey, signer: StoredLinkSigner(key: key), persist: { account in
            try LinkStore.storeSecret(account: account, Secret(keyData: data, password: nil))
        })
    }

    /** Keeps a link's name, system and pinned host key (its key is already in the keychain). */
    func save(_ link: DeviceLink, name: String, os: String) {
        computers.removeAll { $0.deviceId == link.deviceId }
        computers.insert(LinkedComputer(deviceId: link.deviceId, linkId: link.linkId, name: name, os: os, linkedAt: Date(), lastUsed: Date(), hostKey: link.hostKey), at: 0)
        persist()
    }

    /** Keeps (or forgets, with nil) the computer's unattended password alongside its key. */
    func setPassword(_ link: DeviceLink, _ password: String?) {
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .userPresence, nil) else { return }
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: LinkStore.passwordService,
            kSecAttrAccount as String: link.deviceId,
        ]
        SecItemDelete(base as CFDictionary)
        guard let password else { return }
        var add = base
        add[kSecValueData as String] = Data(password.utf8)
        add[kSecAttrAccessControl as String] = access
        SecItemAdd(add as CFDictionary, nil)
    }

    nonisolated private static func storeSecret(account: String, _ secret: Secret) throws {
        guard canProtect else { throw LinkError.noPasscode }
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .userPresence, nil) else {
            throw LinkError.keychain(errSecParam)
        }
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(base as CFDictionary)
        var add = base
        add[kSecValueData as String] = try JSONEncoder().encode(secret)
        add[kSecAttrAccessControl as String] = access
        let status = SecItemAdd(add as CFDictionary, nil)
        guard status == errSecSuccess else { throw LinkError.keychain(status) }
    }

    /**
     * The link of a computer, after Face ID (or the passcode): its pinned
     * host key, a signer backed by its keychain key, and the saved password.
     * The keychain read runs off the main thread; the system shows its own
     * prompt.
     */
    func unlock(_ c: LinkedComputer) async throws -> (DeviceLink, String?, LinkSigner) {
        let context = LAContext()
        context.localizedReason = String(format: String(localized: "Connect to %@"), c.name.isEmpty ? String(localized: "the computer") : c.name)
        let service = LinkStore.service, pwService = LinkStore.passwordService
        let result: (OSStatus, Data?, Data?) = await Task.detached {
            func read(_ svc: String) -> Data? {
                var out: CFTypeRef?
                let q: [String: Any] = [
                    kSecClass as String: kSecClassGenericPassword,
                    kSecAttrService as String: svc,
                    kSecAttrAccount as String: c.deviceId,
                    kSecReturnData as String: true,
                    kSecUseAuthenticationContext as String: context,
                ]
                return SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess ? out as? Data : nil
            }
            var out: CFTypeRef?
            let s = SecItemCopyMatching([
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: c.deviceId,
                kSecReturnData as String: true,
                kSecUseAuthenticationContext as String: context,
            ] as CFDictionary, &out)
            return (s, out as? Data, read(pwService))
        }.value
        switch result.0 {
        case errSecSuccess:
            guard let data = result.1, let secret = try? JSONDecoder().decode(Secret.self, from: data) else {
                throw LinkError.keychain(errSecDecode)
            }
            let key = try StoredKey.load(secret.keyData, context: context)
            let password = secret.password ?? result.2.flatMap { String(data: $0, encoding: .utf8) }
            return (DeviceLink(deviceId: c.deviceId, linkId: c.linkId, hostKey: c.hostKey), password, StoredLinkSigner(key: key))
        case errSecUserCanceled, errSecAuthFailed:
            throw LinkError.cancelled
        default:
            throw LinkError.keychain(result.0)
        }
    }

    func update(_ deviceId: String, _ change: (inout LinkedComputer) -> Void) {
        guard let i = computers.firstIndex(where: { $0.deviceId == deviceId }) else { return }
        change(&computers[i])
        persist()
    }

    /** Forgets a computer here: its key and saved password go from the keychain. */
    func forget(_ deviceId: String) {
        for svc in [LinkStore.service, LinkStore.passwordService] {
            SecItemDelete([
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: svc,
                kSecAttrAccount as String: deviceId,
            ] as CFDictionary)
        }
        computers.removeAll { $0.deviceId == deviceId }
        persist()
    }

    private func persist() {
        if let data = try? JSONEncoder().encode(computers) { defaults.set(data, forKey: "links") }
    }
}
