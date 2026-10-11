# Changelog

All notable changes to go-link Desk. Newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added (screen, phase 2)

- **The real screen on macOS:** ScreenCaptureKit hands each changed frame to the system's H.264 encoder (VideoToolbox, on the hardware) without a copy, at up to 60 frames per second. Nothing is captured while nobody watches, and a still screen sends nothing but a few sharpening frames.
- **Presets** Speed, Balanced and Quality, all up to 60 frames per second, and **Original resolution (Retina)**. The picture follows the viewer's window; Quality sends the display's own pixels.
- **Several displays:** the viewer picks one and the picture switches without reconnecting.
- **The pointer** travels as its shape and position and the browser draws it, sharp at any size; or, as an option, it is drawn into the picture.
- **Bandwidth estimation** (Google Congestion Control, TWCC): the encoder follows what the path carries, and a short bandwidth lowers the frame rate and the size. Lost packets are resent.
- **Latency measured from end to end** with a beacon: on an Intel Mac on the LAN, 30 to 47 ms from the host's screen to the viewer's at 1080p and 58 frames per second.
- **Permissions:** the host's window shows Screen Recording and Accessibility, with a button that asks for them. Without Screen Recording, the viewer sees the test card and is told why.
- **On the website:** display, quality, view (Fit, Original, Stretch), Retina and remote-cursor options; session details with the target bitrate, the estimated bandwidth, resent packets and the screen latency, with a Measure button.
- **H.264 tuning:** Constrained High with the low-latency rate control (5 dB sharper text than Baseline on an M1 for a tenth more bits), falling back without it where the encoder refuses it.
- **Headless hosts on macOS** serve the main thread's event loop, which ScreenCaptureKit needs.
- **Measurement builds** (`GO_TAGS=desk_test_screen`) for testing the real screen without a person to accept; never a release.

### Added (sessions)

- **A session from end to end, with the test card:** the viewer types the code, says its name and knocks; the person at the computer gets a connection request (the declared name and browser, what the viewer will be able to do, 60 seconds to answer, and the first time a warning about phone scams) and accepts or rejects. Only an accepted viewer gets a WebRTC offer: the video (VP8) and a `control` channel with the host's name, round trips, what the host sends and what the browser receives.
- **Always visible on the host:** while a session lasts, the tray icon turns red and gets a Disconnect item, and a small panel floats above every window with who is connected, since when, what they can do and Disconnect.
- **Every way a session ends:** Disconnect on either side, the viewer leaving (signalhub's `peer_left`), the host leaving, or the network. The viewer is told why.
- **One session at a time:** a second viewer gets "busy". A host without a window turns every viewer down (nobody can accept).
- **ICE restart:** when the connection drops, the host offers again with new credentials, so a session survives a network change.
- **Each viewer gets its own track and encoder,** so a keyframe one browser needs costs nobody else anything.
- **On the website,** `/c/<code>` asks for your name (remembered), waits for the answer, and shows the remote screen filling the window (fitted, never cropped, drawn smooth), with session details (latency, path, frames sent, received and decoded, size, bitrate, loss, codec), full screen and Disconnect. In English, Spanish and Portuguese.
- **`--test-auto-accept`** accepts every viewer without asking, for the end-to-end tests only: the host refuses it unless `--source testpattern`.
- **End-to-end tests** with a real Chromium: a viewer gets in and decodes frames, a second one is turned away as busy, leaving ends the session and frees the host, Disconnect ends it, and a host with nobody to ask turns the viewer down.

### Added (host)

- **The host registers with signalhub and shows a session code:** `cmd/desk` registers as `app: "go-link-desk"`, opens an invite-only room and asks for its invitation; the 9-digit code and the `/c/<invite>` link show in a minimal window (Fyne, with a tray icon, in English, Spanish and Portuguese) or, with `--headless`, in the log. "New code" replaces the invitation and the old code stops working. The room and its code are opened again after every reconnect.
- **The generic pieces of go-link's device,** adapted where they named games: the signalhub client with reconnection, pairing and linked browsers (the remembered-browser proof now signs `go-link-desk link proof`, so a proof never passes between the two apps), the PIN gate, status, CPU and RAM metrics, update checks against this repository's releases, the VP8 and Opus encoders and the VideoToolbox H.264 encoder (called directly; no ffmpeg), the VP8 payloader and playout-delay helpers, telemetry storage, and the test card without its gamepad (it shows the frame rate and the remote pointer instead).
- **Flags:** `--config`, `--server-signaling`, `--web-url`, `--headless`, `--debug`, `--version` and `--source screen|testpattern` (the source is used from the next phase on).

### Added (web)

- **desk.go-link.org's first pages:** the landing, `/c` with the code form (the code groups as `915 355 636` while typed; a pasted `/c/<invite>` link works too) and `/c/<code or invite>`, which for now only confirms the code. Strict CSP, self-hosted fonts, light and dark themes, English, Spanish and Portuguese.
- **The shared package** (`@go-link-desk/shared`): go-link's signalhub client, signaling URL helpers, code editing, pure TypeScript SHA-256 and HMAC, linked-browser storage and the design tokens, under go-link-desk names (`APP = "go-link-desk"`, `/c/` links, `go-link-desk.*` storage keys).

### Added (build and CI)

- **`make check`** runs what CI runs: gofmt, `go vet` (with and without the window), `go test -race`, govulncheck, typecheck, Vitest, the web build and gitleaks.
- **A portable macOS app** (`make device-darwin-arm64`, `-amd64`, `-universal`): macOS 13 or newer, libvpx and libopus built from source and linked in, hardened runtime with no entitlement exceptions.
- **`scripts/check-deps.sh`:** fails when a binary needs a library that does not come with the operating system (`otool -L` on macOS, `ldd` on Linux). CI checks the Linux build (Docker, Debian stable) and the macOS build, including the encoders.
- **End-to-end tests** (`make e2e`): a signalhub and a headless host of their own; the code opens the host's invite-only room, a wrong code opens nothing, and the website takes the code.
- **Cross-language test vectors** in `protocol/vectors/`, read by both the Go and the TypeScript tests (the linked-browser proof first).

### Docs

- Architecture, protocol (including the proposed input channels), security and threat model, provenance of every file taken from go-link, and the rules for signalhub proposals.
