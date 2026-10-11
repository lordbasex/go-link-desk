<!-- Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> -->
# go-link Desk for iPhone and iPad

The iPhone and iPad app connects to another computer with the 9-digit code go-link Desk shows there. Once the person at that computer accepts, it shows the computer's screen and controls it. It only connects: an iPhone or iPad is never shared.

- SwiftUI, iOS 17 or newer.
- Bundle id `org.go-link.desk.ios`.
- It speaks the same protocol as the website ([docs/protocol.md](../../docs/protocol.md)), so the computer cannot tell them apart.

It is distributed through **TestFlight** (see [TestFlight](#testflight)).

## Layout

| Path | What it is |
|---|---|
| `DeskCore/` | A Swift package with the portable logic. `swift test` runs it on the Mac. No UIKit and no WebRTC here. |
| `DeskViewer/` | The app: Home, the session screen, Settings and the libwebrtc peer. |
| `project.yml` | The [XcodeGen](https://github.com/yonaskolb/XcodeGen) spec. `GoLinkDesk.xcodeproj` is generated and not committed. |
| `Config/` | Build settings; your Apple team goes in the gitignored `Local.xcconfig`. |
| `scripts/` | The translations check, and a small App Store Connect API client. |

What `DeskCore` holds:

- the signalhub client;
- the files channel (browse, download, send);
- the session (join, knock, offer and answer, the control messages);
- the input channel's packets, tested against `protocol/vectors/input.json`;
- the keys by position (`KeyCodes.gen.swift`, generated from `protocol/keycodes.json`);
- the pointer and zoom;
- the clipboard's pieces;
- the recent computers.

What `DeskViewer` does:

- The picture (`PictureLayerView`) goes to an `AVSampleBufferDisplayLayer` and is shown as soon as it is decoded, as in the Mac app.
- `RemoteScreen` takes the fingers, an iPad's trackpad or mouse, and the keyboard.

Dependencies are Swift packages only: [stasel/WebRTC](https://github.com/stasel/WebRTC) (a prebuilt libwebrtc, BSD-3, pinned in `project.yml`) and the local `DeskCore`.

## Using it

**Fingers, trackpad mode (the default):**

- One finger slides the pointer: slowly to be precise, quickly to go further.
- A tap clicks.
- A long press, then a slide, drags.
- Two fingers: a tap right-clicks and a pinch zooms. Zoomed in, a two-finger slide moves the view, and the view follows the pointer.
- Three fingers scroll. The system's three-finger gestures (undo, the edit menu) are off over the computer's screen.

**Touch mode** (the screen menu or Settings): the pointer goes where you touch, and two fingers scroll.

**An iPad's trackpad or mouse:** the pointer follows it, the buttons click and scrolling scrolls.

**Keyboards:**

- A hardware keyboard's keys go by position: the same place on the computer's keyboard, whatever the layouts.
- The on-screen keyboard sends the text typed.
- Over the on-screen keyboard, a bar has Esc, Tab, Ctrl, Alt/⌥, Win/⌘, ⇧, the arrows, Delete, Home/End, Page Up/Down and F1 to F12. The modifiers latch for the next key, so Ctrl then C is Ctrl+C.

**The toolbar:**

- Files (with the files permission):
  - browse the computer's home folder;
  - tap a file to download it into the Files app (On My iPhone or iPad › go-link Desk);
  - Send takes a file from the Files app, or a photo or video, to the computer's Downloads/go-link Desk.
- Chat with the person at the computer. A number counts the messages not read, and a short notice shows one that comes in while the chat is closed.
- Lock, to watch without touching anything. Nothing is sent and the view stays still; hold the lock for a second to unlock.
- ? opens the guide to the gestures, keys and toolbar. It also opens by itself the first time a session starts.
- On a phone held upright, the toolbar drops the computer's name, and Lock and ? move into the screen menu.

- Special keys:
  - on a Mac: switch apps, Spotlight, force quit, lock, screenshot;
  - elsewhere: Ctrl+Alt+Del, Start, Task manager, lock;
  - everywhere: copy, paste, undo, select all.
- Paste this device's clipboard (with the clipboard permission).
- The screen menu:
  - the computer's displays;
  - the picture preset (Speed, Balanced, Quality);
  - trackpad or touch;
  - mute;
  - the details (frames per second, size, bitrate, loss, round trip, direct or relay).

## Build and run

You need Xcode 26 and XcodeGen (`brew install xcodegen`).

```bash
cd mobile/ios
make test-core      # DeskCore's tests, and every text translated
make build-sim      # a Simulator build, no signing (SIM="iPhone 17 Pro")
open GoLinkDesk.xcodeproj
```

Debug builds also accept these launch arguments:

- `-code 915355636`: connects at once.
- `-signal ws://127.0.0.1:8391/ws`: a local signalhub.

For example:

```bash
xcrun simctl launch booted org.go-link.desk.ios -code 915355636
```

Debug builds log the session's steps and numbers to the unified log, but never a code or a name. To watch them:

```bash
xcrun simctl spawn booted log stream --level info --predicate 'subsystem == "org.go-link.desk.ios"'
```

To try it against a computer without a person there, run a test host:

```bash
desk --headless --source testpattern --test-auto-accept --test-input-log
```

It accepts every viewer, shows the test card, and logs what the app sends instead of injecting it.

## Signing, devices and TestFlight

Nothing about signing is committed. Either put your team in `Config/Local.xcconfig` (copy the example), or source the environment the Mac app's signing uses:

```bash
source ~/.go-link-signing/env.sh   # TEAM_ID, ASC_KEY_ID, ASC_ISSUER_ID, ASC_KEY_PATH
make device-build                  # a signed build for an iPhone or iPad
make install-device                # build, install and open it on the connected one
make testflight VERSION=0.1.0      # archive, sign and upload a build
```

With the App Store Connect API key, `xcodebuild` creates the certificates and profiles by itself, registers the device and uploads, without Xcode's Accounts. `scripts/asc.py` calls the same API for what `xcodebuild` does not do, such as the TestFlight groups and the beta review.

### TestFlight

App Store Connect has the app **go-link Desk** (bundle id `org.go-link.desk.ios`, SKU `golink-desk-ios`) with two groups:

- **Equipo** (internal): the team's own App Store Connect users get every build as soon as Apple processes it, with no review.
- **Amigos** (external): anyone with the public link, up to 100 people. Each build is submitted to Beta App Review, and once it is approved the link installs it through the TestFlight app.

The beta's description is in English, Spanish and Portuguese. The privacy policy is https://desk.go-link.org/privacy.
