<!-- Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> -->
# go-link Desk for Android

The Android app for phones and tablets connects to another computer with the 9-digit code go-link Desk shows there. Once the person at that computer accepts, it shows the computer's screen and controls it. It only connects: the phone or tablet is never shared.

It is the iOS app ([mobile/ios](../ios/README.md)) on Android: the same screens, the same amber look, the same texts in English, Spanish and Portuguese, and the same protocol as the website ([docs/protocol.md](../../docs/protocol.md)), so the computer cannot tell them apart.

- Kotlin and Jetpack Compose, Android 11 (API 30) or newer.
- Application id `org.golink.desk`.

## Layout

| Path | What it is |
|---|---|
| `core/` | Plain Kotlin, no Android types: the portable logic, tested on the JVM (`make test-core`). The iOS app's `DeskCore`, ported. |
| `app/` | The app: Home, the session screen, Settings, the sheets (chat, files, guide), the libwebrtc peer and the keystore. |
| `scripts/strings.py` | Writes `res/values*/strings.xml`: the English is there, the translations are the iOS app's. |

What `core` holds: the signalhub client; the three doors (code, pairing code, linked computer), with the computer's proof (tested against `protocol/vectors/link-proof.json`) and the unattended password; the files channel; the session; the input packets (tested against `protocol/vectors/input.json`); the keys by position (`KeyCodes.gen.kt`, generated from `protocol/keycodes.json`); the pointer and zoom; the clipboard's pieces; the recent computers.

What `app` does:

- The picture: libwebrtc decodes it (H.264 in hardware when the device has it) and draws it into a `TextureView` sized to the picture; the zoom is the view's transform.
- The knock lists only the codecs this device decodes (an emulator may lack H.264 High). When the device cannot encode H.264, the video call's lines are turned down in the answer: this app has no calls yet.
- `RemoteScreenView` takes the fingers, a mouse or trackpad, a hardware keyboard (by position) and the on-screen keyboard (as text).

Dependencies (all Apache-2.0 unless noted): Kotlin, kotlinx coroutines and serialization, Jetpack Compose and AndroidX, OkHttp, and [stream-webrtc-android](https://github.com/GetStream/webrtc-android) (a prebuilt libwebrtc; libwebrtc itself is BSD-3). JUnit (EPL-1.0) for the tests only. The same as go-link's player app, without its camera and QR scanner.

## Using it

**Three ways in:**

- **A session code** (Home): the person at the computer accepts this time.
- **Link a computer** (My computers › Link, with its pairing code from Settings › Unattended access there): the person there accepts once, and this device is linked.
- **A linked computer** (My computers): it opens after the fingerprint, the face or the screen lock (the system's own prompt), without anyone accepting.
  - Each link's token (and the computer's unattended password, once typed) is sealed with a key of Android's keystore, in the device's secure hardware, never backed up. Sealing needs no prompt (the key's public half); opening needs the owner (the private half is usable only right after the prompt).
  - The computer proves itself before the token goes out.

**Fingers**, as on iOS. Trackpad mode (the default): one finger slides the pointer, a tap clicks, a long press then a slide drags; two fingers tap to right-click and pinch to zoom (zoomed in, they move the view); three fingers scroll. Touch mode: the pointer goes where you touch, and two fingers scroll. A mouse or trackpad moves the pointer, clicks and scrolls.

**The toolbar:** disconnect, the keyboard (with a bar of the keys it lacks: Esc, Tab, Ctrl, Alt, Win or ⌘, the arrows, F1 to F12), special keys and shortcuts, chat (with "is typing" and the message sound), files (downloads go to Downloads › go-link-desk), the screen menu (displays, picture, fingers, sound, details), the lock to watch without touching, and the guide. On a phone held upright the lock and the guide move into the screen menu.

## Build

```bash
cd mobile/android
make test-core        # the portable logic's tests and the texts
make debug            # app/build/outputs/apk/debug/app-debug.apk
make install          # on the connected device or emulator
```

It needs JDK 17 (`brew install openjdk@17`) and the Android SDK (`~/Library/Android/sdk`, or `ANDROID_HOME`; a gitignored `local.properties` with `sdk.dir=` also works).

Debug builds only: `adb shell am start -n org.golink.desk/.MainActivity --es code 915355636` connects at once, and `--es signal ws://10.0.2.2:8391/ws` picks a local signalhub (debug builds accept `ws://` to the development machine).

## Release

The store listing, graphics, screenshots script and the Play Console answers are in [play/](play/README.md).


`make bundle VERSION=1.0.0` builds the App Bundle for Google Play, signed with the upload key named by `DESK_SIGNING` (a properties file outside the repository with `storeFile`, `storePassword`, `keyAlias` and `keyPassword`) or by the `DESK_KEYSTORE*` environment variables. Nothing secret is committed.
