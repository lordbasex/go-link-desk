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
viewer → host   {"kind":"knock","name":"Ana","ua":"Chrome 141 on macOS","codecs":["h264-high","h264","vp8"]}
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
- `codecs` are the video codecs the browser decodes (`RTCRtpReceiver.getCapabilities`): `h264-high` (Constrained High, profile `640c`), `h264` (Constrained Baseline) and `vp8`. The host sends H.264 from the system's encoder when it has one and the browser takes it, High when it can; otherwise VP8. Without the field, VP8.
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

The host creates them, as in go-link. `control`, `pointer` and `input` are implemented; `files` comes in its phase.

| Label | Settings | Content | Status |
|---|---|---|---|
| `control` | ordered, reliable, JSON | see below | implemented |
| `pointer` | `ordered: false`, `maxRetransmits: 0`, binary | pointer moves only | implemented |
| `input` | ordered, reliable, binary | buttons, wheel, keys, text, `release_all` | implemented |
| `files` | ordered, reliable | go-link's file protocol (section 6) | proposed, phase 5 |

### `control` messages

| Message | Direction | Content |
|---|---|---|
| `welcome` | host → viewer | `{host: {name, os}, source}` when the channel opens; `source` is `test-pattern` or `screen`. |
| `displays` | host → viewer | `{displays: [{id, name, width, height, points_width, points_height, primary}], current}`: the host's screens (pixels, and points: the desktop's coordinates) and the one shown. Sent when the channel opens and after every `display_select`. Empty for the test card. |
| `ping` / `pong` | host → viewer → host | `{id}` every 2 s; the round trip over the peer-to-peer path. |
| `stream_stats` | host → viewer | Every 2 s: `{fps, width, height, kbps, codec, target_kbps, estimate_kbps}`: what the host sent this viewer in the last window (a still screen sends nothing, and says so), the bitrate its encoder aims at, and what the congestion control estimates the path carries. |
| `client_report` | viewer → host | `{metrics: {freeze_ms, video_loss_pct, jitter_ms, hidden}}` every 2 s. The host moves the viewer's playout delay with it (go-link's `playout_delay.go`). |
| `video_want` | viewer → host | `{width, height}`: the size the viewer shows the picture at, in device pixels (on resize, debounced). |
| `quality` | viewer → host | `{preset: "speed" \| "balanced" \| "quality", retina}`. See *Video* below. |
| `display_select` | viewer → host | `{id}`: show another display. Same track, no renegotiation; a keyframe follows. |
| `cursor_mode` | viewer → host | `{in_video}`: draw the pointer into the picture (`true`) instead of sending it. |
| `cursor` | host → viewer | `{id, png, hot_x, hot_y, width, height, pixels}`: the pointer's image (PNG in base64, up to twice its size in points), its hot spot in the image's pixels, its size on screen in points, and the image's width in pixels. A viewer gets each image once; after that `png` is left out and the `id` is enough. |
| `cursor_pos` | host → viewer | `{x, y, visible}`: where the pointer is, 0..1 over the display shown; up to 60 a second, only when it moves. |
| `beacon` | viewer → host | `{seq}`: the latency beacon, white for an odd `seq`, black for an even one. See *Latency* below. |
| `notice` | host → viewer | `{code}`: `no_screen_permission` (the host may not record its screen: the viewer sees the test card), `capture_failed`, or `no_accessibility` (the viewer may control, but the host has no Accessibility permission, so nothing it sends reaches the computer). |
| `permissions` | host → viewer | `{view, control}`: what the viewer may do now. Sent when the person at the computer changes it during the session; the session starts with the `accepted` message's. |
| `session_end` | both | `{reason}` (host → viewer, see the reasons above), or `{}` from a viewer that hangs up. The host closes the connection after sending it. |

Later phases add `clipboard` and `chat`. A message type nobody handles is dropped. A message is at most 64 KiB.

### Video

- **The picture's size.** The display's size in points (its pixels with **Retina**), at most the preset's width, and no larger than the viewer's window (`video_want`). A window less than a tenth smaller than the picture gets the picture as it is: shrinking text by a few pixels blurs all of it. **Quality** always sends the display's own pixels, whatever the window.

  | Preset | Width | Frames per second | Bitrate ceiling |
  |---|---|---|---|
  | Speed | 1280 | 60 | 4 Mbps |
  | Balanced (default) | 1920 | 60 | 10 Mbps |
  | Quality | the display's pixels | 60 | 20 Mbps |

- **Only when it changes.** ScreenCaptureKit hands over a frame when the picture changes; a still screen sends nothing. A few moments after it stops, the host sends the last frame again (four times, 0.4 s apart) so the encoder sharpens what it left blurry while it moved, and again for a viewer that needs a keyframe.
- **The bandwidth.** Google Congestion Control on the host, from the browser's transport-wide feedback (TWCC), estimates what each viewer's path carries; the encoder aims at 90% of it, under the preset's ceiling. Under 2.5 Mbps the session drops to 30 frames per second, under 1.5 to 1280 pixels wide, under 800 kbps to 960 at 20, under 400 to 640 at 15; it climbs back a rung once the estimate is a third above it.
- **Losses** are resent (NACK, RTX); a viewer that still cannot decode asks for a keyframe (PLI), which only its own encoder makes.

### Latency

The viewer flips the beacon and times, with `requestVideoFrameCallback`, how long until the top-left corner of its video turns white (or black): capture, encoder, network, decoder and display. Half the control channel's round trip (the flip's own trip) is taken out, and the median of seven flips is shown. On the screen, the host shows a 32-point square in the top-left corner of the display shown, above everything and letting clicks through, for four seconds after the last flip; the test card draws it itself.

## 5. Input packets (implemented)

Big endian. The test vectors in `protocol/vectors/input.json` are read by the Go and the TypeScript tests, so both sides encode and decode the same bytes.

```
pointer channel (unordered, no retransmits):
  0x01 abs : u16 seq | u8 display | u16 x | u16 y      x, y normalized 0..65535 over the display
  0x02 rel : u16 seq | i16 dx | i16 dy                  pointer lock ("game mode")

input channel (ordered, reliable):
  0x10 button : u8 button (0 left, 1 middle, 2 right, 3 back, 4 forward) | u8 down | u8 display | u16 x | u16 y
                carries the position, so a click lands right even if moves were dropped;
                display 0xFF: no position, act where the pointer is (pointer lock, or a
                release outside the picture)
  0x20 wheel  : i16 dx | i16 dy | u8 mode (0 pixel, 1 line)    the browser's sign: dy > 0 scrolls the content down
  0x30 key    : u16 keycode | u8 down (0 up, 1 down, 2 the keyboard's repeat) | u8 mods
                mods: bit 0 shift, 1 ctrl, 2 alt, 3 meta, 4 caps lock
                keycode = index into protocol/keycodes.json (KeyboardEvent.code)
  0x31 text   : UTF-8 bytes, at most 1024           IME commits and "Type text…"
  0x3F release_all
```

- `display` is the display's index in the `displays` message.
- `seq` wraps at 65536; the host drops a pointer move older than the last one it applied. The viewer sends at most one move a frame.
- **Keys go by position** (`KeyboardEvent.code`), not by character: the host's own layout makes the characters, as if the viewer's keyboard were plugged into it. `protocol/keycodes.json` lists every code once; its index is the wire keycode, with the macOS virtual key code, the Windows scan code and the X11 keysym (`-1`: none). `pkg/keymap` and `packages/shared/src/keycodes.gen.ts` are generated from it, and their tests fail when they fall behind.
- A key comes with the modifiers held on the viewer's keyboard: the host applies them even if their own presses never arrived (a Shift held before the page had the focus). Caps Lock is the host's own and is not applied.
- Text the viewer types out (a paste, an IME's commit) goes as `text`: the host types it character by character, whatever its layout.
- **Nothing stays stuck.** The viewer sends `release_all` when the page loses the focus, is hidden, leaves full screen or loses pointer lock. On macOS browsers a key pressed with Cmd held never reports its release, so the viewer lets go of those keys when Cmd goes up. The host lets go of every key and button it pressed when the viewer asks, loses the control permission, hangs up, or its connection stops answering.
- The host acts only on packets from a viewer that may control (`permissions.control`), and only with the Accessibility permission.

## 6. Files (from go-link, phase 5)

- **Downloads:** `download` → `download_ready`, then `read {offset, length}` answered by binary chunks (8-byte big-endian offset, then up to 60 KB of data). The transfer ends with `done`, `cancel` or `download_error`, and the SHA-256 is checked at the end.
- **Uploads:** `begin {id, name, size}`, then 16 KB binary chunks, then `end`. The result comes back on `control` as `upload_result`. The host writes to a temporary file and renames it, and never overwrites an existing file. The browser waits on `bufferedAmount` (backpressure).
- Both depend on the `files` permission.
