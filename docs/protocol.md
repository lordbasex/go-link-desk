# Protocol

Everything between the browser and the host. Signaling goes through signalhub v1 (its README is the contract); everything after that travels over WebRTC DataChannels, encrypted end to end with DTLS.

Status of each part: **implemented** (in this repository and tested), **from go-link** (implemented there, copied here, wired in a later phase), or **proposed** (to be implemented and tested in the phase noted).

## 1. Signalhub (implemented)

`wss://signal.go-link.org/ws?v=1`, one JSON object per message, `app: "go-link-desk"`.

| Step | Host | Viewer |
|---|---|---|
| Connect | receives `hello {peer_id, ice_servers}` | receives `hello` |
| Own a session | `register {app, device_id, device_secret}` → `code` (pairing code, used for unattended access) | |
| Open the room | `room_open {invite_only: true, meta: {name, os}}` → `room_opened {room_id}` | |
| Make the code | `invite_create {room_id}` → `invite_created {room_id, invite, code}` | |
| Join | receives `peer_joined {session_id, remote, room_id}` | `join {app, code}` or `join {app, invite}` → `joined {session_id, remote, room_id}` |
| Talk | `signal {to, payload}` / receives `signal {from, payload}` | the same, only to the host |
| Leave | receives `peer_left {from}` | |

- `meta` is public: anyone who joins sees it. It only carries the computer's short name and its OS.
- The code is 9 digits (`915355636`, shown as `915 355 636`). It has no expiry; a new `invite_create` replaces it.
- A room opened `invite_only` cannot be joined by its `room_id`.

## 2. Session start (implemented)

All of these travel in signalhub's opaque `signal.payload`, with the same pattern as go-link's PIN gate (`pin_required` / `pin_result`).

```
viewer → host   {"kind":"knock","name":"Ana","ua":"Chrome 141 on macOS"}
host   → viewer {"kind":"accepted","permissions":{"view":true,"control":false,"audio":false,"clipboard":false,"files":false}}
host   → viewer {"kind":"rejected","reason":"declined"|"timeout"|"busy"|"unavailable"}
host   → viewer {"kind":"offer","sdp":"…"}                    (only after accepted; again for an ICE restart)
viewer → host   {"kind":"answer","sdp":"…"}
both            {"kind":"candidate","candidate":{…}}           (trickle ICE)
```

- The person at the host decides within 60 seconds; no answer is `rejected` with `timeout`. A host without a window has nobody to ask and answers `unavailable` at once.
- **One session at a time.** A knock while another viewer is in session, or while another knock is being asked, is `rejected` with `busy`.
- **One knock per guest.** A guest that was turned down, or whose session ended, is ignored until it leaves signalhub's session; to try again it joins again (signalhub limits `join` to 5 a minute per IP).
- The host never sends an offer, and ignores every offer, answer or candidate, from a viewer that was not accepted. A viewer never sends an offer.
- `name` (up to 40 characters) and `ua` (up to 60) are what the viewer declares. The host keeps printable characters only, without line breaks or control and format characters, and shows them as declared, never as verified.
- `view` is always granted; the other permissions are part of the message from the start and are granted in the phases that use them.
- The host leaving signalhub (`peer_left` from the host) ends the session for the viewer. The viewer leaving signalhub (`peer_left` to the host) ends it for the host.
- **ICE restart.** When the connection is `disconnected` for 3 seconds, or `failed`, the host offers again with new ICE credentials over signalhub, up to 3 times in a row; after that the session ends with `network`. The viewer answers every offer on the same connection, applying signals strictly in order (a new candidate must never pass its offer).

| Session end reason | Meaning |
|---|---|
| `host` | The person at the host pressed Disconnect. |
| `viewer` | The viewer hung up (`session_end` on `control`) or left signalhub. |
| `network` | The connection failed and could not be brought back. |
| `signal` | The host lost signalhub, and the room with it. |

### Proposed

A session password (phase 6), with go-link's PIN gate: `pin_required` after the knock, `pin`, and `pin_result {ok, reason, left}`.

## 3. Linked browsers (from go-link)

For unattended access (phase 6). The browser links once with the host's pairing code, then comes back with `reach`.

1. `claim {app, code}` → both get `paired`. The host opens a data-only WebRTC connection.
2. First time: browser `auth {}` → host `auth_ok {device_id, link_id, token}`. The browser keeps the token; the host keeps only its SHA-256.
3. Coming back with `reach {app, device_id}`:
   1. browser → `auth_challenge {link_id, nonce}` (16 to 128 characters)
   2. host → `auth_proof {link_id, proof}`, where `proof = base64url(HMAC-SHA256(key = SHA-256(token), "go-link-desk link proof\n" + nonce))`, without padding
   3. only when the proof is right: browser → `auth {link_id, token}`
   4. host → `auth_ok` or `auth_failed` (and hangs up)

The host proves itself first, so a fake signaling server never learns a token. A browser that shows no valid token within 15 seconds is dropped. The vector both sides test against is `protocol/vectors/link-proof.json`.

## 4. DataChannels

The host creates them, as in go-link. `control` is implemented; the others come in their phases.

| Label | Settings | Content | Status |
|---|---|---|---|
| `control` | ordered, reliable, JSON | see below | implemented |
| `pointer` | `ordered: false`, `maxRetransmits: 0`, binary | pointer moves only | proposed, phase 3 |
| `input` | ordered, reliable, binary | buttons, wheel, keys, text, `release_all` | proposed, phase 3 |
| `files` | ordered, reliable | go-link's file protocol (section 6) | proposed, phase 5 |

### `control` messages

| Message | Direction | Content |
|---|---|---|
| `welcome` | host → viewer | `{host: {name, os}, source}` when the channel opens; `source` is `test-pattern` or `screen`. |
| `ping` / `pong` | host → viewer → host | `{id}` every 2 s; the round trip over the peer-to-peer path. |
| `stream_stats` | host → viewer | `{fps, width, height, kbps, codec}` every 2 s: what the host sends this viewer, to compare with what arrives. |
| `client_report` | viewer → host | `{metrics: {freeze_ms, video_loss_pct, jitter_ms, hidden}}` every 2 s. The host moves the viewer's playout delay with it (go-link's `playout_delay.go`). |
| `session_end` | both | `{reason}` (host → viewer, see the reasons above), or `{}` from a viewer that hangs up. The host closes the connection after sending it. |

Later phases add `video_want`, `displays`, `display_select`, `cursor`, `clipboard`, `chat` and `permissions`. A message type nobody handles is dropped. A message is at most 64 KiB.

Two input channels, because a lost pointer move does not matter but a lost click or key does: go-link sends everything on one unreliable channel, which is fine for a gamepad that repeats its state and wrong for a keyboard (stuck keys).

## 5. Input packets (proposed, phase 3)

Big endian. Test vectors go in `protocol/vectors/`, read by the Go and the TypeScript tests.

```
pointer channel:
  0x01 abs : u16 seq | u8 display | u16 x | u16 y      x, y normalized 0..65535 over the display
  0x02 rel : u16 seq | i16 dx | i16 dy                  pointer-lock mode

input channel:
  0x10 button : u8 button (0 left, 1 middle, 2 right, 3 back, 4 forward) | u8 down | u8 display | u16 x | u16 y
                carries the position, so a click lands right even if moves were dropped
  0x20 wheel  : i16 dx | i16 dy | u8 mode (0 pixel, 1 line)
  0x30 key    : u16 keycode | u8 down | u8 mods (bit 0 shift, 1 ctrl, 2 alt, 3 meta, 4 caps lock)
                keycode = index into protocol/keycodes.json (KeyboardEvent.code)
  0x31 text   : UTF-8 bytes            IME commits and "paste as typing"
  0x3F release_all
```

- `seq` wraps at 65536; the host drops a pointer move older than the last one it applied.
- `protocol/keycodes.json` maps every `KeyboardEvent.code` to an index, and that index to the macOS virtual key code, the Windows scan code and the X11 keysym. The Go and TypeScript tables are generated from it.
- The viewer sends `release_all` on `blur`, on `visibilitychange` and when leaving full screen. The host releases everything when a viewer disconnects or stops answering.

## 6. Files (from go-link, phase 5)

- **Downloads:** `download` → `download_ready`, then `read {offset, length}` answered by binary chunks (8-byte big-endian offset, then up to 60 KB of data). The transfer ends with `done`, `cancel` or `download_error`, and the SHA-256 is checked at the end.
- **Uploads:** `begin {id, name, size}`, then 16 KB binary chunks, then `end`. The result comes back on `control` as `upload_result`. The host writes to a temporary file and renames it, and never overwrites an existing file. The browser waits on `bufferedAmount` (backpressure).
- Both depend on the `files` permission.
