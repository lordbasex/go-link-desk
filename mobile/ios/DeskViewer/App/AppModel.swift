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
        guard session == nil, let digits = Recents.digits(code) else { return }
        let s = SessionModel(app: self, code: digits)
        session = s
        s.start()
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
