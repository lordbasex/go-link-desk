// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI

/** Settings: your name, the fingers, the signaling server, and the app's version. */
struct SettingsView: View {
    @EnvironmentObject var app: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var server = ""
    @State private var serverError = ""
    @State private var testing = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField(UIDevice.current.name, text: $app.name)
                        .textInputAutocapitalization(.words)
                } header: {
                    Text("Your name")
                } footer: {
                    Text("Shown to the computer you connect to. Empty: this device's name.")
                }
                Section("Fingers") {
                    Picker("Fingers", selection: $app.touchMode) {
                        Text("Trackpad").tag(false)
                        Text("Touch").tag(true)
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    Text(app.touchMode
                        ? String(localized: "The pointer goes where you touch, and a tap clicks there.")
                        : String(localized: "Slide a finger to move the pointer and tap to click. Long press then slide to drag; two fingers scroll, right click with a tap, and pinch to zoom."))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Section {
                    TextField(DeskProtocol.officialSignalURL, text: $server)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    if !serverError.isEmpty {
                        Text(serverError).font(.footnote).foregroundStyle(.red)
                    }
                    Button(testing ? String(localized: "Checking…") : String(localized: "Save")) { save() }.disabled(testing)
                    if !app.customSignal.isEmpty {
                        Button("Use go-link's server", role: .destructive) {
                            app.customSignal = ""
                            server = ""
                            serverError = ""
                        }
                    }
                } header: {
                    Text("Signaling server")
                } footer: {
                    Text("Empty: go-link's. Your own signalhub server starts with wss://; it is checked before it is saved.")
                }
                Section {
                    LabeledContent("Version", value: app.version)
                    Link(String("desk.go-link.org"), destination: URL(string: "https://desk.go-link.org")!)
                } footer: {
                    Text("Free and open source software (MIT license). The picture, sound and controls go straight between the devices, encrypted end to end.")
                }
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
            .onAppear { server = app.customSignal }
        }
    }

    private func save() {
        let text = server.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.isEmpty {
            app.customSignal = ""
            serverError = ""
            return
        }
        guard case let .ok(url) = SignalUrls.check(text, allowDevHosts: AppModel.isDebug) else {
            serverError = String(localized: "That is not a server address: it starts with wss://.")
            return
        }
        testing = true
        Task {
            let problem = await SignalClient.test(url: url, factory: URLSessionSockets(), scheduler: MainScheduler.shared)
            testing = false
            if problem == nil {
                app.customSignal = url
                server = url
                serverError = ""
            } else {
                serverError = String(localized: "That server did not answer.")
            }
        }
    }
}
