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
                        if model.hostTyping {
                            TypingBubble(name: model.hostName.isEmpty ? String(localized: "The computer") : model.hostName)
                                .id("typing")
                                .transition(.opacity)
                        }
                    }
                    .padding(12)
                }
                .onChange(of: model.chat.count) {
                    if let last = model.chat.last { withAnimation { scroll.scrollTo(last.id, anchor: .bottom) } }
                }
                .onChange(of: model.hostTyping) {
                    if model.hostTyping { withAnimation { scroll.scrollTo("typing", anchor: .bottom) } }
                }
                .animation(.easeInOut(duration: 0.2), value: model.hostTyping)
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
                        .onChange(of: text) { _, new in model.wrote(new) }
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
            .onDisappear { model.quiet() }
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

/** "Name is typing" with three dots that rise in turn, like a messaging app. */
private struct TypingBubble: View {
    let name: String

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                TimelineView(.animation) { t in
                    let time = t.date.timeIntervalSinceReferenceDate
                    HStack(spacing: 4) {
                        ForEach(0..<3) { i in
                            let wave = sin((time * 2 * .pi / 1.2) - Double(i) * 0.6)
                            Circle()
                                .fill(Color.secondary)
                                .frame(width: 7, height: 7)
                                .opacity(0.4 + 0.6 * max(0, wave))
                                .offset(y: -3 * max(0, wave))
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 11)
                    .background(Theme.card, in: RoundedRectangle(cornerRadius: 16))
                }
                Text(verbatim: String(format: String(localized: "%@ is typing…"), name))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
            Spacer(minLength: 40)
        }
    }
}
