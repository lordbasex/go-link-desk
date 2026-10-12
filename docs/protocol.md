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
host   → viewer {"kind":"rejected","reason":"declined"|"timeout"|"busy"|"unavailable"|"off"}
host   → viewer {"kind":"offer","sdp":"…"}                    (only after accepted; again for an ICE restart)
viewer → host   {"kind":"answer","sdp":"…"}
both            {"kind":"candidate","candidate":{…}}           (trickle ICE)
```

- The person at the host decides within 60 seconds; no answer is `rejected` with `timeout`. A host without a window has nobody to ask and answers `unavailable` at once.
- **One session at a time.** A knock while another viewer is in session, or while another knock is being asked, is `rejected` with `busy`.
- **One knock per guest.** A guest that was turned down, or whose session ended, is ignored until it leaves signalhub's session; to try again it joins again (signalhub limits `join` to 5 a minute per IP).
- The host never sends an offer, and ignores every offer, answer or candidate, from a viewer that was not accepted. A viewer never sends an offer.
- The viewer may be a browser or go-link Desk itself: the app sends `ua` as `go-link Desk 1.2 on macOS` and `codecs` `["h264-high","h264"]` (it draws H.264 with the system's decoder), and does not take a video call yet (it leaves the call's lines without a track).
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
| `idle` | Nothing came from the viewer (input, chat) for the time set in the host's Settings. |
| `revoked` | The person at the host revoked the viewer's link (it also gets `unlinked`), or a linked browser failed the password. |

## 3. Unattended access (implemented)

A browser links once with the host's **pairing code** (Settings › Unattended access shows it; it is not the session code), and from then on comes back with `reach`, without a code and without anyone accepting. Unattended access is off by default: while it is off, every knock that came through `claim` or `reach` is `rejected` with `off`.

The browser's knock and the WebRTC signals are the ones of section 2, inside signalhub's `signal` payloads; only the door and what happens on the control channel change.

**Linking (claim).**

1. `claim {app, code}` → both get `paired`; signalhub's code is good once, and the host asks for a new one at once.
2. The browser knocks. The person at the host is asked, in the same request window, whether to **link** this browser: accepting accepts this session and every later visit of this browser, with the permissions ticked now (the link's profile).
3. `accepted`, the offer, the session as in section 2. On `control`: browser `auth {}` → host `auth_ok {device_id, link_id, token}`. The browser keeps the token; the host keeps only its SHA-256, the browser's declared name and the permissions.

**Coming back (reach).**

1. `reach {app, device_id}` → the browser gets `paired`, the host `reached` (signalhub proves nothing about this browser). The browser knocks.
2. The host connects it **gated**: the offer and the answer go through, but the browser gets nothing on the connection (no picture, sound, pointer, displays, not even `welcome`) and nothing it sends reaches the session, until it is let in. It has 30 seconds to prove its token.
3. On `control`:
   1. browser → `auth_challenge {link_id, nonce}` (16 to 128 characters)
   2. host → `auth_proof {link_id, proof}`, where `proof = base64url(HMAC-SHA256(key = SHA-256(token), "go-link-desk link proof\n" + nonce))`, without padding; or `auth_failed` if it has no such link (revoked)
   3. only when the proof is right: browser → `auth {link_id, token}`
   4. host → `auth_ok` or `auth_failed` (and hangs up)
4. The unattended password (unattended access always has one): host → `password_required {}`; browser → `password {password}`; host → `password_result {ok, reason, left, retry_after}`, where `reason` is `wrong` (with `left` tries), `blocked` (this browser used its 5 tries: the host hangs up) or `locked` (20 wrong passwords in 10 minutes lock every try for 10 minutes: `retry_after` seconds). The browser has 2 minutes to type it. A `password` before `auth_ok` counts for nothing.
5. The host lets it in: `accepted {permissions}` over signalhub with the link's permissions, and from then on it is a session like any other (`welcome`, the picture, `session_end`). If another session is on, `rejected` with `busy` instead.

The host proves itself first, so a fake signaling server never learns a token. The browser ignores `password_required` and `accepted` until the proof checked out and `auth_ok` came; `accepted` travels over signalhub and may overtake `auth_ok`, so it waits for it. The vector both sides test against is `protocol/vectors/link-proof.json`.

**Revoking.** The person at the host revokes a link in Settings: its record goes from `device.json`, a browser connected with it gets `unlinked` and its session ends with `revoked`, and later its proof fails (`auth_failed`). A browser that gets `auth_failed` or `unlinked` forgets the link. A linked browser may also send `unlink {}` to forget itself on the host.

## 4. DataChannels

The host creates them, as in go-link.

| Label | Settings | Content | Status |
|---|---|---|---|
| `control` | ordered, reliable, JSON | see below | implemented |
| `pointer` | `ordered: false`, `maxRetransmits: 0`, binary | pointer moves only | implemented |
| `input` | ordered, reliable, binary | buttons, wheel, keys, text, `release_all` | implemented |
| `files` | ordered, reliable | uploads, browsing and downloads (section 6) | implemented |

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
| `notice` | host → viewer | `{code}`: `no_screen_permission` (the host may not record its screen: the viewer sees the test card), `capture_failed`, `no_accessibility` (the viewer may control, but the host has no Accessibility permission, so nothing it sends reaches the computer), `wayland_session` (a Linux Wayland desktop without the remote desktop portal or PipeWire: the test card), `share_denied` (the person at a Wayland desktop did not allow sharing: the test card), or `no_remote_input` (they shared the screen but not the mouse and keyboard: the viewer can only watch). |
| `permissions` | host → viewer | `{view, control, audio, clipboard, files}`: what the viewer may do now. Sent when the person at the computer changes it during the session; the session starts with the `accepted` message's. |
| `clipboard` | both | `{id, part, parts, text, paste?}`: one piece of a clipboard's text, at most 16 KiB of UTF-8, never splitting a character; a text is at most 1 MiB. Only with `permissions.clipboard`. Host → viewer: something new was copied on the host. Viewer → host: the viewer pasted (Cmd+V or Ctrl+V); `paste` on the last piece asks the host to paste it (Cmd+V) where its cursor is, for a viewer that may control. A piece out of order drops the text. |
| `chat` | both | `{text}`: a chat message, at most 2000 characters. The host keeps printable text and line breaks (at most 20 lines) and drops control and format characters. |
| `typing` | both | `{on}`: this side is writing a chat message. Sent again every 3 s while it writes, and `on: false` after a 4-second pause, when the line is emptied or when the message goes. The other side shows "… is typing" until it hears `false`, a message comes, or 6 s pass without news. |
| `call_request` | viewer → host | `{video, audio}`: asks for a video call, saying what the viewer will send. The person at the host takes it or not. |
| `call_state` | host → viewer | `{state, host_video, host_audio}`: `ringing`, `active`, `declined` or `ended`, and whether the host's camera and microphone are on. |
| `call_media` | viewer → host | `{video, audio}`: the viewer turned its camera or microphone on or off. |
| `call_end` | viewer → host | hangs up. |
| `session_end` | both | `{reason}` (host → viewer, see the reasons above), or `{}` from a viewer that hangs up. The host closes the connection after sending it. |

A message type nobody handles is dropped. A message is at most 64 KiB.

### Video

- **The picture's size.** The display's size in points (its pixels with **Retina**), at most the preset's width, and no larger than the viewer's window (`video_want`). A window less than a tenth smaller than the picture gets the picture as it is: shrinking text by a few pixels blurs all of it. **Quality** always sends the display's own pixels, whatever the window.

  | Preset | Width | Frames per second | Bitrate ceiling |
  |---|---|---|---|
  | Speed | 1280 | 60 | 4 Mbps |
  | Balanced (default) | 1920 | 60 | 10 Mbps |
  | Quality | the display's pixels | 60 | 20 Mbps |

- **Only when it changes.** ScreenCaptureKit hands over a frame when the picture changes; a still screen sends nothing. A few moments after it stops, the host sends the last frame again (four times, 0.4 s apart) so the encoder sharpens what it left blurry while it moved, and again for a viewer that needs a keyframe.
- **The bandwidth.** Google Congestion Control on the host, from the browser's transport-wide feedback (TWCC), estimates what each viewer's path carries, bounded by what that viewer was actually sent (a cap that starts at 4 Mbps, grows 15% a second while the encoder fills it without loss, and drops to what got through when `client_report` says 10% or more was lost); the encoder aims at 90% of it, under the preset's ceiling. Under 2.5 Mbps the session drops to 30 frames per second, under 1.5 to 1280 pixels wide, under 800 kbps to 960 at 20, under 400 to 640 at 15; it climbs back one rung at a time, once the estimate is a third above it and the last change is 10 seconds old.
- **Losses** are resent (NACK, RTX); a viewer that still cannot decode asks for a keyframe (PLI), which only its own encoder makes.

### Sound

Every viewer's offer has an Opus track (48 kHz, stereo, 96 kbps) next to its video, in the same media stream: allowing the sound later needs no new negotiation. Nothing is sent on it unless the viewer may hear (`permissions.audio`). On the screen, ScreenCaptureKit captures the computer's sound without the host's own (`excludesCurrentProcessAudio`), only while a viewer may hear it; the test card plays a 1 kHz tone. One encoder serves every viewer: the sound is the same for all.

The viewer plays it in its own `<audio>` element (the video stays muted). Browsers let a page start sound only after a click; the Connect click usually counts, otherwise the page shows a button.

### Video call

Every offer also carries a call's media, so a call starts and ends without a new negotiation: the host's camera (the viewer's screen codec) and microphone (Opus, mono) as send-only tracks in the stream `go-link-desk-call`, and two receive-only lines for the viewer's camera and microphone. The viewer answers those two send-only, with no track until a call is taken (`replaceTrack`). The host's receive line for video offers only H.264 in packetization mode 1 (Constrained Baseline first), what every browser's camera encoder sends; the host decodes it with VideoToolbox. Nothing of the viewer's camera or microphone is decoded, shown or played unless a call is active, and the host's camera and microphone are not opened until the person at the host takes it.

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

## 6. Files (implemented)

On the `files` channel (ordered, reliable): text messages are JSON, binary messages are file data. Only with `permissions.files`; one upload and one download at a time per viewer.

- **Uploads (viewer → host):** `begin {id, name, size}`, binary chunks of 16 KiB, then `end {id}` (or `abort {id}`); the host answers `upload_result {id, ok, name, error?}`, where `name` is what it saved the file as. The host writes to a temporary file in Downloads/go-link Desk and renames it when complete, never over an existing file (`name (2).ext`); a name loses any folder, control and format characters, and its leading dots. At most 20 GB. The browser keeps at most 1 MiB queued in the channel (`bufferedAmount`), so the session's other messages and the picture keep moving.
- **Browsing (the host's home folder):** `list {id, path}` → `listing {id, path, entries: [{name, dir, size, mtime}]}`, folders first, at most 5000. `path` is relative to the home folder; anything that leaves it (`..`, an absolute path, a symbolic link pointing out), or goes through a hidden file or folder or `~/Library` (also after following symbolic links), is refused with `files_error`, and those are never listed.
- **Downloads (host → viewer):** `download {id, path}` → `download_ready {id, name, size}`; then `read {id, offset, length}`, answered by binary messages (8-byte big-endian offset, then up to 60 KiB); `done` or `cancel` ends it, `download_error` reports a failure. The browser keeps 16 reads in flight. Chromium writes straight to a file the person picks (File System Access API); other browsers keep it in memory and save it at the end.
- No checksum: DTLS already authenticates every byte, and SCTP delivers them in order.
