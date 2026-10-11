// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI
import UIKit

/**
 * What the app keeps between launches (UserDefaults, this app only): the
 * name the person at the computer sees, the recent computers, the
 * signaling server, and how the screen is handled.
 */
@MainActor
final class AppModel: ObservableObject {
    private let defaults = UserDefaults.standard

    @Published var name: String { didSet { defaults.set(name, forKey: "name") } }
    @Published private(set) var recents: [Recent]
    /** A custom signaling server ("" for the official one). */
    @Published var customSignal: String { didSet { defaults.set(customSignal, forKey: "signal") } }
    /** Trackpad (the default) or touch: how a finger moves the pointer. */
    @Published var touchMode: Bool { didSet { defaults.set(touchMode, forKey: "touch_mode") } }
    @Published var preset: Preset { didSet { defaults.set(preset.rawValue, forKey: "preset") } }
    @Published var session: SessionModel?
    let links = LinkStore()
    /** Why the last link or unlock failed, for Home. */
    @Published var linkError: String?

    init() {
        name = defaults.string(forKey: "name") ?? ""
        customSignal = defaults.string(forKey: "signal") ?? ""
        touchMode = defaults.bool(forKey: "touch_mode")
        preset = Preset(rawValue: defaults.string(forKey: "preset") ?? "") ?? .balanced
        if let data = defaults.data(forKey: "recents"), let list = try? JSONDecoder().decode([Recent].self, from: data) {
            recents = list
        } else {
            recents = []
        }
    }

    /** The server in use: a valid custom one, else the official one. */
    var signalURL: String {
        SignalUrls.resolve(customSignal.isEmpty ? nil : customSignal, allowDevHosts: AppModel.isDebug).url
    }

    /** The name sent with the knock: the one typed, else the device's. */
    var knockName: String {
        let n = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return n.isEmpty ? UIDevice.current.name : n
    }

    /** "go-link Desk 1.0 on iPadOS 27.0": what the person at the computer sees as the browser. */
    var agent: String {
        let v = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"
        let device = UIDevice.current
        let system = device.userInterfaceIdiom == .pad ? "iPadOS" : "iOS"
        return "go-link Desk \(v) on \(system) \(device.systemVersion)"
    }

    var version: String {
        let v = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"
        let b = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "0"
        return "v\(v) (\(b))" + (AppModel.isDebug ? " · debug" : "")
    }

    func connect(code: String) {
        guard let digits = Recents.digits(code) else { return }
        open(.join(digits), label: CodeInput.group(digits))
    }

    /** Links this device to a computer with its pairing code (Settings › Unattended access there). */
    func link(code: String) {
        guard LinkStore.canProtect else {
            linkError = LinkStore.LinkError.noPasscode.errorDescription
            return
        }
        guard let digits = Recents.digits(code) else { return }
        open(.claim(digits), label: CodeInput.group(digits))
    }

    /** Comes back to a linked computer: Face ID (or the passcode) first. */
    func reach(_ c: LinkedComputer) {
        guard session == nil else { return }
        Task {
            do {
                let link = try await links.unlock(c)
                open(.reach(link), label: c.name)
            } catch let e as LinkStore.LinkError {
                if case .cancelled = e { return }
                linkError = e.errorDescription
            } catch {
                linkError = error.localizedDescription
            }
        }
    }

    private func open(_ door: Door, label: String) {
        guard session == nil else { return }
        let s = SessionModel(app: self, door: door, label: label)
        session = s
        s.start()
    }

    /** Keeps a link made with the pairing code (again when the computer says its name). */
    func link(_ link: DeviceLink, name: String, os: String) {
        do {
            try links.save(link, name: name, os: os)
            session?.justLinked = true
        } catch {
            linkError = error.localizedDescription
        }
        objectWillChange.send()
    }

    func forgetLink(_ deviceId: String) {
        links.forget(deviceId)
        objectWillChange.send()
    }

    func remember(_ r: Recent) {
        recents = Recents.add(r, to: recents)
        save()
    }

    func forget(_ code: String) {
        recents.removeAll { $0.code == code }
        save()
    }

    func clearRecents() {
        recents = []
        save()
    }

    private func save() {
        if let data = try? JSONEncoder().encode(recents) { defaults.set(data, forKey: "recents") }
    }

    static var isDebug: Bool {
        #if DEBUG
        return true
        #else
        return false
        #endif
    }
}
