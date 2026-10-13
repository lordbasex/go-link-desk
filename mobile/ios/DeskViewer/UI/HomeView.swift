// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI

/** Home: the other computer's code, and the recent computers. */
struct HomeView: View {
    @EnvironmentObject var app: AppModel
    @State private var code = ""
    @State private var settings = false
    @State private var linking = false
    @FocusState private var focused: Bool

    private var ready: Bool { Recents.digits(code) != nil }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 28) {
                    header
                    connectCard
                    computers
                    if !app.recents.isEmpty { recents }
                    Text("Ask the person at the other computer for the 9-digit code go-link Desk shows there. They accept the session on their screen.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                .padding(20)
                .frame(maxWidth: 560)
                .frame(maxWidth: .infinity)
            }
            .scrollDismissesKeyboard(.interactively)
            .background(Theme.background.ignoresSafeArea())
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { settings = true } label: { Image(systemName: "gearshape") }
                        .accessibilityLabel(Text("Settings"))
                }
            }
            .sheet(isPresented: $settings) { SettingsView().environmentObject(app) }
            .sheet(isPresented: $linking) { LinkSheet().environmentObject(app) }
            .alert(String(localized: "Cannot open it"), isPresented: Binding(get: { app.linkError != nil }, set: { if !$0 { app.linkError = nil } })) {
                Button("OK") { app.linkError = nil }
            } message: {
                Text(verbatim: app.linkError ?? "")
            }
        }
    }

    private var header: some View {
        HStack(spacing: 12) {
            Image("Logo")
                .resizable()
                .frame(width: 44, height: 44)
                .clipShape(RoundedRectangle(cornerRadius: 10))
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: "go-link Desk").font(.title2.weight(.semibold))
                Text("Remote desktop").font(.subheadline).foregroundStyle(.secondary)
            }
        }
        .padding(.top, 8)
    }

    private var connectCard: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Connect to another computer").font(.headline)
            TextField("", text: $code, prompt: Text(verbatim: "000 000 000"))
                .font(.system(size: 34, weight: .semibold, design: .monospaced))
                .multilineTextAlignment(.center)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .focused($focused)
                .padding(.vertical, 12)
                .background(Theme.field, in: RoundedRectangle(cornerRadius: 14))
                .onChange(of: code) { _, new in
                    // Grouped as it is typed or pasted: "915 355 636".
                    let grouped = CodeInput.group(String(new.filter { ("0"..."9").contains($0) }.prefix(CodeInput.digits)))
                    if grouped != new { code = grouped }
                }
                .accessibilityLabel(Text("Its 9-digit code"))
            Button {
                focused = false
                app.connect(code: code)
            } label: {
                Text("Connect").font(.headline).frame(maxWidth: .infinity).padding(.vertical, 6)
            }
            .buttonStyle(.borderedProminent)
            .tint(Theme.accent)
            .disabled(!ready)
            // No other computer at hand: go-link Desk's demo, always on.
            Button {
                focused = false
                app.tryDemo()
            } label: {
                HStack(spacing: 8) {
                    if app.demoLoading { ProgressView() } else { Image(systemName: "play.circle") }
                    Text("Try the demo")
                }
                .font(.subheadline.weight(.semibold)).frame(maxWidth: .infinity).padding(.vertical, 4)
            }
            .buttonStyle(.bordered)
            .tint(Theme.accent)
            .disabled(app.demoLoading || app.session != nil)
            .accessibilityHint(Text("Connects to a demo computer with Minesweeper"))
        }
        .padding(18)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: 20))
    }

    /** My computers: the linked ones, opened with Face ID without anyone accepting. */
    private var computers: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("My computers").font(.headline)
                Spacer()
                Button { linking = true } label: { Label("Link", systemImage: "plus") }.font(.subheadline)
            }
            if app.links.computers.isEmpty {
                Text(verbatim: String(format: String(localized: "Link your own computers once with their pairing code, and open them later with %@, without anyone there."), LinkStore.biometryName))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Theme.card, in: RoundedRectangle(cornerRadius: 16))
            } else {
                VStack(spacing: 0) {
                    ForEach(app.links.computers) { c in
                        Button { app.reach(c) } label: { ComputerRow(computer: c) }
                            .buttonStyle(.plain)
                            .contextMenu {
                                Button(role: .destructive) { app.forgetLink(c.deviceId) } label: {
                                    Label("Forget this computer", systemImage: "trash")
                                }
                            }
                        if c.id != app.links.computers.last?.id { Divider().padding(.leading, 56) }
                    }
                }
                .background(Theme.card, in: RoundedRectangle(cornerRadius: 16))
            }
        }
    }

    private var recents: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Recent sessions").font(.headline)
                Spacer()
                Button("Clear") { app.clearRecents() }.font(.subheadline)
            }
            VStack(spacing: 0) {
                ForEach(app.recents) { r in
                    Button { app.connect(code: r.code) } label: { RecentRow(recent: r) }
                        .buttonStyle(.plain)
                        .contextMenu {
                            Button(role: .destructive) { app.forget(r.code) } label: { Label("Remove from the list", systemImage: "trash") }
                        }
                    if r.id != app.recents.last?.id { Divider().padding(.leading, 56) }
                }
            }
            .background(Theme.card, in: RoundedRectangle(cornerRadius: 16))
        }
    }
}

private struct RecentRow: View {
    let recent: Recent

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: icon)
                .font(.title3)
                .frame(width: 32, height: 32)
                .foregroundStyle(Theme.accent)
            VStack(alignment: .leading, spacing: 2) {
                Text(recent.name.isEmpty ? CodeInput.group(recent.code) : recent.name).font(.body.weight(.medium))
                Text(verbatim: "\(CodeInput.group(recent.code)) · \(Recents.systemName(recent.os)) · \(recent.at.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Image(systemName: "chevron.right").font(.footnote).foregroundStyle(.tertiary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
    }

    private var icon: String {
        switch recent.os {
        case "darwin": return "desktopcomputer"
        case "windows": return "pc"
        default: return "display"
        }
    }
}

private struct ComputerRow: View {
    let computer: LinkedComputer

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: computer.os == "windows" ? "pc" : computer.os == "darwin" ? "desktopcomputer" : "display")
                .font(.title3)
                .frame(width: 32, height: 32)
                .foregroundStyle(Theme.accent)
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: computer.name.isEmpty ? String(localized: "Linked computer") : computer.name).font(.body.weight(.medium))
                Text(verbatim: [Recents.systemName(computer.os), computer.lastUsed?.formatted(date: .abbreviated, time: .shortened) ?? ""]
                    .filter { !$0.isEmpty }.joined(separator: " · "))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Image(systemName: LinkStore.biometrySymbol).foregroundStyle(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
    }
}

/** Link a computer: its pairing code (Settings › Unattended access there), and the person there accepts once. */
private struct LinkSheet: View {
    @EnvironmentObject var app: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("", text: $code, prompt: Text(verbatim: "000 000 000"))
                        .font(.system(size: 28, weight: .semibold, design: .monospaced))
                        .multilineTextAlignment(.center)
                        .keyboardType(.numberPad)
                        .focused($focused)
                        .onChange(of: code) { _, new in
                            let grouped = CodeInput.group(String(new.filter { ("0"..."9").contains($0) }.prefix(CodeInput.digits)))
                            if grouped != new { code = grouped }
                        }
                } header: {
                    Text("The computer's pairing code")
                } footer: {
                    Text(verbatim: String(format: String(localized: "On the computer: go-link Desk › Settings › Unattended access, turned on. The person there accepts once; from then on this device opens it with %@, without anyone accepting. Link only your own computers."), LinkStore.biometryName))
                }
                Section {
                    Button {
                        dismiss()
                        app.link(code: code)
                    } label: {
                        Label("Link and connect", systemImage: "link").frame(maxWidth: .infinity)
                    }
                    .disabled(Recents.digits(code) == nil)
                }
            }
            .navigationTitle("Link a computer")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } } }
            .onAppear { focused = true }
        }
    }
}
