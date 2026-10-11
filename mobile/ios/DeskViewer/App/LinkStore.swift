// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import Foundation
import LocalAuthentication
import Security

/** A linked computer as My computers shows it. Its token is not here: it is in the keychain. */
struct LinkedComputer: Codable, Equatable, Identifiable {
    var deviceId: String
    var linkId: String
    var name: String
    var os: String
    var linkedAt: Date
    var lastUsed: Date?

    var id: String { deviceId }
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
    static var canProtect: Bool {
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

    /** Keeps a new link: its token in the keychain behind the owner's face or passcode. */
    func save(_ link: DeviceLink, name: String, os: String) throws {
        guard LinkStore.canProtect else { throw LinkError.noPasscode }
        var error: Unmanaged<CFError>?
        guard let access = SecAccessControlCreateWithFlags(nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .userPresence, &error) else {
            throw LinkError.keychain(errSecParam)
        }
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: LinkStore.service,
            kSecAttrAccount as String: link.deviceId,
        ]
        SecItemDelete(base as CFDictionary)
        var add = base
        add[kSecValueData as String] = Data(link.token.utf8)
        add[kSecAttrAccessControl as String] = access
        let status = SecItemAdd(add as CFDictionary, nil)
        guard status == errSecSuccess else { throw LinkError.keychain(status) }
        computers.removeAll { $0.deviceId == link.deviceId }
        computers.insert(LinkedComputer(deviceId: link.deviceId, linkId: link.linkId, name: name, os: os, linkedAt: Date(), lastUsed: Date()), at: 0)
        persist()
    }

    /**
     * The link of a computer, after Face ID (or the passcode). Runs the
     * keychain read off the main thread: the system shows its own prompt.
     */
    func unlock(_ c: LinkedComputer) async throws -> DeviceLink {
        let context = LAContext()
        context.localizedReason = String(format: String(localized: "Connect to %@"), c.name.isEmpty ? String(localized: "the computer") : c.name)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: LinkStore.service,
            kSecAttrAccount as String: c.deviceId,
            kSecReturnData as String: true,
            kSecUseAuthenticationContext as String: context,
        ]
        let result: (OSStatus, Data?) = await Task.detached {
            var out: CFTypeRef?
            let s = SecItemCopyMatching(query as CFDictionary, &out)
            return (s, out as? Data)
        }.value
        switch result.0 {
        case errSecSuccess:
            guard let data = result.1, let token = String(data: data, encoding: .utf8) else { throw LinkError.keychain(errSecDecode) }
            return DeviceLink(deviceId: c.deviceId, linkId: c.linkId, token: token)
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

    /** Forgets a computer here: its token goes from the keychain. */
    func forget(_ deviceId: String) {
        SecItemDelete([
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: LinkStore.service,
            kSecAttrAccount as String: deviceId,
        ] as CFDictionary)
        computers.removeAll { $0.deviceId == deviceId }
        persist()
    }

    private func persist() {
        if let data = try? JSONEncoder().encode(computers) { defaults.set(data, forKey: "links") }
    }
}
