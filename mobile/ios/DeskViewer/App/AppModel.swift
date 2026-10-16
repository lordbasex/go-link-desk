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
    /** A sound when a chat message comes (on by default). */
    @Published var chatSound: Bool { didSet { defaults.set(chatSound, forKey: "chat_sound") } }
    @Published var session: SessionModel?
    let links = LinkStore()
    /** Why the last link or unlock failed, for Home. */
    @Published var linkError: String?
    /** The demo computer's code is being read (Try the demo). */
    @Published private(set) var demoLoading = false

    init() {
        name = defaults.string(forKey: "name") ?? ""
        customSignal = defaults.string(forKey: "signal") ?? ""
        touchMode = defaults.bool(forKey: "touch_mode")
        preset = Preset(rawValue: defaults.string(forKey: "preset") ?? "") ?? .balanced
        chatSound = defaults.object(forKey: "chat_sound") as? Bool ?? true
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

    /**
     * Try the demo: connects to go-link Desk's demonstration computer, whose
     * current code is at desk.go-link.org/demo.json (it changes whenever
     * that computer reconnects). It accepts at once and shows a small
     * desktop with Minesweeper.
     */
    func tryDemo() {
        guard session == nil, !demoLoading else { return }
        demoLoading = true
        Task {
            defer { demoLoading = false }
            do {
                let code = try await DemoCode.fetch()
                connect(code: code)
            } catch {
                linkError = String(localized: "The demo computer is not available right now. Try again in a minute.")
            }
        }
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
                let (link, password, signer) = try await links.unlock(c)
                open(.reach(link), label: c.name, password: password, signer: signer)
            } catch let e as LinkStore.LinkError {
                if case .cancelled = e { return }
                linkError = e.errorDescription
            } catch {
                linkError = error.localizedDescription
            }
        }
    }

    private func open(_ door: Door, label: String, password: String? = nil, signer: LinkSigner? = nil) {
        guard session == nil else { return }
        let s = SessionModel(app: self, door: door, label: label)
        session = s
        s.start(password: password, signer: signer)
    }

    /** Keeps a link made with the pairing code (again when the computer says its name). */
    func link(_ link: DeviceLink, name: String, os: String) {
        links.save(link, name: name, os: os)
        session?.justLinked = true
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

/** The demonstration computer's current code (desk.go-link.org/demo). */
enum DemoCode {
    static let url = URL(string: "https://desk.go-link.org/demo.json")!

    private struct File: Decodable {
        let online: Bool
        let code: String?
    }

    static func fetch() async throws -> String {
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 10)
        req.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard (resp as? HTTPURLResponse)?.statusCode == 200 else { throw URLError(.badServerResponse) }
        let f = try JSONDecoder().decode(File.self, from: data)
        guard f.online, let code = f.code, Recents.digits(code) != nil else { throw URLError(.resourceUnavailable) }
        return code
    }
}
