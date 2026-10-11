// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import SwiftUI

/**
 * How to use the computer from here: the fingers (trackpad and touch),
 * the keyboard and its keys, the toolbar, an iPad's trackpad and mouse, and
 * the lock. It opens by itself the first time a session starts, and
 * from the toolbar's ? button.
 */
struct GuideView: View {
    @Environment(\.dismiss) private var dismiss
    let touchMode: Bool

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(touchMode ? String(localized: "You are in touch mode: the pointer goes where you touch.") : String(localized: "You are in trackpad mode: the screen works like a laptop's trackpad."))
                        .font(.subheadline).foregroundStyle(.secondary)
                }
                Section("Trackpad mode") {
                    Gesture("hand.draw", "One finger, slide", "Moves the pointer. Slow for precision, quick to go far.")
                    Gesture("hand.tap", "One finger, tap", "Click. Two quick taps: double click.")
                    Gesture("hand.point.up.left", "One finger, hold then slide", "Drag: select text, move a window or a file.")
                    Gesture("hand.tap.fill", "Two fingers, tap", "Right click (the menu).", fingers: 2)
                    Gesture("arrow.up.left.and.arrow.down.right", "Two fingers, pinch", "Zoom in and out. Zoomed in, slide two fingers to move around.", fingers: 2)
                    Gesture("arrow.up.and.down", "Three fingers, slide", "Scroll up, down and sideways.", fingers: 3)
                }
                Section("Touch mode") {
                    Gesture("hand.point.up", "Touch", "The pointer goes there and clicks.")
                    Gesture("hand.point.up.left", "Hold then slide", "Drag.")
                    Gesture("hand.tap.fill", "Two fingers, tap", "Right click.", fingers: 2)
                    Gesture("arrow.up.and.down", "Two fingers, slide", "Scroll.", fingers: 2)
                }
                Section("Keyboard") {
                    Gesture("keyboard", "The keyboard button", "Opens this device's keyboard: what you type is written on the computer.")
                    Gesture("command", "The bar over the keyboard", "Esc, Tab, the arrows, Delete and F1 to F12. Ctrl, Alt (⌥) and Win (⌘) stay pressed for the next key: Ctrl, then C, is Ctrl+C.")
                    Gesture("keyboard.badge.ellipsis", "A keyboard connected to the iPad", "Its keys work as on the computer, shortcuts included.")
                }
                Section("The toolbar") {
                    Gesture("command", "Special keys", "Shortcuts of the computer (switch apps, Ctrl+Alt+Del, Spotlight…) and Paste this device's clipboard there.")
                    Gesture("bubble.left.and.bubble.right", "Chat", "Write to the person at the computer; a number shows messages you have not read.")
                    Gesture("folder", "Files", "Browse the computer's files: tap one to download it to this device, or send one of yours.")
                    Gesture("display", "Screen", "The computer's displays, the picture's quality, trackpad or touch, the sound and the details.")
                    Gesture("lock", "Lock", "To watch without touching anything: the screen stops sending. Hold the lock for a second to unlock.")
                    Gesture("chevron.up", "Hide", "Hides the toolbar; the arrow brings it back.")
                }
                Section("An iPad's trackpad or mouse") {
                    Gesture("cursorarrow", "Move and click", "The pointer follows it; the buttons click (the secondary one is right click) and scrolling scrolls.")
                }
            }
            .navigationTitle("How to use it")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
        }
    }
}

/** One gesture: its symbol (and how many fingers), what to do and what it does. */
private struct Gesture: View {
    let symbol: String
    let title: LocalizedStringKey
    let text: LocalizedStringKey
    let fingers: Int

    init(_ symbol: String, _ title: LocalizedStringKey, _ text: LocalizedStringKey, fingers: Int = 0) {
        self.symbol = symbol
        self.title = title
        self.text = text
        self.fingers = fingers
    }

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            ZStack(alignment: .bottomTrailing) {
                Image(systemName: symbol)
                    .font(.title2)
                    .foregroundStyle(Theme.accent)
                    .frame(width: 40, height: 40)
                if fingers > 0 {
                    Text(verbatim: "\(fingers)")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.black)
                        .frame(width: 16, height: 16)
                        .background(Theme.accent, in: Circle())
                }
            }
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.body.weight(.medium))
                Text(text).font(.subheadline).foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}
