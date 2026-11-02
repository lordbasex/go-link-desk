# The engine's socket

What `internal/appsock` and `cmd/desk/main_lordos.go` implement and what `engine/fake.js` answers for tests. The
app has one door to the engine, `engine/index.js`: on LordOS it talks to the companion the app host started (`lord.broker.companion()` through the SDK's `companion`,
stage 2) with `engine/socket.js`, and with no connection it shows `engineGone`, never the fake. Tests ask for the fake
(`tools/harness/use-fake.js`).

## Transport

- Fd 3 inherited from `lord-server` (GO-LINK.md 3.2): one `SOCK_SEQPACKET` socket, app on one end, engine on the other. No
  listener, no path, no port. End of file on it ends the engine; the engine gone is a refusal of anyone asking
  (`ErrUnavailable`, as `NobodyToAsk` does).
- One message is one UTF-8 JSON text, **64 KB at most**; anything longer isn't delivered. Pictures don't travel here: a
  thumbnail is a file in the app's data folder, and the message has its path. On the app's side the SDK's `companion`
  carries it, and `{ companion: 'started' | 'refused' | 'ended', code }` are the system's own messages.
- Every message is an object with a `t` (type). A request that wants an answer carries a numeric `id`, and gets
  `{ "t": "res", "id": n, "data": {...} }` or `{ "t": "res", "id": n, "err": "reason" }`.
- Times are milliseconds since 1970 in UTC. Texts for people are never sent by the engine: it sends states and reasons,
  and the app words them from go-link's catalogs in the person's language.

## Engine to app: state

There is one message: `{ "t": "state", "set": { ...fields } }`. Each field given replaces that field whole; the others
stay. The first frame after connecting has every field. The app holds no state of its own beyond what's on screen.

| Field | Shape | Comes from |
|---|---|---|
| `version`, `hostname`, `localIP`, `webURL`, `signalURL` | strings | `uiOptions` |
| `signal` | `connecting`, `connected`, `disconnected` | `StatusService` |
| `session` | `null` or `{ code, invite }`, code as 9 digits | `StatusService` |
| `active` | `null` or `{ peer, name, browser, since, permissions, unattended, recordingSince, screenshots, lastScreenshot }` | `ActiveSession` |
| `pairing` | `{ code }` | `Status.Pairing` |
| `perms` | `[{ id, granted }]`, ids `screen` and `accessibility`, answered from the `screen-share` and `remote-input` system permissions | API `SHARE_STATE` |
| `config` | `{ theme, app_icon, tray_icon, signal_url, web_url, udp_port, map_udp_port, announce_ips, language, no_update_check, autostart, idle_minutes, lock_on_end, request_ticks, viewer_name, session_windows, unattended: { enabled, has_password }, assistants: { enabled, act } }`. Never the password's hash | `SettingsService` |
| `links` | `[{ id, name, created, last_seen, permissions }]` | `LinkService` |
| `devices` | `[{ id, name, label, os, last_used, favorite, thumb }]`, never keys | `RemotesService` |
| `recents` | `[{ code, name, os, at, thumb }]` | `RemotesService` |
| `assistants` | `{ clients: [{ name, registered, err }], saved: { device: true }, errs: { device: reason } }` | `mcpserver` |
| `ports` | `null` or `{ asked, open, via, outside, port }` | `PortMapper` |
| `consent` | `null` or `{ id, name, browser, link, deadline, warning }`. `warning` is true until the scam warning has been shown once | `Consent.Ask` |
| `callAsk` | `null` or `{ name, video }` | `CallConsent.AskCall` |
| `call` | `null` or `{ name, hostCamera, hostMic, viewerCamera, viewerMic }` | `CallService` |
| `chat` | `[{ fromViewer, text, at }]`, this session's | `ChatService` |
| `typing` | bool | `ChatService` |
| `transfer` | `null` or `{ upload, name, size, done, over, err }` | `FilesService` |
| `remotes` | sessions to other computers: `[{ id, door, title, status, left, host: { name, os, terminal }, perms, hud, sound, preset, retina, displays, current, chat, notes, typing, sent, tabs, frames, keyframes }]`; `frames` and `keyframes` count what arrived, every few seconds | `viewer.Session` |
| `update` | `{ state, latest, url }`; `state`: `idle`, `checking`, `current`, `available`, `error`. Never `installing`: the updater is off, and APT updates | none on LordOS, see below |
| `netTest` | `null` or `running` | `network.test` |
| `engineGone` | set only by the app: the engine isn't there, so nobody can come in | `engine/index.js` |

A list too long for a frame loses its oldest elements, never the frame. A field's value that hasn't changed isn't sent.

`remotes[].status` is `viewer.Status` as a word: `connecting`, `waiting`, `password`, `wrong`, `checking`, `live`,
`reconnecting`, `timeout`, `busy`, `unavailable`, `off`, `declined`, `ended`, `ended_host`, `ended_viewer`,
`ended_network`, `ended_signal`, `ended_idle`, `ended_revoked`, `notfound`, `ratelimited`, `unreachable`, `unlinked`,
`offline`, `untrusted`, `self`.
`remotes[].notes` is the list of `onNotice` codes: `no_screen_permission`, `no_accessibility`, `capture_failed`, `share_denied`, `no_remote_input`.

## App to engine

| `t` | Fields | Does |
|---|---|---|
| `code.new` | | a new session code |
| `session.end` | | disconnects the viewer |
| `session.permissions` | `permissions` | what the viewer may do |
| `consent.answer` | `consent`, `accept`, `permissions` | answers the request |
| `chat.send`, `chat.typing` | `text`; `on` | the session's chat |
| `call.answer`, `call.set`, `call.end` | `accept`, `camera`, `mic` | the call |
| `settings.set` | `key`, `value` | one setting, or `password` (`"pw\nagain"`), `password_clear`, `server_default`, `relaunch`, `assistant_pass:<device>`, `assistant_forget:<device>`. A refusal comes back as `res.err` |
| `link.set`, `link.revoke` | `link`, `what`, `on` | a linked device |
| `perm.allow` | `perm` | asks the system's permission |
| `connect` | `kind` (`join`, `claim`), `code` | a session by code |
| `demo` | | go-link's demo computer |
| `device.connect`, `device.favorite`, `device.forget`, `device.rename` | `device`, `label` | My computers |
| `recent.connect`, `recent.remove`, `recent.clear` | `code` | Recent sessions |
| `remote.close`, `remote.password`, `remote.chat`, `remote.typing` | `remote`, `password`, `text`, `on` | a session to another computer |
| `remote.tool` | `remote`, `tool` (`info`, `sound`, `preset`, `retina`, `display`, `fullscreen`), `value` | its toolbar |
| `remote.keys`, `remote.typeClipboard` | `remote`, `keys`; `text`, the app's clipboard: the engine has none outside a share | its Keys menu |
| `remote.pointer`, `remote.button`, `remote.wheel`, `remote.key` | `remote`, `x`, `y` (0 to 1), `inside`; `button`, `down`; `dx`, `dy`, `lines`; `keycode`, `down`, `repeat`, `mods` | its input |
| `files.list` (asks), `files.download` (asks), `files.upload` (asks), `files.reveal` | `remote`, `path`, `name` | see Files |
| `update.check`, `network.test` | | see below |
| `quit` | | stops the engine |

## Files

The maintainer's rule, for this computer's visitors and for the computers this one visits: **a visitor sees only the
user's home folder, nothing above it, and files sent to the computer are saved in `~/Downloads`**, as in go-link's other
clients.

- `path` in `files.list` and `files.download` is relative to the home folder: `""` is the home folder itself. The app
  shows it as `~/`, and its back button stops there.
- The engine refuses `res.err = "outside_home"` for an absolute path, a `..` component, a `~`, or a symbolic link that
  leads out of the home folder. The check belongs to the engine that owns the files, not to the viewer: a viewer that
  asks for more gets the same refusal.
- `files.upload` has no destination field: the sender can't choose. It lands in `~/Downloads` with go-link's own
  rename on a clash, and the answer is `{ saved }`, the name there.
- `files.download` saves in the viewer's own `~/Downloads`.
- Enforced twice in the engine: `services.CheckHomePath` before a request leaves or is served, `services.FilesGuard` in
  front of the file service, so a listing doesn't show links out of the home folder and a `~/Downloads` that is a link
  out of it takes nothing. `files.upload` takes `path`, the file on this side, relative to this home folder, under the
  same rule; without it, `name`.
- The sandbox grants the file service no more than this (GO-LINK.md D10): reads of the home's visible entries as they were at
  start, write in `~/Downloads` and the app's data folder. The home itself is listed through fd 5. go-link's own filter of
  hidden files stays, at every level of the path.

## What the app does on its own

Copying to the clipboard (`clipboard.set` from the SDK). Check for Updates opens the Store and `update.check` exists
only to keep the update window's states, since `internal/updater` isn't built under `lordos`. `network.test`, asked
with an `id`, runs `netprobe` and answers `{ url }`, the page's address, for the app to open.

## Refusals

`res.err` is one of a closed list: `outside_home`, `no_session`, `no_files`, `too_late` (a consent or call answer with
nothing waiting), `bad_code`, `not_found`, `demo_offline`, `empty`, `unknown`, `bad_value`, `bad_permissions`,
`bad_server`, `bad_website`, `bad_port`, `bad_address`, `password_first`, `password_length`, `password_mismatch`,
`not_available` (`autostart`, `assistant_pass:`, `assistant_forget:`: D4 and D9), `system_asks` (`perm.allow`: the
API has no way to ask but a share, so the system's prompt comes with the first one).

A visitor who knocks while the session is taken (5673, or another app) is turned down as `in_use`, with nobody asked
(`SocketConsent.Ready`, GO-LINK.md section 5). With the API gone or the session locked, it's `unavailable`.

## Not defined yet

The picture and sound of a session to another computer (`MediaView`, GO-LINK.md 7) and a terminal's output need a
stream of their own and don't fit 64 KB frames. The engine is pure Go, with no decoder, and the system has no API that
takes encoded frames from an app: frames are counted and dropped, and today's views show a still. So are call sound and
the camera: the engine has no audio or camera device, and a call opens with neither.
