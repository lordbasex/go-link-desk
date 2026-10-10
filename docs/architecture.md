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
| `backend-device/internal/models` | Domain types: configuration, status. |
| `backend-device/internal/services` | Signaling handlers (pairing, room opener, session), linked browsers, PIN gate, status, metrics, updates, WebRTC helpers. |
| `backend-device/internal/controllers` | (From phase 1.) The routing of control-channel and owner messages, which go-link keeps in one big `switch` in its `main.go`. |
| `backend-device/internal/repositories` | `device.json`, written atomically with mode 0600. |
| `backend-device/internal/gui` | The window and tray icon (Fyne). Built unless `-tags headless`. |
| `backend-device/internal/telemetry` | Per-session measurements (SQLite, pure Go). |
| `backend-device/pkg/*` | Reusable libraries: `signalclient`, `encoder` (VP8, Opus, VideoToolbox H.264), `testpattern`, `ids`, `instancelock`, `jsonkeys`, `sysinfo`, `trayicon`, `watermark`. Capture, input injection, clipboard and audio come in later phases, one implementation per OS behind build tags. |
| `frontend/` | npm workspaces: `packages/shared` (signalhub client, codes, HMAC, design tokens) and `apps/web` (desk.go-link.org). |
| `protocol/` | The single source of truth shared by Go and TypeScript: `keycodes.json` and test vectors. |
| `e2e/` | Playwright: a signalhub and a headless host of their own, and a real Chromium. |

## The session room and its code

The host keeps one **invite-only** room open on signalhub for as long as it is connected:

1. `register {app: "go-link-desk", device_id, device_secret}` makes the connection the owner of a session.
2. `room_open {invite_only: true, meta: {name, os}}` → `room_opened {room_id}`.
3. `invite_create {room_id}` → `invite_created {invite, code}`. The 9-digit **code** and the `desk.go-link.org/c/<invite>` link are what the host shows.

The code lives as long as the room: signalhub gives it no expiry. **New code** sends another `invite_create`; signalhub keeps one invitation per room, so the old code stops working at once. After a reconnect the room is opened again and gets a new code.

Signalhub has no admission step: a guest that joins is a session member and can `signal` the host right away. Consent is the host's job (phase 1): the viewer knocks inside a `signal` payload, the host asks the person at the computer, and only an accepted viewer gets a WebRTC offer. A rejected viewer cannot be removed from signalhub's session; the host ignores it and gives it nothing until it leaves.

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
