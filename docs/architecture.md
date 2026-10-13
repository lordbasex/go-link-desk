# Architecture

go-link Desk is a remote desktop that follows go-link's design: the computer being controlled runs a small portable app (the **host**), whoever controls it needs only a **browser** or go-link Desk itself (the same app is also a viewer), and the two meet through **signalhub**, a generic signaling server that never sees the session.

```
 Browser (desk.go-link.org)                               Controlled computer
┌───────────────────────────┐   wss  signalhub v1   ┌──────────────────────────────────┐
│ React viewer              │◄── signal.go-link.org ──►│ go-link Desk host (Go)         │
│  <video> + local cursor   │                       │  capture → encoder → track       │
│  input → DataChannels     │◄══ WebRTC P2P (DTLS/SRTP) ══►│ DataChannels → inject     │
└───────────────────────────┘   TURN only as fallback └──────────────────────────────────┘
```

- **Peer to peer.** Video, audio and control travel over WebRTC between the browser and the host. STUN and TURN come from signalhub's `hello`; TURN only carries traffic when there is no direct path.
- **A star.** The host owns a signalhub session; each viewer is a guest that can only talk to the host.
- **No REST API, no accounts, no backend of our own.** Everything after signaling travels over DataChannels. Favorites and recents live in the browser's `localStorage`.
- **signalhub is used as it is,** with `app: "go-link-desk"`. Anything it would need is written as a generic proposal in [signalhub-proposals.md](signalhub-proposals.md), never patched here.

## Components

| Path | What it is |
|---|---|
| `backend-device/` | The host (Go module `github.com/lordbasex/go-link-desk/backend-device`). |
| `backend-device/cmd/desk` | The entry point: flags, logging, configuration and the wiring of the services. |
| `backend-device/internal/models` | Domain types: configuration, status, sessions (knock, permissions, reasons). |
| `backend-device/internal/services` | Signaling handlers (pairing, room opener, session and consent), the WebRTC stream, linked browsers, PIN gate, status, metrics, updates. |
| `backend-device/internal/controllers` | The routing of viewers' control-channel messages by type, which go-link keeps in one big `switch` in its `main.go`. |
| `backend-device/internal/repositories` | `device.json` and `remotes.json` (the computers this app connects to), written atomically with mode 0600. |
| `backend-device/internal/viewer` | The app's own viewer: a session to another computer with the website's protocol (join, claim, reach and its proof), the WebRTC answerer, frames put back together from RTP, the files client and the input packets. No window: the GUI drives it, and the end-to-end tests run it headless (`--test-viewer`). |
| `backend-device/internal/gui` | The connection request, the session panel, the tray icon and, on systems without the native window, the main window and Settings (Fyne); on macOS, what drives the native main window and Settings (`home.go`, `prefs.go`) and the sessions to other computers (`remote.go`). Built unless `-tags headless`. |
| `backend-device/internal/telemetry` | Per-session measurements (SQLite, pure Go). |
| `backend-device/test` | Integration tests: the real stream against a browser played by Pion. |
| `backend-device/pkg/remoteview` | The native window on macOS (AppKit): Home (this computer's code, the address bar, My computers and the recent sessions as cards, a list or a detailed list), a tab per session (the picture drawn by the system from H.264, the remote pointer, the toolbar, mouse and keyboard), and Settings. Light and dark themes. |
| `backend-device/pkg/fyneview` | The main window and Settings on Windows and Linux, drawn with Fyne to look like the Mac's: the same pages, colors (dark and light), Inter and Lucide icons. On Windows the frame takes the app's colors (DWM). |
| `backend-device/pkg/*` | Reusable libraries: `signalclient`, `capture` (ScreenCaptureKit, XShm, PipeWire, Desktop Duplication), `inject`, `clipboard`, `cursor`, `portal`, `beacon`, `permission`, `mainloop`, `encoder` (VideoToolbox and Media Foundation H.264, VP8, Opus), `testpattern`, `ids`, `instancelock`, `jsonkeys`, `sysinfo`, `trayicon`, `watermark`. One implementation per OS behind build tags. |
| `frontend/` | npm workspaces: `packages/shared` (signalhub client, the session's WebRTC side, codes, HMAC, design tokens) and `apps/web` (desk.go-link.org). |
| `protocol/` | The single source of truth shared by Go and TypeScript: `keycodes.json` and test vectors. |
| `e2e/` | Playwright: a signalhub and a headless host of their own, and a real Chromium. |

## The session room and its code

The host keeps one **invite-only** room open on signalhub for as long as it is connected:

1. `register {app: "go-link-desk", device_id, device_secret}` makes the connection the owner of a session.
2. `room_open {invite_only: true, meta: {name, os}}` → `room_opened {room_id}`.
3. `invite_create {room_id}` → `invite_created {invite, code}`. The 9-digit **code** and the `desk.go-link.org/c/<invite>` link are what the host shows.

The code lives as long as the room: signalhub gives it no expiry. **New code** sends another `invite_create`; signalhub keeps one invitation per room, so the old code stops working at once. After a reconnect the room is opened again and gets a new code.

Signalhub has no admission step: a guest that joins is a session member and can `signal` the host right away. Consent is the host's job: the viewer knocks inside a `signal` payload, the host asks the person at the computer, and only an accepted viewer gets a WebRTC offer. A rejected viewer cannot be removed from signalhub's session; the host ignores it and gives it nothing until it leaves. The messages are in [protocol.md](protocol.md#2-session-start-implemented).

## A session

```
viewer: join {code} ─► joined ─► knock {name, ua}
host:   peer_joined ─► "Ana wants to see your screen" [Reject] [Accept]  (60 s)
host:   accepted ─► offer (video track + control channel) ─► answer ─► ICE ─► video
end:    Disconnect (host) · session_end or leaving (viewer) · the network · signalhub
```

- **Who answers the knock** is a `services.Consent`: the window's request dialog; on a host without a window, nobody (every knock is turned down with `unavailable`); with `--test-auto-accept`, every knock is accepted, which the host only allows together with `--source testpattern`, so skipping consent can never show anyone a real screen.
- **One session at a time.** Remote support is one person helping another; a second viewer, or a second knock while one is being asked, gets `busy`. The stream itself already gives each viewer its own track and encoder, so several viewers are a decision away, not a rewrite.
- **Always visible.** While a session lasts the tray icon is red, the tray menu has Disconnect, and a small panel floats above every window and Space with who is connected, since when, what they can do and Disconnect. It cannot be closed, only ended.
- **The stream** (`internal/services/stream_service.go`) gives each viewer its own video track and VP8 encoder: a keyframe one viewer needs (a new viewer, a lost picture, a connection back) costs the others nothing, unlike go-link's shared track, where one viewer's Picture Loss Indication made keyframes for everyone. Nothing is encoded, and the test card is not drawn, while nobody watches.
- **Control messages** from the viewer go through `internal/controllers`: each message type registers its handler (today `session_end`), and anything else is dropped.

## The picture

```
ScreenCaptureKit ──NV12 pixel buffer──► VideoToolbox H.264 (hardware) ──► the viewer's track
 (only changed frames; scaled on        (no copy; Constrained High;         (its own track and
  the GPU to the size the viewer         low-latency rate control           encoder: keyframes
  needs; cursor left out)                on Apple silicon)                  cost nobody else)
        │                                                    ▲
        └── pointer shape and position ──► control ─────────┘ drawn by the browser
```

- **Capture** (`pkg/capture`) runs only while someone watches, so macOS's recording indicator only shows during a session. It hands over a frame when the picture changes; a still screen sends nothing, except its last frame again a few times, so the encoder sharpens it, and for a viewer that needs a keyframe. Size, frame rate, cursor and display change while it runs.
- **H.264 from the system's encoder** is the main codec: on an M1 it encodes 1080p at 60 fps for 6.7% of one core, against 74% at 30 fps for VP8 (libvpx), and it gives the same quality in fewer bits. VP8 stays for browsers without H.264 and systems without an encoder. The browser says which codecs it takes when it knocks.
- **VP8** (Linux, and Windows without H.264) is tuned for screens as WebRTC tunes it: each frame carries its real time (a desktop changes far less often than 60 times a second, and a frame gets the bits of the time since the last), the bitrate changes while the encoder runs, a 1-second rate control buffer, libvpx's screen content mode, threads no more than the cores, and a **fixed speed of 6**. libvpx's own choice of speed ran, on a busy screen near the frame budget, to settings that were slower and sent 3 to 20 times the bitrate asked; the bandwidth estimate read that as congestion and dropped the picture to 640×400.
- **The size** follows the preset and the viewer's window (`video_want`); **Quality** sends the display's own pixels. The bandwidth estimate (Google Congestion Control, TWCC) sets the bitrate and, when short, a smaller size and frame rate: some encoders (VideoToolbox on Intel Macs) send more than asked on a busy screen, and a smaller picture is what makes frames small enough to arrive whole.
- **The pointer** is not in the picture: its shape and position travel on `control` and the browser draws it, sharp at any size. "Remote cursor in the picture" draws it into the video instead.
- **Without a window** (headless) the host still serves the main thread's event loop on macOS (`pkg/mainloop`): ScreenCaptureKit answers there.
- **Permissions.** Screen Recording and Accessibility belong to the app's signature. The window shows both, with a button that asks the system and opens the right page of System Settings. Without Screen Recording, a session shows the test card and the viewer is told why.

## Control

```
browser: pointer, buttons, wheel, keys ──pointer (unordered) / input (reliable)──► InputService ──► pkg/inject (CGEvent)
          (KeyboardEvent.code, positions                                          (only a viewer with      (holds what it pressed,
           0..65535 over the display)                                              control; seq order)      lets it all go)
```

- **Moves on an unreliable channel, everything else on a reliable one.** A late move is useless and a newer one replaces it; a lost click or key is a bug. A button carries its own position, so it lands where the viewer clicked even when moves were dropped.
- **Keys by position.** The viewer sends which key, not which character, and the host's layout decides: shortcuts, dead keys and a Spanish keyboard's ñ work as on the host. Text typed out (a paste, an IME) goes as text.
- **The viewer's pointer becomes the host's.** While controlling, the browser shows the host's pointer shape as its own cursor, so there is one pointer, not two.
- **Control is a permission** of the session, asked for in the request (off by default) and changed live from the session panel (`permissions` message). Taking it back lets go of everything held.
- **Without Accessibility** the host cannot inject: the viewer is told (`no_accessibility`) instead of clicking into nothing.

## Sound, clipboard and chat

- **Sound** comes from the same ScreenCaptureKit stream as the picture (macOS 13), 32-bit float, turned into 16-bit stereo and cut into 20 ms Opus frames. It is captured only while a viewer may hear it.
- **The clipboard** is text, both ways, only with its permission. The host looks at `NSPasteboard.changeCount` three times a second and sends what is copied from the moment the viewer may share it, never what was already there. The viewer's Cmd+V or Ctrl+V is not sent as keys: the browser's `paste` event carries what is copied on the viewer's computer, without asking for permission in any browser, and the host puts it on its clipboard and presses its own Cmd+V. What the host copies is written with `navigator.clipboard.writeText`; Safari and Firefox allow that only during a click or a key press, so the text waits for the next one, or for a Copy button.
- **Chat** travels on `control`. The host shows it in the session panel without ever taking the focus (the viewer may be typing into another app); its log records a message's length, never its words.
- **Tablets and phones.** A finger drives the host's pointer like a touchpad (drag to move, tap to click, two fingers to right-click or scroll), because a finger is too coarse to click where it lands; the host's pointer is drawn. A touchscreen keyboard sends text rather than keys, so what it inserts goes as `text`, and its Backspace and Return as keys.

## Video call

```
browser camera (H.264) ──► samplebuilder ──► VideoToolbox decoder ──► the call window
browser microphone (Opus) ──► libopus ──► AVAudioEngine (voice processing) ──► speaker
host camera (AVFoundation, NV12) ──► VideoToolbox H.264 ──► the viewer's call track
host microphone ──► AVAudioEngine (echo cancellation) ──► Opus ──► the viewer's call track
```

- **The other side's voice and the host's microphone share one AVAudioEngine** with voice processing: what the speaker plays is taken out of what the microphone hears (echo cancellation), and the computer's other sound keeps its volume. ScreenCaptureKit leaves the host's own process out, so the viewer never hears its own voice back.
- **The camera is light:** 640×480 at 24 fps and 600 kbps each way, next to the screen.
- **Hosts without a camera** (and the end-to-end tests) send the test card and a 440 Hz tone instead.

## Files

- **A channel of their own** (`files`), so a big transfer never holds a control or input message back; uploads are paced by the browser's channel buffer (1 MiB), downloads by the browser's reads.
- **Uploads land in Downloads/go-link Desk,** a known place the person at the computer finds, through a temporary file renamed at the end: a file is never half there, and never over another.
- **Downloads come from the home folder,** browsed but never left; every file that moves is in the host's log (who, which, how big, which way), the audit trail.
- **Measured:** 1 GB from Chromium to the host on this Intel Mac (loopback) in 63 s, 16 MB/s, while the picture kept its frames (the longest gap 150 ms).

## The website

desk.go-link.org is the static build in an S3 bucket behind CloudFront (`scripts/deploy-web.sh`, `make web-deploy`), with the same Content Security Policy as the build's, HSTS, and a Permissions-Policy that allows the camera, microphone and clipboard only to the site itself. Pages are revalidated on every visit and hashed assets are cached for a year, so a deploy shows at once. The deploy user may only write that bucket and invalidate that distribution.

## Two kinds of browsers

| Kind | Arrives with | Gets |
|---|---|---|
| **Viewer** | `join` with the session code or invite | After the host accepts: video, audio and the `control`, `pointer`, `input` and `files` channels, within the permissions granted. |
| **Linked browser** | `claim` with the pairing code (first time, accepted by the person at the host), `reach` with the `device_id` (after) | Unattended access, off by default. The host proves itself first with an HMAC, then the browser shows its token, then the password (unattended access always has one), and it becomes a full session with its link's permissions, nobody asked. |

## The app as a viewer

go-link Desk also connects to other computers, in the same window: Home has the address bar (a session code, or a pairing code to link), My computers and the recent sessions; each session opens in a tab (or, in Settings, a window of its own).

- **The website's protocol, as it is.** `internal/viewer` joins, knocks, answers the offer and speaks the same control messages, input packets and files protocol as the website; the host cannot tell them apart (it sees `go-link Desk 1.x on macOS` where a browser says its name). A linked computer proves itself before the token goes out, exactly as in the browser.
- **The picture is H.264 drawn by the system.** The viewer puts RTP packets back into access units itself (Pion's sample builder holds a frame until the next one starts, and a still screen sends no next one), and hands them to an `AVSampleBufferDisplayLayer`: the hardware decodes and draws, no frame is copied through Go. VP8 (a Linux or Windows computer without an H.264 encoder) is decoded with libvpx, already linked in for the host's own VP8, and its I420 pictures copied into NV12 `CVPixelBuffer`s (BT.601, video range: what the hosts' converters write) for the same layer; a frame that built on a lost one is not drawn, and a keyframe is asked for. A lost packet waits 250 ms for its resend, then the frames until a keyframe are dropped and one is asked for (PLI); so is one when a hidden tab comes back.
- **Input** is captured by the picture's view: keys by position (macOS key codes mapped through `protocol/keycodes.json`), the browser's wheel sign, Cmd+V pasting this computer's clipboard there, and the keys pressed with Cmd let go when Cmd goes up (macOS may never report their release). While the viewer controls, shortcuts go to the other computer, not to this app.
- **Sound** is decoded with libopus and played with the call's audio engine; files go through the same client as the website's (`files.ts`), into Downloads, never over a file.
- **Not yet:** the video call from the app (it ignores the call's lines).
- **What it keeps** (`remotes.json`, 0600): the links (with their tokens, as the browser's `localStorage` does), labels and favorites, the recent sessions, and a small thumbnail of each, taken from the window's own pixels while the session's tab is shown.

## The iPhone and iPad app

`mobile/ios` is a third viewer, with the same protocol: join with the session code, knock, answer the offer, and the control, pointer and input channels. It only connects; it never shows its own screen.

- **Built like go-link's player app, not on the host's Go code.** Swift, with Google's libwebrtc (stasel/WebRTC). libwebrtc brings the jitter buffer, the loss recovery (NACK, PLI), the congestion feedback and the hardware H.264 decoder that Safari and Chrome use, on the networks phones roam between. The protocol is ported to Swift (`DeskCore`) and checked against the same vectors as the host and the website (`protocol/vectors/input.json`, `protocol/keycodes.json`).
- **The picture is shown the way the Mac app shows it:** each decoded frame goes to an `AVSampleBufferDisplayLayer` at once. libwebrtc's Metal view drew nothing in the Simulator, and this path is the shortest.
- **The pointer:** the app asks for the pointer's image and position (`cursor_mode` off) and draws it where the finger put it, without waiting for the picture. While zoomed in, the view follows the pointer.
- **Keys by position from a hardware keyboard, text from the on-screen one** (the 0x31 text packet), so any layout and any language type right.

## The Android app

`mobile/android` is the iPhone and iPad app on Android phones and tablets: the same screens and look, the same texts (the translations are generated from the iOS app's), the same protocol.

- **Built like go-link's Android player app:** Kotlin and Jetpack Compose, with libwebrtc (stream-webrtc-android). The iOS app's `DeskCore` is ported to plain Kotlin (`core`), tested on the JVM against the same vectors and the same cases.
- **The picture** goes into a `TextureView` sized to the picture, so the zoom is the view's transform and the overlays (pointer, toolbar) draw above it.
- **Codecs:** the knock lists what this device decodes (H.264 High, Baseline, VP8). A device that cannot encode H.264 turns down the video call's lines in the answer, since an answer with no common codec on a line fails.
- **Linked computers** are sealed with an RSA key of the keystore: its public half seals (no prompt, as the iOS keychain write), its private half opens only right after the fingerprint, face or screen lock (`BiometricPrompt` with the key's cipher). Hence Android 11 or newer.

## The Linux host

The computer that shares can be a Linux desktop in an **X11 (Xorg)** or a **Wayland** session, with the same window, consent, session panel and settings (Fyne) as everywhere. Tested on Ubuntu 24.04 with GNOME (Xorg and Wayland) and with KDE Plasma 5.27 (X11 and Wayland).

**X11:**

- **The picture:** XShm reads the monitor (XRandR lists them) into shared memory; libyuv (`pkg/capture/frame.c`, shared with Windows) scales it to the size the viewers need (a box filter, so text stays readable) and turns it into I420 for the encoder, skipping a still screen (nothing changed since the last grab). The pointer comes apart (XFixes), as on macOS.
- **Control:** XTest presses keys by the layout's keycode for each key's X11 keysym (`keymap.Key.Native`), so keys go by position; characters the layout lacks are typed on a free keycode mapped to them for the moment. Mouse buttons 1 to 9 and the wheel's clicks (4 to 7).
- **The clipboard:** the CLIPBOARD selection as UTF-8 text, on a thread that owns it when this side copies and asks its owner when it is read; XFixes counts every copy.

**Wayland** lets no program read the screen or move the mouse by itself: the desktop's **remote desktop portal** (xdg-desktop-portal, over D-Bus) does both, in one session (`pkg/portal`).

- **Permission:** the desktop asks the person at the computer the first time, after they accept the connection in go-link Desk (GNOME's own dialog; KDE trusts programs that are not sandboxed and only shows a notice). The portal's restore token, kept next to the settings (`wayland-restore-token`), lets later sessions start without asking; it is kept only when the mouse and keyboard were allowed too (GNOME asks them apart, "Allow Remote Interaction"), so a screen-only choice is asked again. Refusing shows the test card (`share_denied`); a screen without control tells the viewer they can only watch (`no_remote_input`). The session starts with the first viewer and ends with the last, so the desktop's sharing indicator is on only meanwhile.
- **The picture:** the shared monitors arrive as PipeWire streams. libpipewire is loaded with `dlopen` (the binary links none of it: a system without it runs the host, without Wayland capture); the headers' inline helpers build the format request (32-bit RGB in plain memory, no GPU buffers) and parse the answer. Each new picture is copied and scaled to I420 by the same code as X11.
- **The pointer** comes as each stream's metadata (position and image, the image only when it changes), read from every buffer; it is drawn into the picture when the viewer asks for it there, and handed to the cursor package otherwise. The metadata's size range must reach GNOME's 384×384 for it to be agreed on.
- **Control:** the portal's `Notify*` calls. The portal handles calls on a pool of threads, so calls sent without waiting reach the desktop out of order (typed text came out shuffled): a goroutine of the session sends them one at a time, each after the previous one's answer, and drops moves only when its queue is full. Keys go by position (evdev codes, from the shared table's Windows scan codes); typed text goes by the keys of the desktop's layout, read from XKB through XWayland, with the Shift or AltGr each level takes (the portals' own keysym lookups skip levels: KDE typed `'` for `¿`). An accented letter the layout lacks goes by its dead key (á: ´ then a, with a short pause each side, since it is composed apart), and anything else through an input method's Unicode entry (Ctrl+Shift+U, the code, space) when one runs (IBus on GNOME); without one, it is left out.
- **The clipboard:** where the desktop has the data-control protocol (`zwlr_data_control_manager_v1`: KDE Plasma, Sway, Hyprland), a Wayland client of its own reads and sets it, answering the desktop's events while it reads (a clipboard manager such as Klipper takes over what this app copies, and asks for it first). GNOME has no such protocol, but XWayland mirrors its clipboard both ways, so the X11 code above does it. KDE mirrors the X11 one only towards an X11 window in focus, hence the protocol.
- **A desktop without the portal (or without PipeWire)** cannot share: the host says so in its window and to the viewer (`wayland_session`), and sessions show the test card; an Xorg session works.

**Both:**

- **The sound** is what the speakers play: the default output's monitor, through PulseAudio's simple API, which PipeWire answers too (pipewire-pulse). libpulse-simple is loaded at run time, and the few types it needs are declared in the code, so building needs no PulseAudio headers. Unlike macOS, the host's own sound (a video call) is in it: Linux tells no program's sound apart.
- **Keeping the display awake** during a session: every inhibit service the desktop has, over D-Bus (GNOME's session manager, the screen saver and power services of KDE Plasma and Xfce, the Inhibit portal), on a connection that is closed to release them.
- **Locking the screen** when a session ends: `loginctl lock-session`.

## The Windows host

Windows 10 and 11, 64-bit (x86-64 and ARM64), as an ordinary user's program: no administrator rights, per-monitor DPI aware so every monitor is in real pixels. Tested on Windows 11 25H2.

- **The picture:** the Desktop Duplication API reads the monitor on its GPU when the desktop changes; GDI where it cannot run (a remote desktop session, a rotated monitor, a driver without it). libyuv scales and converts it as on Linux. While Windows shows the lock screen or a UAC prompt, which no ordinary program may read, the last picture stays.
- **H.264** from Media Foundation, low latency and constant bitrate: the GPU's encoder first (AMD's, Intel's or NVIDIA's, an asynchronous transform that takes NV12, driven by its events from a thread of its own), tried once with a frame that must come back as a keyframe with its SPS and PPS; else Windows' own (software on every Windows). A GPU encoder that stops during a session (a driver reset) hands over to Windows' own from a keyframe, and is not used again until the program restarts. The log's `video encoder` line names the encoder and says whether it is hardware. mfplat.dll is loaded when the first encoder opens, so the N editions without the Media Feature Pack start too and use VP8.
- **Control:** SendInput, keys by scan code (the computer's layout makes the characters), typed text as Unicode. Windows lets no ordinary program drive a window running as administrator, nor the secure desktop.
- **The rest:** sound from a WASAPI loopback stream of the default output, the pointer's shape and position for the viewer to draw, the clipboard (Unicode text), a power request that keeps the display awake, Windows+L to lock, and the user's Run key to start with Windows.
- **One copy at a time** (`pkg/instancelock`, on every system: `flock` on macOS and Linux, `LockFileEx` on Windows, on `desk.lock` next to the settings). On Windows, opening it again wakes the running one through a named event, so its window comes forward instead of a second copy quitting silently.
- **The window:** `pkg/fyneview`, the Mac's look in Fyne. The frame is Windows' own (its buttons), colored by DWM in the app's colors. The app's theme and font are set before anything measures text: Fyne keeps the faces it measured with.
- **Release:** an installer for the user (NSIS, in `%LOCALAPPDATA%\Programs\go-link Desk`, with its uninstaller) and the same program in a zip. Not signed yet: SmartScreen warns before the first run.

## Unattended access

- **One door, three ways in.** The session service takes guests from the session room (`join`), from the pairing code (`claim`) and from `reach`; all of them knock with the same message, and only what follows the knock changes. A browser that came with the pairing code is asked about like any viewer, in a request window that says it is a link; one that came with `reach` is connected gated.
- **Gated, not a second connection.** go-link opens a data-only connection to a linked browser. Desk connects a returning browser with the session's own connection, its tracks and channels, but kept in a waiting list of the stream: no encoder, no sound, no pointer, no displays, no `welcome`, and its control messages go only to the unattended service. Letting it in moves it to the viewers and asks a keyframe: no new negotiation, no second ICE, and nothing a gated browser sends can reach the session. A handover from a data link to a session connection was the alternative; it costs a second negotiation and a moment with two connections to the same browser.
- **The link's profile is what the person ticked** when the browser was linked, and can be changed or revoked in Settings, one line per browser (its declared name, when linked, last used).
- **Settings** holds what concerns unattended access (off by default, the linking code, the password, the linked browsers) away from the main window, which only shows the session code. Its changes go to `device.json` through the settings service, which the other services read.
- **Around a session:** the display is kept awake while one is on (an IOKit assertion on macOS, the desktop's inhibit services on Linux; a viewer's activity is not the local person's), a session with no input from the viewer for the time set in Settings ends with `idle`, the screen can lock when a session ends (Control-Command-Q on macOS, as the person would, with the Accessibility permission the input already needs; `loginctl lock-session` on Linux), and the host can open at login (a LaunchAgent on macOS, an XDG autostart file on Linux, pointing at the binary; nothing is installed).
- **My computers** on the website is this browser's list of linked computers and recent sessions, in `localStorage`: favorites first, a name the person picks, the last use, and a thumbnail of the screen. The browser takes the thumbnail from the picture it is showing, every few seconds during the session; the host sending one at the end was the plan, but the browser already has the picture, and a session that ends with the network still leaves it one.

## Identifiers

| What | Value |
|---|---|
| Product | go-link Desk |
| Website | `https://desk.go-link.org` |
| signalhub `app` | `go-link-desk` |
| Signaling | `wss://signal.go-link.org/ws?v=1` (shared with go-link) |
| macOS bundle ID | `org.go-link.desk` |
| Binary | `go-link-desk` |
| Configuration | `os.UserConfigDir()/go-link-desk/device.json` |
| Logs | `~/go-link-desk/logs/desk.log`, on Windows `%LOCALAPPDATA%\go-link-desk\logs\desk.log` (when not started from a terminal) |
| Browser storage keys | `go-link-desk.*` |

## The shared signaling server

go-link and go-link Desk use the same signalhub. Its settings must name both:

```
ALLOWED_APPS=go-link,go-link-desk
ALLOWED_ORIGINS=https://go-link.org,https://play.go-link.org,https://desk.go-link.org,...
```

Without `go-link-desk` in `ALLOWED_APPS`, the host's `register` gets `app not allowed`. `MAX_ROOMS_PER_SESSION` and the rate limits are global to all apps; a Desk host uses one room, and `join`, `claim` and `reach` share 5 attempts per minute per IP, so viewers must not retry faster than that.

## Updates

The app looks for a newer release when it opens (after 30 s) and every 6 hours (`services.UpdateService`, the GitHub releases list; drafts and pre-releases are skipped), and on **Check for Updates…** (the Mac's app menu, the tray's menu, the About window off the Mac). A release found on its own opens the update window once, and not during a session: then when it ends. **Update** (`internal/updater`):

1. **Download and check.** It fetches the release's `SHA256SUMS` and `SHA256SUMS.sig`, an Ed25519 signature by the update key, whose public half is in the program and whose private half never leaves the release computer (`make release-sums`, `build/signsums`). Then it fetches the one file this copy needs, checks its hash against the signed list and throws it away otherwise. The release's host (GitHub) is not trusted with what gets installed.
2. **Install, by how this copy was installed:**
   - **macOS:** the disk image is mounted, the app in it must carry a valid signature by the same developer team as the running app (`codesign`), and it is copied next to the running bundle and swapped in by two renames. A development build (ad hoc signature) or an app in a folder the user may not write is updated by hand.
   - **Windows:** the installer runs silently with `/S /UPDATE`. It closes the app, installs over it for the user (no administrator) and opens the new one. A portable copy (the .zip) is updated by hand.
   - **Linux:** a copy of the user's own (the .tar.gz's `install.sh`, in `~/.local/bin`) runs the new `install.sh`; a package in `/usr/bin` is installed with its package manager through `pkexec` (`dpkg -i`, `rpm -U` or `pacman -U`), so the desktop asks for the administrator's password. Any other copy is updated by hand.
3. **Open the new version.** The app quits; on macOS and Linux a shell of its own waits for it to exit and opens the new one, and on Windows the installer does.

"By hand" opens the release's page instead. `DESK_UPDATE_BASE` points the downloads at a test server; the signature is still required.

## A portable host

The host is one self-contained program: no installer, no administrator rights, no program or library that the operating system does not already have.

- libvpx and libopus are built from source and linked in (`backend-device/build/macos/static-libs.sh`, `backend-device/build/linux.Dockerfile`, `backend-device/build/windows.Dockerfile`), and so is libyuv on Linux and Windows.
- No helper programs: go-link's ffmpeg path for H.264 does not exist here. H.264 comes from the system's encoder (VideoToolbox on macOS, Media Foundation on Windows, VA-API on Linux), called directly. Libraries a system may lack (libpipewire, libpulse-simple, mfplat) are loaded at run time, so the host starts without them.
- On macOS the host is an app bundle (`go-link-desk.app`), because Screen Recording and Accessibility permissions belong to an app's signature and bundle ID. It can run from Downloads or from the disk image.
- `scripts/check-deps.sh` fails the build when a binary needs anything else; CI runs it on every pull request.
- "Nothing to install" is not "stores nothing": the identity, the linked browsers and the settings live in `device.json`.

## Decisions

- **The app is also a viewer (phase 6b).** The plan had the viewer only in the browser; the app now connects too, in one window with a tab per session. It reuses everything the host already had (Pion, signalclient, libopus, the call's audio engine, the input protocol) and the website's protocol unchanged, so the host needs no change and the browser keeps working as before.
- **The main window and Settings are native on macOS** (AppKit through cgo): Fyne cannot hold the system's video layer, and a single window with the sessions as tabs needs it. Fyne stays for the connection request, the session panel, the chat and files windows, and as the whole window on systems without the native one.
- **Windows and Linux get the Mac's window, drawn with Fyne** (`pkg/fyneview`), rather than a native one per system: one implementation for both, the same pages and texts as the Mac, at the cost of sessions not yet opening in the app's own tabs there (Connect opens the website's viewer).
- **The minimum hardware is measured, not guessed:** `build/vbench` encodes a terminal's text scrolling at each size, with the host's encoders and tuning, and the website's requirements page (`/docs/requirements`) comes from its numbers.
- **The signaling server can be changed in Settings** (a person's own signalhub), and takes effect when the app opens again: the host's room, codes and links live on the server it registered with.

- **Phase 0 copies only go-link's generic pieces.** go-link's `internal/services` is one package; its stream, test room and recording services depend on the game room manager and on ROM symbols. They come in the phase that uses them, adapted (stream and session in phase 1, recordings in phase 8), instead of compiling dead code through stubs.
- **The unattended password is hashed with argon2id** (`golang.org/x/crypto/argon2`, OWASP's minimum parameters: 19 MiB, 2 passes), never kept or logged in clear, and checked with the PIN gate's limits. `x/crypto` was already in the build through Pion; it is now a direct dependency (BSD-3-Clause).
- **The linked-browser proof is domain-separated:** it signs `go-link-desk link proof\n<nonce>`, so a proof made for one app never passes in the other.
- **The dev server uses port 5180,** like go-link's, because the shared signalhub already allows `http://localhost:5180`.
- **The Fyne window is new.** go-link's window is built around the game library; Desk keeps only its mechanisms (the i18n catalogs, the tray icon, the stop-safe app loop).
- **Phase 1 streamed the test card;** phase 2 swapped the source for the screen. The test card stays for diagnostics, CI and the end-to-end tests (`--source testpattern`), and as the fallback when the screen cannot be recorded.
- **60 frames per second in every preset** (the plan said 30 for Balanced): it costs 6.7% of an M1 core at 1080p, comparable products advertise 60, and a still screen sends nothing anyway. The bandwidth ladder drops to 30 when the network is short.
- **The whole display is captured, the host's own windows included** (the session panel, the beacon): what the viewer sees is what the person at the computer sees, and a viewer never clicks on a window it cannot see.
- **Measurement builds** (`-tags desk_test_screen`, `make device-darwin-arm64 GO_TAGS=desk_test_screen`) may accept viewers of the real screen without asking, to measure capture, latency and CPU on a test machine. A normal build refuses `--test-auto-accept` with the screen, and the release target refuses the tag.
- **The permissions the dialog shows are the ones that exist.** In phase 1 a viewer can only see the screen, so the dialog says that and has no checkboxes; each phase adds the checkbox of what it brings (control, audio, clipboard, files), already defined in the `accepted` message.
- **Control has one checkbox,** "Use your mouse and keyboard": a viewer that may control the mouse but not the keyboard is not a useful permission, and one checkbox reads clearly in a request dialog.
- **The host applies the modifiers a key comes with** rather than only the ones whose presses it saw: a viewer that held Shift before the page had the focus still types a capital.
- **Sound, clipboard and control are separate boxes,** each a different risk (hearing a call, reading what was copied, using the computer). They came unticked until phase 6b; now they come ticked in a session's request (not in a link's, which is for good), because asking for support should take one click, and the person at the computer unticks what they do not want, there or during the session. Settings can make them start unticked again ("A request starts with: Only the screen"); a one-click "Allow full control" button was weighed and left out, since the scammer on the phone asks for that click as easily as for Accept.
- **A video call is asked for during the session, not in the connection request:** it is a moment, not a standing permission; the person at the host answers it then, with their camera and microphone as boxes.
- **Audio comes with its phase.** The test card's 1 kHz tone is not sent yet: audio, its permission and the browser's autoplay unlock arrive together in phase 4.
