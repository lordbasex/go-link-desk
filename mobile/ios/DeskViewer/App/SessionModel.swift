// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import AudioToolbox
import DeskCore
import OSLog
import SwiftUI
import UIKit
@preconcurrency import WebRTC

/**
 * One session as the screens see it: DeskSession with its own signalhub
 * connection and libwebrtc peer, the picture's track, what the computer
 * says (its name, displays, pointer) and the numbers of the details panel.
 */
@MainActor
final class SessionModel: ObservableObject, MediaListener {
    /** The session's steps and numbers in the unified log (never a code or a name). */
    static let log = Logger(subsystem: "org.go-link.desk.ios", category: "session")

    let door: Door
    /** The session code, for a session by code ("" otherwise). */
    var code: String {
        if case let .join(c) = door { return c }
        return ""
    }
    /** What the toolbar shows before the computer says its name. */
    let label: String
    private unowned let app: AppModel
    private let peers = DeskPeers()
    private(set) var session: DeskSession!

    @Published private(set) var status: SessionStatus = .joining
    @Published private(set) var hostName = ""
    @Published private(set) var hostOS = ""
    @Published private(set) var permissions = Permissions()
    @Published private(set) var displays: [Display] = []
    @Published private(set) var currentDisplay = 0
    @Published private(set) var video: RTCVideoTrack?
    @Published private(set) var stats = LiveStats()
    @Published private(set) var notice = ""
    @Published var showStats = false
    @Published var keyboard = false
    @Published var muted = false
    /** The computer's files, once its channel opens (with the files permission). */
    @Published private(set) var files: FilesModel?
    @Published var showFiles = false
    @Published var showGuide = false
    /** The chat: its lines, how many came while it was closed, and the last one as a toast. */
    @Published private(set) var chat: [ChatLine] = []
    @Published private(set) var unread = 0
    @Published var showChat = false {
        didSet { if showChat { unread = 0; toast = nil } }
    }
    @Published var toast: String?
    private var toastTimer: Timer?
    /** The person at the computer is writing a message. */
    @Published private(set) var hostTyping = false
    private var hostTypingTimer: Timer?
    /** This side writing: "typing" again every few seconds, "stopped" after a pause. */
    private var typingSentAt: Date?
    private var typingPause: Timer?
    /** The link made with the pairing code, saved again when the computer says its name. */
    private var pendingLink: DeviceLink?
    /** Linked in this session, to say so. */
    @Published var justLinked = false
    /** Locked to watch: the screen sends nothing until it is unlocked. */
    @Published var locked = false {
        didSet { if locked { keyboard = false } }
    }
    /** The computer's pointer image and where it is (0..1), when it moves by itself. */
    @Published private(set) var cursor: CursorShape?
    private(set) var hostPointer: (x: Double, y: Double, visible: Bool)?
    var onHostPointer: ((Double, Double, Bool) -> Void)?

    private var audio: RTCAudioTrack?
    private var meter = StatsMeter()
    private var statsTimer: Timer?
    private var hidden = false
    private var lifecycle: [NSObjectProtocol] = []

    init(app: AppModel, door: Door, label: String) {
        self.app = app
        self.door = door
        self.label = label
    }

    /** password: the computer's unattended password kept with its link, sent by itself when asked. */
    func start(password: String? = nil) {
        peers.media = self
        let signal = SignalClient(url: app.signalURL, factory: URLSessionSockets(), scheduler: MainScheduler.shared)
        let s = DeskSession(door: door, name: app.knockName, ua: app.agent, signal: signal, peers: peers, scheduler: MainScheduler.shared)
        session = s
        let ev = s.events
        ev.status = { [weak self] st in self?.statusChanged(st) }
        ev.host = { [weak self] name, os in
            guard let self else { return }
            self.hostName = name
            self.hostOS = os
            switch self.door {
            case let .join(code):
                self.app.remember(Recent(code: code, name: name, os: os, at: Date()))
            case let .reach(link):
                self.app.links.update(link.deviceId) { $0.name = name; $0.os = os; $0.lastUsed = Date() }
                self.app.objectWillChange.send()
            case .claim:
                if let link = self.pendingLink { self.app.link(link, name: name, os: os) }
            }
        }
        ev.permissions = { [weak self] p in self?.permissions = p }
        ev.displays = { [weak self] d, current in
            self?.displays = d
            self?.currentDisplay = current
        }
        ev.notice = { [weak self] code in self?.notice = code }
        ev.cursor = { [weak self] shape in self?.cursor = shape }
        ev.cursorPos = { [weak self] x, y, visible in
            self?.hostPointer = (x, y, visible)
            self?.onHostPointer?(x, y, visible)
        }
        ev.clipboard = { text in UIPasteboard.general.string = text }
        ev.chat = { [weak self] text in self?.received(text) }
        ev.typing = { [weak self] on in self?.setHostTyping(on) }
        ev.linked = { [weak self] link in
            guard let self else { return }
            self.pendingLink = link
            self.app.link(link, name: self.hostName, os: self.hostOS)
        }
        s.savedPassword = password
        ev.passwordAccepted = { [weak self] pw in
            guard let self, case let .reach(link) = self.door else { return }
            self.app.links.setPassword(link, pw)
        }
        ev.passwordRejected = { [weak self] in
            guard let self, case let .reach(link) = self.door else { return }
            self.app.links.setPassword(link, nil)
        }
        ev.unlinked = { [weak self] in
            guard let self, case let .reach(link) = self.door else { return }
            self.app.forgetLink(link.deviceId)
        }
        // The picture is sent smaller while the app is in the background.
        let nc = NotificationCenter.default
        lifecycle = [
            nc.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.hidden = true }
            },
            nc.addObserver(forName: UIApplication.willEnterForegroundNotification, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.hidden = false }
            },
        ]
        s.start()
    }

    private func statusChanged(_ st: SessionStatus) {
        status = st
        SessionModel.log.info("status \(String(describing: st), privacy: .public)")
        switch st {
        case .connecting, .live:
            if statsTimer == nil {
                session.setQuality(app.preset)
                // The pointer comes as an image and a position, drawn here at once.
                session.setCursorInVideo(false)
                statsTimer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
                    MainActor.assumeIsolated { self?.tick() }
                }
            }
        default:
            break
        }
        if st.isOver {
            statsTimer?.invalidate()
            statsTimer = nil
            UIApplication.shared.isIdleTimerDisabled = false
        } else if st == .live {
            // The screen stays on while the computer is shown.
            UIApplication.shared.isIdleTimerDisabled = true
            // The first time, how to use it.
            if !UserDefaults.standard.bool(forKey: "guide_seen") {
                UserDefaults.standard.set(true, forKey: "guide_seen")
                showGuide = true
            }
        }
    }

    private func tick() {
        peers.current?.stats { [weak self] report in
            guard let self else { return }
            self.stats = self.meter.next(StatsReader.sample(report))
            let st = self.stats
            SessionModel.log.info("stats fps=\(st.fps, format: .fixed(precision: 1)) size=\(st.width)x\(st.height) kbps=\(st.kbps) loss=\(st.lossPct, format: .fixed(precision: 1)) rtt=\(st.rttMs ?? -1) path=\(st.path ?? "-", privacy: .public) video=\(self.video != nil)")
            self.session.report(lossPct: self.stats.lossPct, hidden: self.hidden)
        }
    }

    private func setHostTyping(_ on: Bool) {
        hostTypingTimer?.invalidate()
        hostTypingTimer = nil
        hostTyping = on
        guard on else { return }
        // Without news for a while, it stopped.
        hostTypingTimer = Timer.scheduledTimer(withTimeInterval: 6, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated { self?.hostTyping = false }
        }
    }

    /** The chat's line changed: tell the computer this side is writing, or stopped. */
    func wrote(_ text: String) {
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            quiet()
            return
        }
        if typingSentAt.map({ Date().timeIntervalSince($0) >= 3 }) ?? true {
            typingSentAt = Date()
            session.sendTyping(true)
        }
        typingPause?.invalidate()
        typingPause = Timer.scheduledTimer(withTimeInterval: 4, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated { self?.quiet() }
        }
    }

    func quiet() {
        typingPause?.invalidate()
        typingPause = nil
        if typingSentAt != nil {
            typingSentAt = nil
            session.sendTyping(false)
        }
    }

    private func received(_ text: String) {
        setHostTyping(false) // the message it was writing came
        chat.append(ChatLine(fromHost: true, text: text, at: Date()))
        // The message sound (Settings › Message sound): the system's "received" chime.
        if app.chatSound { AudioServicesPlaySystemSound(1003) }
        if chat.count > 500 { chat.removeFirst(chat.count - 500) }
        guard !showChat else { return }
        unread += 1
        toast = text
        toastTimer?.invalidate()
        toastTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated { self?.toast = nil }
        }
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    /** Sends a chat line; false when there is nothing to send or the session is over. */
    @discardableResult
    func say(_ text: String) -> Bool {
        guard session.sendChat(text) else { return false }
        quiet()
        chat.append(ChatLine(fromHost: false, text: text.trimmingCharacters(in: .whitespacesAndNewlines), at: Date()))
        return true
    }

    func hangUp() {
        session.hangUp()
    }

    /** Back to Home. */
    func close() {
        session.hangUp()
        for o in lifecycle { NotificationCenter.default.removeObserver(o) }
        lifecycle = []
        app.session = nil
    }

    func setPreset(_ p: Preset) {
        app.preset = p
        session.setQuality(p)
    }

    func selectDisplay(_ id: Int) {
        currentDisplay = id
        session.selectDisplay(id)
    }

    func setMuted(_ on: Bool) {
        muted = on
        audio?.isEnabled = !on
    }

    /** Sends this device's clipboard to the computer and pastes it there. */
    func pasteHere() {
        if let text = UIPasteboard.general.string, !text.isEmpty {
            session.sendClipboard(text, paste: true)
        }
    }

    var touchMode: Bool {
        get { app.touchMode }
        set { app.touchMode = newValue; objectWillChange.send() }
    }

    var preset: Preset { app.preset }

    // MARK: - MediaListener

    func onVideo(_ track: RTCVideoTrack) {
        SessionModel.log.info("video track")
        video = track
    }

    func onAudio(_ track: RTCAudioTrack) {
        audio = track
        track.isEnabled = !muted
    }

    func onFiles(_ client: FilesClient) {
        files = FilesModel(client: client)
    }

    func onMediaClosed() {
        video = nil
        audio = nil
        files = nil
        showFiles = false
    }
}

/** One line of the session's chat. */
struct ChatLine: Identifiable, Equatable {
    let id = UUID()
    let fromHost: Bool
    let text: String
    let at: Date
}
