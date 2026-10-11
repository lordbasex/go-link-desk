// swift-tools-version:5.10
// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import PackageDescription

// The portable logic of go-link Desk for iOS: the signaling client, the
// session (knock, offer, control messages), the input packets and keys,
// the pointer and zoom. No UIKit and no WebRTC here, so it builds and
// tests on the Mac with `swift test`.
let package = Package(
    name: "DeskCore",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "DeskCore", targets: ["DeskCore"]),
    ],
    targets: [
        .target(name: "DeskCore"),
        .testTarget(name: "DeskCoreTests", dependencies: ["DeskCore"]),
    ]
)
