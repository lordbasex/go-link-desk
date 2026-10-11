# Architecture

go-link Desk is a remote desktop that follows go-link's design: the computer being controlled runs a small portable app (the **host**), whoever controls it only needs a **browser**, and the two meet through **signalhub**, a generic signaling server that never sees the session.

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
| `backend-device/internal/repositories` | `device.json`, written atomically with mode 0600. |
| `backend-device/internal/gui` | The window, the connection request, the session panel and the tray icon (Fyne). Built unless `-tags headless`. |
| `backend-device/internal/telemetry` | Per-session measurements (SQLite, pure Go). |
| `backend-device/test` | Integration tests: the real stream against a browser played by Pion. |
| `backend-device/pkg/*` | Reusable libraries: `signalclient`, `capture` (ScreenCaptureKit), `cursor`, `beacon`, `permission`, `mainloop`, `encoder` (VideoToolbox H.264, VP8, Opus), `testpattern`, `ids`, `instancelock`, `jsonkeys`, `sysinfo`, `trayicon`, `watermark`. One implementation per OS behind build tags; Windows and Linux come in phase 7. Input injection, clipboard and audio come in later phases. |
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

## The website

desk.go-link.org is the static build in an S3 bucket behind CloudFront (`scripts/deploy-web.sh`, `make web-deploy`), with the same Content Security Policy as the build's, HSTS, and a Permissions-Policy that allows the camera, microphone and clipboard only to the site itself. Pages are revalidated on every visit and hashed assets are cached for a year, so a deploy shows at once. The deploy user may only write that bucket and invalidate that distribution.

## Two kinds of browsers

| Kind | Arrives with | Gets |
|---|---|---|
| **Viewer** | `join` with the session code or invite | After the host accepts: video, audio and the `control`, `pointer`, `input` and `files` channels, within the permissions granted. |
| **Linked browser** | `claim` with a pairing code (first time), `reach` with the `device_id` (after) | Unattended access, off by default (phase 6). The host proves itself first with an HMAC, then the browser shows its token. |

go-link's linked browsers only get `control` and `files`. In go-link Desk a linked browser that passes the token check has to become a full session (video and input) without a consent dialog, within the permission profile of its link. That upgrade is designed in phase 6.

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
| Logs | `~/go-link-desk/logs/desk.log` (when not started from a terminal) |
| Browser storage keys | `go-link-desk.*` |

## The shared signaling server

go-link and go-link Desk use the same signalhub. Its settings must name both:

```
ALLOWED_APPS=go-link,go-link-desk
ALLOWED_ORIGINS=https://go-link.org,https://play.go-link.org,https://desk.go-link.org,...
```

Without `go-link-desk` in `ALLOWED_APPS`, the host's `register` gets `app not allowed`. `MAX_ROOMS_PER_SESSION` and the rate limits are global to all apps; a Desk host uses one room, and `join`, `claim` and `reach` share 5 attempts per minute per IP, so viewers must not retry faster than that.

## A portable host

The host is one self-contained program: no installer, no administrator rights, no program or library that the operating system does not already have.

- libvpx and libopus are built from source and linked in (`backend-device/build/macos/static-libs.sh`, `backend-device/build/linux.Dockerfile`).
- No helper programs: go-link's ffmpeg path for H.264 does not exist here. H.264 comes from the system's encoder (VideoToolbox on macOS), called directly.
- On macOS the host is an app bundle (`go-link-desk.app`), because Screen Recording and Accessibility permissions belong to an app's signature and bundle ID. It can run from Downloads or from the disk image.
- `scripts/check-deps.sh` fails the build when a binary needs anything else; CI runs it on every pull request.
- "Nothing to install" is not "stores nothing": the identity, the linked browsers and the settings live in `device.json`.

## Decisions

- **Phase 0 copies only go-link's generic pieces.** go-link's `internal/services` is one package; its stream, test room and recording services depend on the game room manager and on ROM symbols. They come in the phase that uses them, adapted (stream and session in phase 1, recordings in phase 8), instead of compiling dead code through stubs.
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
- **Sound, clipboard and control are separate boxes, all unticked:** each is a different risk (hearing a call, reading what was copied, using the computer), and the person at the computer picks each one.
- **Audio comes with its phase.** The test card's 1 kHz tone is not sent yet: audio, its permission and the browser's autoplay unlock arrive together in phase 4.
