# Changelog

All notable changes to go-link Desk. Newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added (unattended access and my computers, phase 6)

- **Unattended access** (Settings, off by default): a browser linked once with the computer's linking code comes back from My computers without a code and without anyone accepting, also after the computer restarts. Linking asks the person at the computer, and the permissions they tick are the link's.
- **The computer proves itself first, then the browser its token,** over the encrypted connection; until then the browser is connected but gets nothing, not even the computer's name. The host keeps only the token's hash.
- **An optional password** for unattended access, after the link: argon2id, 5 tries per browser, and 20 wrong ones in 10 minutes lock it for 10 minutes.
- **Linked browsers in Settings:** their name, when linked and last used, their permissions (changed there) and Revoke; a revoked browser is turned away, also mid-session.
- **My computers** on the website (`/devices`): the linked computers with a thumbnail of their screen, favorites first, renamed and forgotten here; the recent sessions; `/link` to link a computer.
- **Around a session:** disconnect after 5 minutes to an hour without activity, lock the screen when a session ends, keep the display awake while one is on, and open go-link Desk at login (a LaunchAgent on macOS, an autostart entry on Linux).
- **The host's language** can be picked in Settings.
- **`--test-unattended`** and **`--test-unattended-password`** for the end-to-end tests (only with `--test-auto-accept`, and never saved).
- **End-to-end tests:** a browser linked with the code comes back without it after the host restarts, types the password when one is set, and is turned away once its link is revoked; without unattended access the linking code links nothing.
- **Dependency:** `golang.org/x/crypto` (BSD-3-Clause, already in the build through Pion) is now direct, for argon2id.

### Added (files, phase 5)

- **Send files to the computer** by dropping them anywhere on the remote screen or choosing them: they land in Downloads › go-link Desk, never over a file already there, with progress on both sides and a Stop button.
- **Browse the computer's home folder and download** its files: Chromium writes straight to a file the person picks; other browsers save it when complete.
- **The permission** "Send and receive files", unticked by default, in the request and the session panel; the panel shows the file moving.
- **An audit trail:** every file sent or received is in the host's log.
- **Measured:** 1 GB sent in 63 s (16 MB/s) while the picture kept flowing (`E2E_BIG_FILE_MB=1024`).
- **End-to-end tests:** a big file's content checked by SHA-256 while the picture's frames are timed, a second file with the same name saved beside it, a download from the host's Documents with hidden files left out, and no files without the permission.
- `make check` also type-checks the end-to-end tests.

### Added (video call, phase 4b)

- **A video call between the viewer and the person at the host,** both ways: the viewer asks from the session's toolbar, the person at the host takes it (choosing camera and microphone) or not, and each side can mute, turn the camera off or hang up.
- **On the host:** the camera with AVFoundation, the microphone with the system's echo cancellation (AVAudioEngine voice processing), the viewer's camera decoded by VideoToolbox in a call window, and its voice played. No new dependencies.
- **On the website:** a call panel with the host's camera large and the viewer's own small, using the browser's camera and microphone with echo cancellation.
- **No new negotiation:** a call's media is in every offer and starts with `replaceTrack`.
- **macOS:** the camera and audio-input entitlements and their usage texts in Info.plist.
- **End-to-end test:** a fake camera and microphone in Chromium; the host sees and hears the viewer, the viewer sees the host's test card and hears its tone, and a click on the call's video never reaches the host.

### Added (sound, clipboard and chat, phase 4)

- **The computer's sound** in the browser: ScreenCaptureKit captures it (without the host's own) and Opus carries it at 96 kbps stereo, only to a viewer allowed to hear it. The page asks for a click if the browser blocks autoplay, and has a mute button. The test card plays its 1 kHz tone.
- **The clipboard, both ways, as text:** what is copied on the host lands on the viewer's clipboard (in Safari and Firefox at the next click or key, or with a Copy button), and the viewer's Cmd+V or Ctrl+V pastes what is copied on the viewer's computer into the host. Up to 1 MiB, in 16 KiB pieces.
- **Chat** between the viewer and the person at the computer, in the session panel and in a panel on the website, with an unread count.
- **Permissions** "Hear your computer's sound" and "Share the clipboard (copy and paste)" in the connection request and the session panel, unticked by default.
- **Tablets and phones:** touchpad mode (drag to move the pointer, tap to click, two fingers to right-click and scroll) with the host's pointer drawn, and a Keyboard button for the touchscreen keyboard, whose typing reaches the host.
- **Session details** float over the picture, as in go-link's player, also in full screen, with new rows (quality, jitter buffer, freezes, sound) and "Show on screen": the key figures in one line over the picture.
- **desk.go-link.org** is published (S3 and CloudFront): `make web-deploy`.
- **`--test-clipboard-file`** keeps the host's clipboard in a file for the end-to-end tests.
- **End-to-end tests:** the tone is heard by a viewer allowed to, silence for one that is not; the clipboard both ways; the chat.

### Added (control, phase 3)

- **Control of the host's mouse and keyboard** from the browser, when the person at the computer allows it: a "Use your mouse and keyboard" checkbox in the connection request, off by default, and a switch in the session panel that gives or takes it back during the session.
- **Mouse:** moves at up to one per frame on an unreliable channel, clicks that carry their own position, drags, double and triple clicks, back and forward buttons, and smooth (pixel) or line scrolling.
- **Keyboard by position:** the host's own layout makes the characters, so shortcuts, dead keys and letters like ñ work as on the host. Key repeat, IME input (Chinese, Japanese) and "Type text…" for pasting text as typing.
- **Nothing stays stuck:** everything held is let go when the page loses the focus, is hidden or leaves full screen, when the viewer loses control, hangs up or its connection stops answering. On macOS browsers, keys pressed with Cmd are let go when Cmd is.
- **On the website:** a Keys menu with the host's special shortcuts (Cmd+Tab, Force Quit, Spotlight and screenshots on a Mac; Ctrl+Alt+Del, Alt+Tab and the Windows key elsewhere), "Ctrl is Cmd" for viewers on Windows or Linux, game mode (pointer lock), the keyboard locked in full screen so the system's shortcuts reach the host, and the host's pointer shape as the browser's cursor. The viewer is told when the host has no Accessibility permission.
- **One list of keys,** `protocol/keycodes.json`, with the Go and TypeScript tables generated from it, and shared test vectors for the binary packets.
- **`--test-input-log`** writes viewers' input to the log instead of injecting it, for the end-to-end tests; it needs `--test-auto-accept`.
- **End-to-end tests** of control: a click, keys, Ctrl+C, the wheel, typed text and the release when the page loses the focus, and a watch-only viewer that gets no keyboard.

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
