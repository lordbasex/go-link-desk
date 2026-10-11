// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI

/** go-link Desk for iPhone and iPad: connect to another computer with its code, and see and use it. */
@main
struct DeskApp: App {
    @StateObject private var app = AppModel()

    var body: some Scene {
        WindowGroup {
            Group {
                if let s = app.session {
                    SessionView(model: s)
                } else {
                    HomeView()
                }
            }
            .environmentObject(app)
            .tint(Theme.accent)
            .onAppear(perform: debugLaunch)
        }
    }

    /** Debug builds only: -code 915355636 connects at once, -signal ws://127.0.0.1:8391/ws picks a local server. */
    private func debugLaunch() {
        #if DEBUG
        let args = UserDefaults.standard
        if let s = args.string(forKey: "signal"), !s.isEmpty, CommandLine.arguments.contains("-signal") { app.customSignal = s }
        if let c = args.string(forKey: "code"), CommandLine.arguments.contains("-code") { app.connect(code: c) }
        #endif
    }
}

/** The app's colors: go-link Desk's amber on a dark ground, like the Mac app. */
enum Theme {
    static let accent = Color(red: 0xF2 / 255, green: 0xA3 / 255, blue: 0x3A / 255) // --color-accent
    static let background = Color(uiColor: .systemGroupedBackground)
    static let card = Color(uiColor: .secondarySystemGroupedBackground)
    static let field = Color(uiColor: .tertiarySystemFill)
}
