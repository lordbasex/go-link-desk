// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import SwiftUI

/** The session's chat with the person at the computer. */
struct ChatView: View {
    @ObservedObject var model: SessionModel
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            ScrollViewReader { scroll in
                ScrollView {
                    LazyVStack(spacing: 8) {
                        if model.chat.isEmpty {
                            Text("Write to the person at the computer. They see it in a window on their screen.")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                                .padding(.top, 40)
                                .padding(.horizontal, 24)
                        }
                        ForEach(model.chat) { line in
                            Bubble(line: line, host: model.hostName).id(line.id)
                        }
                    }
                    .padding(12)
                }
                .onChange(of: model.chat.count) {
                    if let last = model.chat.last { withAnimation { scroll.scrollTo(last.id, anchor: .bottom) } }
                }
                .onAppear {
                    if let last = model.chat.last { scroll.scrollTo(last.id, anchor: .bottom) }
                }
            }
            .safeAreaInset(edge: .bottom) {
                HStack(spacing: 8) {
                    TextField("Message", text: $text, axis: .vertical)
                        .lineLimit(1...5)
                        .focused($focused)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .background(Theme.field, in: RoundedRectangle(cornerRadius: 18))
                        .onSubmit(send)
                    Button(action: send) {
                        Image(systemName: "arrow.up.circle.fill").font(.system(size: 30))
                    }
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    .accessibilityLabel(Text("Send"))
                }
                .padding(10)
                .background(.bar)
            }
            .navigationTitle("Chat")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
            .onAppear { focused = true }
        }
    }

    private func send() {
        if model.say(text) { text = "" }
    }
}

private struct Bubble: View {
    let line: ChatLine
    let host: String

    var body: some View {
        HStack {
            if !line.fromHost { Spacer(minLength: 40) }
            VStack(alignment: line.fromHost ? .leading : .trailing, spacing: 2) {
                Text(line.text)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .foregroundStyle(line.fromHost ? Color.primary : Color.black)
                    .background(line.fromHost ? Theme.card : Theme.accent, in: RoundedRectangle(cornerRadius: 16))
                    .textSelection(.enabled)
                Text(verbatim: (line.fromHost && !host.isEmpty ? host + " · " : "") + line.at.formatted(date: .omitted, time: .shortened))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            if line.fromHost { Spacer(minLength: 40) }
        }
    }
}
