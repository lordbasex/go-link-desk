# Changelog

All notable changes to go-link Desk. Newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **The Linux host** (phase 7, X11 sessions): a Linux desktop shares its screen as a Mac does. The screen (XShm, every monitor with XRandR), the pointer (XFixes), the mouse and the keyboard by position (XTest, with characters the layout lacks), the clipboard both ways, locking the screen when a session ends (loginctl), and the same window, consent, session panel and settings. Tested on Ubuntu 24.04 (GNOME on Xorg): the consent, the picture, clicks, right clicks, typing (ñ, €, accents) and the clipboard both ways. A Wayland session shows the test card and says, on both sides, to log in with "Ubuntu on Xorg". Not yet: the computer's sound.
- Keys go to each system as its own key (`keymap.Key.Native`: macOS's virtual key, X11's keysym); the test logs keep macOS's numbers everywhere. Paste on Linux is Ctrl+V.
- **A new desk.go-link.org.** The landing (`/`) says what go-link Desk is, with real screenshots: the four ways to connect (the web, iPhone and iPad, Android, the Mac app) and their state, the shared computer (macOS today; Windows and Linux coming), how it works, what it does, why it is safe, downloads, questions, and a footer with the site map, the scam warning, a live check of the connection server and the language. A code typed in its hero opens the session at once.
- **The web app at `/web`, as the iPhone and iPad app:** Home with the code, My computers (rename, favorite, forget in each row's menu) and the recent sessions, and Settings in a sheet (language, appearance, message sound). `/c` and `/devices` lead there; session and linking links keep working.
- **The web session's toolbar, as the iPhone app's:** a floating capsule over the picture (disconnect, the computer's name, Keys, chat, files, video call, sound, Screen, full screen, hide) whose Keys and Screen buttons open menus (special keys, Type text, Ctrl = Cmd, Game mode; displays, picture, view, Retina, the remote cursor). In full screen nothing covers the picture: the toolbar and the details button come back when the pointer reaches the top, or with a tap on the handle.
- **The website speaks the browser's language:** Spanish or Portuguese when the browser prefers them, English otherwise; the switcher still wins.
- **go-link Desk for Android phones and tablets** (`mobile/android`, Android 11 or newer): the iPhone and iPad app on Android, with the same screens, look and texts (English, Spanish and Portuguese, generated from the iOS app's). A session code, linking with the pairing code, and linked computers opened with the fingerprint, face or screen lock (the token sealed in the keystore) and their unattended password kept with the link; the fingers as a trackpad or by touch, a mouse, a hardware keyboard by position and the on-screen keyboard with its bar of keys; special keys, chat with "is typing" and the message sound, files (downloads in Downloads › go-link Desk), displays, picture, sound, details, the lock and the guide. The portable logic (`core`, Kotlin) is the iOS app's `DeskCore`, tested on the JVM against the same vectors; CI runs it and builds the app.
- `gen-keycodes.mjs` also writes the Android app's keyboard table.
- **Google Play, ready to publish** (`mobile/android/play`): the store texts in English, Spanish and Portuguese, the icon and feature graphics, the data safety and content rating answers, and `scripts/screenshots.py`, which takes the phone and 7" and 10" tablet screenshots on emulators. Checked on tablets in both orientations.
- `desk --test-picture FILE` (with `--source testpattern`): a test host shows a still picture instead of the test card, silently, for screenshots of a desktop.
- The website says Android is coming to Google Play, and the privacy page covers the Android app.

### Fixed

- **The code was nearly invisible in the host's window with a light theme** (Linux, Fyne): it took the dark theme's white at start; it now follows the theme.
- **The connection request could stay behind the other windows** (macOS) when it came while the main thread was busy, as right after a session ended: it was brought to the front only within 200 ms of being asked. Now it is looked for until it is on screen (up to 5 s), and put over the app in front even when that app keeps the focus.

## [1.0.0] - 2026-10-11

The first release: go-link Desk for macOS (13 or newer, Intel and Apple silicon, signed and notarized), for iPhone and iPad (iOS and iPadOS 26 or newer, on the App Store), and the website (desk.go-link.org). Tested on an iPhone 17 Pro Max and an iPad Pro (M4), connecting to a Mac.

### Added (the iPhone and iPad app, phase 6c)

- **go-link Desk for iPhone and iPad** (`mobile/ios`): connect to another computer with its 9-digit code, and see and use it once the person there accepts. Home has the code and the recent computers; Settings has your name, the fingers and your own signaling server. English, Spanish and Portuguese.
- **Fingers as a trackpad** (slide, tap, long press to drag; two fingers to right click and zoom; three fingers to scroll) or by touch; an iPad's trackpad, mouse and keyboard; the on-screen keyboard with a bar of special keys; the computer's shortcuts (Ctrl+Alt+Del, Spotlight, switch apps…); the displays, the picture preset, the sound and the details.
- **Files, as on the Mac app:** browse the computer's home folder, tap a file to download it (into the Files app, On My iPhone or iPad › go-link Desk, with Share), and send a file from the Files app or a photo or video to the computer's Downloads/go-link Desk, with progress and Cancel.
- **Unattended access with Face ID** (Touch ID, Optic ID, or the device's passcode): link your own computers once with their pairing code (the person there accepts once), and open them from My computers without anyone there. Each link's token is in the keychain, only on this device and given back only after Face ID; the computer proves itself before the token goes out, as in the website and the Mac app, and its unattended password is asked when it has one.
- **Unattended access always has a password** (all platforms): it cannot be turned on without one, removing it turns it off, and a configuration without one lets no linked device in. Settings asks for it twice, says when it is set or what is wrong, and shows the change at once (it waited for the field to lose the focus). The iPhone and iPad app keeps it with the link, behind Face ID, and sends it by itself; the website asks for it every time. "Linked browsers" are now "linked devices", and the linking code says how to link from the iPhone and iPad app.
- **"Is typing"** in the chat, both ways and everywhere (the computer's session window, the website, the Mac app and the iPhone and iPad app), with three dots that rise in turn: a new `typing {on}` control message.
- **The message sound:** a short sound when the person at the computer writes, on the website (two soft notes, with a 🔔 to turn it off in the chat) and the iPhone and iPad app (the system's chime; Settings › Message sound).
- **Chat** with the person at the computer, with a count of the messages not read and a short notice when one comes in with the chat closed.
- **How to use it:** a guide to the gestures (trackpad and touch), the keyboard and its keys, the toolbar, an iPad's trackpad and mouse, and the lock; it opens the first time a session starts, and from the toolbar's ? button.
- **Lock, to watch** (a film, a presentation): the screen, an iPad's trackpad and keyboard stop sending and the view stays still; the toolbar becomes a small lock, held for a second to unlock.
- **The same protocol as the website and the Mac app**, with libwebrtc (hardware H.264) and the picture shown as soon as it is decoded. `DeskCore`, the app's portable logic, is tested on the Mac (`make test-core`) and in CI, against the same input vectors as the host and the website.
- **Signed and in TestFlight:** `make testflight` archives, signs with the App Store Connect API key and uploads.
- **The Mac app signed and notarized:** `make device-notarized` signs with the Developer ID, notarizes the app and the disk image, and staples their tickets (`make device-dmg` for the disk image alone).
- **A privacy page** on the website (desk.go-link.org/privacy), linked from the footer.

### Added

- **About go-link Desk:** the version, what the app is, its license and website. On macOS it is the system's About panel, first in the app menu, which is now go-link Desk's own (About, Settings… with ⌘,, Hide, Quit) in the app's language; elsewhere, a small window. Also in the menu bar icon's menu.

### Added (the app as a viewer, phase 6b)

- **go-link Desk connects to other computers too,** in one window: Home has the address bar (a 9-digit session code, or a pairing code to link the computer for unattended access), and each session opens in a tab, or in a window of its own (Settings).
- **The same protocol as the website,** so the other computer needs nothing new: the knock, the consent, the permissions, the link, with the computer's proof before the token, and the unattended password.
- **The picture is H.264 drawn by the system** (decoded on the hardware, no copy through Go), with the computer's pointer, and the mouse, keyboard and wheel while it may be controlled: keys by position, shortcuts to the other computer, Cmd+V pasting this computer's clipboard there.
- **In the session's toolbar:** displays, picture preset (Speed, Balanced, Quality, Retina), special keys (switch apps, Spotlight, force quit, lock screen, screenshot; Ctrl+Alt+Del and the Windows key for other systems), type the clipboard, chat, files (browse and download, send with a button or by dropping files on the picture), sound, details and full screen.
- **Home:** this computer's code in a panel of the brand's color (clicking the code or its link copies it), My computers (the linked ones, with favorites), the recent sessions, as cards with a picture of the screen, a list or a detailed list.
- **Settings, native on macOS, in pages:** General (state, appearance: dark, light or the system's theme, the app icon and the menu bar icon in color or dark; the signaling server and website, for a person's own signalhub; language; open at login), Sessions, Unattended access and Permissions.
- **Direct connection (advanced), in Settings:** the UDP port every connection uses and the addresses announced for it, for a computer behind a second router (a double NAT, where browsers on the first network cannot reach it and go through a relay); it shows this computer's address and which port to forward to it, and applies on opening again.
- **The connection request comes with every box ticked** (mouse and keyboard, sound, clipboard, files; not a request to link a browser, which is for good): whoever asks for support accepts with one click, and unticks what they do not want, there or in the session panel. The session panel says "Can see your screen, and:" above its boxes (it said only "See your screen" whatever was allowed). Settings › Sessions › "A request starts with" (Full control, or Only the screen) chooses it ahead of time.
- **A new icon:** a screen with a pointer, with depth in the Dock (amber, or graphite with amber), flat in the menu bar.
- **`--connect`** opens sessions at start; **`--test-viewer`** runs only the viewer, without a window (end-to-end tests only).
- **End-to-end tests:** the app's viewer joins with a code and gets the picture; links with the pairing code, is refused with a wrong password and comes back with the right one.

### Fixed (phase 6b)

- **The bitrate follows what gets through, not only the estimate:** while the picture is small or still, the congestion control saw no congestion and climbed to 30 Mbps, and the next busy screen was sent at 20 Mbps through a relay that carries 2 (40–50% lost, frames dropped, the size falling to 640 and climbing back every half minute). Each viewer's bitrate is now also capped by what it was actually sent: the cap starts at 4 Mbps, grows 15% a second while the encoder fills it and nothing is lost, and drops at once to what got through when the viewer reports 10% loss or more over enough data (by half at most). Without loss it always comes back to 4 Mbps, also on a still screen; beyond that only while the encoder fills it. The session details show that figure as the estimated bandwidth.
- **The picture's size no longer flaps on a shaky network:** the bandwidth ladder still goes down at once, but climbs back one rung at a time and only 10 seconds after its last change. A browser on an iPad saw 21 size changes in two and a half minutes (each a new stream for its decoder) and closed twice.

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
