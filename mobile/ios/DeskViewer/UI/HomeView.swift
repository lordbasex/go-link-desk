// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI

/** Home: the other computer's code, and the recent computers. */
struct HomeView: View {
    @EnvironmentObject var app: AppModel
    @State private var code = ""
    @State private var settings = false
    @FocusState private var focused: Bool

    private var ready: Bool { Recents.digits(code) != nil }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 28) {
                    header
                    connectCard
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
        }
        .padding(18)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: 20))
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
