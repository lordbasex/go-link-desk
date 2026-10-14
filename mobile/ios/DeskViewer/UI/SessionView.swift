// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI

/** A session: the computer's screen, a small toolbar over it, and what the session is doing. */
struct SessionView: View {
    @ObservedObject var model: SessionModel
    @State private var barHidden = false
    /** On a phone held upright the toolbar has no room for everything: the name goes, lock and help move to the screen menu. */
    @Environment(\.horizontalSizeClass) private var width
    private var narrow: Bool { width == .compact }

    var body: some View {
        ZStack(alignment: .top) {
            Color.black.ignoresSafeArea()
            if model.status == .live || model.status == .reconnecting || (model.status == .connecting && model.video != nil) {
                RemoteScreen(model: model)
                    .ignoresSafeArea()
            }
            if case let .password(prompt) = model.status {
                PasswordCard(prompt: prompt, name: model.label) { model.session.sendPassword($0) } cancel: { model.close() }
            } else if let text = statusText {
                StatusCard(text: text, busy: !model.status.isOver, over: model.status.isOver) { model.close() } cancel: { model.close() }
            }
            if model.justLinked && model.status == .live {
                Label(String(format: String(localized: "Linked: next time, it opens with %@, without anyone accepting."), LinkStore.biometryName), systemImage: LinkStore.biometrySymbol)
                    .font(.footnote)
                    .foregroundStyle(.white)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(.black.opacity(0.75), in: Capsule())
                    .padding(.top, 56)
                    .onTapGesture { model.justLinked = false }
                    .task { try? await Task.sleep(for: .seconds(6)); model.justLinked = false }
            }
            if !model.status.isOver && model.status != .joining && model.status != .waiting {
                if model.locked { lockBadge } else { toolbar }
            }
            if let since = model.recordingSince, !model.status.isOver {
                Button { model.toggleRecording() } label: {
                    TimelineView(.periodic(from: since, by: 1)) { ctx in
                        HStack(spacing: 6) {
                            Circle().fill(.red).frame(width: 9, height: 9)
                            Text(verbatim: Self.clock(ctx.date.timeIntervalSince(since))).monospacedDigit()
                        }
                    }
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 7)
                    .background(.red.opacity(0.35), in: Capsule())
                }
                .accessibilityLabel(Text("Stop recording"))
                .padding(.top, 56)
            }
            if let note = model.captureNote {
                Text(verbatim: note)
                    .font(.footnote)
                    .foregroundStyle(.white)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(.black.opacity(0.75), in: Capsule())
                    .padding(.top, 96)
                    .padding(.horizontal, 16)
            }
            if model.showStats, model.status == .live {
                StatsPanel(stats: model.stats, sent: nil)
                    .padding(.top, 64)
            }
        }
        .sheet(isPresented: $model.showGuide) { GuideView(touchMode: model.touchMode) }
        .sheet(isPresented: $model.showChat) { ChatView(model: model) }
        .overlay(alignment: .top) {
            if let t = model.toast, !model.showChat {
                Button { model.showChat = true } label: {
                    HStack(spacing: 8) {
                        Image(systemName: "bubble.left.fill").foregroundStyle(Theme.accent)
                        Text(verbatim: t).lineLimit(2).foregroundStyle(.white)
                    }
                    .font(.footnote)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(.black.opacity(0.75), in: Capsule())
                }
                .padding(.top, 56)
                .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
        .animation(.easeInOut, value: model.toast)
        .sheet(isPresented: $model.showFiles) {
            if let f = model.files { FilesView(model: f) }
        }
        .statusBarHidden(true)
        .persistentSystemOverlays(.hidden)
        .preferredColorScheme(.dark)
    }

    private var statusText: String? {
        switch model.status {
        case .joining: return String(localized: "Connecting…")
        case .waiting: return String(localized: "Waiting for the person at the computer to accept…")
        case .connecting: return model.video == nil ? String(localized: "Accepted. Connecting…") : nil
        case .live: return nil
        case .reconnecting: return String(localized: "The connection dropped. Reconnecting…")
        case let .rejected(r):
            switch r {
            case "timeout": return String(localized: "Nobody answered at the computer.")
            case "busy": return String(localized: "The computer is in another session.")
            case "off", "unavailable": return String(localized: "The computer is not taking sessions now.")
            default: return String(localized: "The person at the computer did not accept.")
            }
        case let .ended(r):
            switch r {
            case "viewer": return String(localized: "You ended the session.")
            case "network", "signal": return String(localized: "The connection was lost.")
            case "idle": return String(localized: "The session ended after a while without use.")
            default: return String(localized: "The person at the computer ended the session.")
            }
        case .notFound: return String(localized: "That code does not lead anywhere. It may have changed: ask for the current one.")
        case .rateLimited: return String(localized: "Too many tries. Wait a minute and try again.")
        case .unreachable: return String(localized: "The server does not answer. Check the connection.")
        case .password: return nil
        case .offline: return String(localized: "The computer is not on, or go-link Desk is closed there.")
        case .unlinked: return String(localized: "That computer no longer has this link (it was removed there). It was forgotten here: link it again with its pairing code.")
        case .untrusted: return String(localized: "Whoever answered could not prove it is your linked computer. Nothing was sent to it.")
        case let .passwordRefused(reason, retryAfter):
            return reason == "locked"
                ? String(format: String(localized: "Too many wrong passwords. Try again in %d minutes."), max(1, retryAfter / 60))
                : String(localized: "Too many wrong passwords.")
        }
    }

    private var title: String {
        model.hostName.isEmpty ? model.label : model.hostName
    }

    private var toolbar: some View {
        HStack(spacing: 4) {
            if barHidden {
                Button { withAnimation { barHidden = false } } label: { Image(systemName: "chevron.down") }
                    .accessibilityLabel(Text("Show the toolbar"))
            } else {
                Button(role: .destructive) { model.hangUp() } label: { Image(systemName: "xmark") }
                    .accessibilityLabel(Text("Disconnect"))
                if !narrow {
                    Text(title)
                        .font(.footnote.weight(.medium))
                        .lineLimit(1)
                        .padding(.horizontal, 6)
                    Divider().frame(height: 20)
                }
                if model.permissions.control {
                    Button { model.keyboard.toggle() } label: { Image(systemName: model.keyboard ? "keyboard.chevron.compact.down" : "keyboard") }
                        .accessibilityLabel(Text("Keyboard"))
                    keysMenu
                }
                Button { model.showChat = true } label: {
                    Image(systemName: "bubble.left.and.bubble.right")
                        .overlay(alignment: .topTrailing) {
                            if model.unread > 0 {
                                Text(verbatim: "\(min(model.unread, 99))")
                                    .font(.system(size: 10, weight: .bold))
                                    .foregroundStyle(.white)
                                    .padding(.horizontal, 4)
                                    .frame(minWidth: 16, minHeight: 16)
                                    .background(.red, in: Capsule())
                                    .offset(x: 9, y: -8)
                            }
                        }
                }
                .accessibilityLabel(Text("Chat"))
                if model.permissions.files && model.files != nil {
                    Button { model.showFiles = true } label: { Image(systemName: "folder") }
                        .accessibilityLabel(Text("Files"))
                }
                if model.permissions.record {
                    captureMenu
                }
                screenMenu
                if !narrow {
                    Button { withAnimation { model.locked = true } } label: { Image(systemName: "lock") }
                        .accessibilityLabel(Text("Lock"))
                    Button { model.showGuide = true } label: { Image(systemName: "questionmark.circle") }
                        .accessibilityLabel(Text("How to use it"))
                }
                Button { withAnimation { barHidden = true } } label: { Image(systemName: "chevron.up") }
                    .accessibilityLabel(Text("Hide the toolbar"))
            }
        }
        .buttonStyle(BarButton())
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .background(.ultraThinMaterial, in: Capsule())
        .padding(.top, 6)
    }

    /** Locked: a small lock in a corner; holding it for a second unlocks. */
    private var lockBadge: some View {
        HStack(spacing: 6) {
            Image(systemName: "lock.fill")
            Text("Hold to unlock").font(.caption)
        }
        .foregroundStyle(.white.opacity(0.85))
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(.black.opacity(0.45), in: Capsule())
        .onLongPressGesture(minimumDuration: 1) {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            withAnimation { model.locked = false }
        }
        .accessibilityAddTraits(.isButton)
        .accessibilityLabel(Text("Unlock"))
        .accessibilityAction { model.locked = false }
        .frame(maxWidth: .infinity, alignment: .trailing)
        .padding(.top, 6)
        .padding(.trailing, 12)
    }

    private var keysMenu: some View {
        Menu {
            ForEach(Shortcut.forSystem(model.hostOS), id: \.id) { s in
                Button(shortcutTitle(s.id)) {
                    for p in s.packets() { model.session.input(p) }
                }
            }
            if model.permissions.clipboard {
                Divider()
                Button { model.pasteHere() } label: { Label("Paste this device's clipboard", systemImage: "doc.on.clipboard") }
            }
        } label: {
            Image(systemName: "command")
        }
        .accessibilityLabel(Text("Special keys"))
    }

    private var captureMenu: some View {
        Menu {
            Button { model.takeScreenshot() } label: { Label("Take a screenshot", systemImage: "camera") }
            Button { model.toggleRecording() } label: {
                if model.recordingSince != nil {
                    Label("Stop recording", systemImage: "stop.circle")
                } else {
                    Label("Record the screen", systemImage: "record.circle")
                }
            }
            Section { Text("The person at the computer sees when you take a screenshot or record.") }
        } label: {
            Image(systemName: model.recordingSince != nil ? "record.circle.fill" : "record.circle")
                .foregroundStyle(model.recordingSince != nil ? Color.red : Color.primary)
        }
        .accessibilityLabel(Text("Screenshot and recording"))
    }

    /** A recording's length: "4:05", "1:02:09". */
    static func clock(_ t: TimeInterval) -> String {
        let s = max(0, Int(t))
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60) : String(format: "%d:%02d", s / 60, s % 60)
    }

    private var screenMenu: some View {
        Menu {
            if model.displays.count > 1 {
                Section("Display") {
                    ForEach(model.displays, id: \.id) { d in
                        Button { model.selectDisplay(d.id) } label: {
                            if d.id == model.currentDisplay { Label(d.name, systemImage: "checkmark") } else { Text(d.name) }
                        }
                    }
                }
            }
            Section("Picture") {
                ForEach(Preset.allCases, id: \.self) { p in
                    Button { model.setPreset(p) } label: {
                        if p == model.preset { Label(presetTitle(p), systemImage: "checkmark") } else { Text(presetTitle(p)) }
                    }
                }
            }
            if model.permissions.control {
                Section("Fingers") {
                    Button { model.touchMode = false } label: {
                        if !model.touchMode { Label("Trackpad", systemImage: "checkmark") } else { Text("Trackpad") }
                    }
                    Button { model.touchMode = true } label: {
                        if model.touchMode { Label("Touch", systemImage: "checkmark") } else { Text("Touch") }
                    }
                }
            }
            Section {
                if model.permissions.audio {
                    Button { model.setMuted(!model.muted) } label: {
                        Label(model.muted ? String(localized: "Turn the sound on") : String(localized: "Mute the sound"), systemImage: model.muted ? "speaker.wave.2" : "speaker.slash")
                    }
                }
                if narrow {
                    Button { withAnimation { model.locked = true } } label: { Label("Lock", systemImage: "lock") }
                    Button { model.showGuide = true } label: { Label("How to use it", systemImage: "questionmark.circle") }
                }
                Button { model.showStats.toggle() } label: {
                    Label(model.showStats ? String(localized: "Hide the details") : String(localized: "Show the details"), systemImage: "chart.bar")
                }
            }
        } label: {
            Image(systemName: "display")
        }
        .accessibilityLabel(Text("Screen"))
    }

    private func presetTitle(_ p: Preset) -> String {
        switch p {
        case .speed: return String(localized: "Speed")
        case .balanced: return String(localized: "Balanced")
        case .quality: return String(localized: "Quality")
        }
    }

    private func shortcutTitle(_ id: String) -> String {
        switch id {
        case "copy": return String(localized: "Copy")
        case "paste": return String(localized: "Paste")
        case "undo": return String(localized: "Undo")
        case "select_all": return String(localized: "Select all")
        case "switch_apps": return String(localized: "Switch apps")
        case "spotlight": return String(localized: "Spotlight")
        case "force_quit": return String(localized: "Force quit")
        case "lock": return String(localized: "Lock the screen")
        case "screenshot": return String(localized: "Screenshot")
        case "start": return String(localized: "Start menu")
        case "ctrl_alt_del": return "Ctrl+Alt+Del"
        case "task_manager": return String(localized: "Task manager")
        default: return id
        }
    }
}

private struct BarButton: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 16, weight: .medium))
            .frame(minWidth: 36, minHeight: 36)
            .contentShape(Rectangle())
            .opacity(configuration.isPressed ? 0.5 : 1)
    }
}

/** What the session is doing, before the picture or once it is over. */
private struct StatusCard: View {
    let text: String
    let busy: Bool
    let over: Bool
    let back: () -> Void
    let cancel: () -> Void

    var body: some View {
        VStack(spacing: 18) {
            if busy { ProgressView().controlSize(.large).tint(.white) }
            Text(text)
                .font(.body)
                .multilineTextAlignment(.center)
                .foregroundStyle(.white)
            if over {
                Button("Back", action: back).buttonStyle(.borderedProminent).tint(Theme.accent)
            } else {
                Button("Cancel", role: .cancel, action: cancel).buttonStyle(.bordered).tint(.white)
            }
        }
        .padding(28)
        .frame(maxWidth: 380)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 20))
        .padding()
        .frame(maxHeight: .infinity)
    }
}

/** The details: frames a second, size, bitrate, loss, round trip and path. */
private struct StatsPanel: View {
    let stats: LiveStats
    let sent: Sent?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            row("fps", String(format: "%.0f", stats.fps))
            row(String(localized: "size"), stats.width > 0 ? "\(stats.width)×\(stats.height)" : "–")
            row("kbps", "\(stats.kbps)")
            row(String(localized: "loss"), String(format: "%.1f %%", stats.lossPct))
            row("rtt", stats.rttMs.map { "\($0) ms" } ?? "–")
            row(String(localized: "path"), stats.path.map { $0 == "relay" ? String(localized: "relay") : String(localized: "direct") } ?? "–")
        }
        .font(.caption.monospacedDigit())
        .foregroundStyle(.white)
        .padding(10)
        .background(.black.opacity(0.6), in: RoundedRectangle(cornerRadius: 10))
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.leading, 12)
        .allowsHitTesting(false)
    }

    private func row(_ k: String, _ v: String) -> some View {
        HStack {
            Text(k).foregroundStyle(.white.opacity(0.6)).frame(width: 44, alignment: .leading)
            Text(v)
        }
    }
}

/** The unattended password the linked computer asks for. */
private struct PasswordCard: View {
    let prompt: PasswordPrompt
    let name: String
    let send: (String) -> Void
    let cancel: () -> Void
    @State private var password = ""
    @FocusState private var focused: Bool

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "lock.shield").font(.largeTitle).foregroundStyle(Theme.accent)
            Text("This computer asks for its unattended access password.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.white)
            SecureField("Password", text: $password)
                .textContentType(.password)
                .focused($focused)
                .padding(10)
                .background(.white.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
                .foregroundStyle(.white)
                .onSubmit(go)
            if prompt.wrong {
                Text(verbatim: String(format: String(localized: "Wrong password. %d tries left."), prompt.left)).font(.footnote).foregroundStyle(.orange)
            }
            HStack {
                Button("Cancel", role: .cancel, action: cancel).buttonStyle(.bordered).tint(.white)
                Button(action: go) {
                    if prompt.checking { ProgressView() } else { Text("Unlock") }
                }
                .buttonStyle(.borderedProminent)
                .tint(Theme.accent)
                .disabled(password.isEmpty || prompt.checking)
            }
        }
        .padding(28)
        .frame(maxWidth: 380)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 20))
        .padding()
        .frame(maxHeight: .infinity)
        .onAppear { focused = true }
        .onChange(of: prompt.wrong) { password = "" }
    }

    private func go() {
        guard !password.isEmpty, !prompt.checking else { return }
        send(password)
    }
}
