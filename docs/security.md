# Security

Remote desktop software is a favorite tool of "tech support" scammers. go-link Desk is built so that a session can only exist with the consent of the person at the computer, is always visible while it lasts, and can be ended at any moment.

## Rules

1. **No session without consent.** Either the person at the host accepts it (a dialog with the viewer's declared name and browser, the permissions, and a 60-second timeout), or it comes from an unattended link that person enabled. Unattended access is off by default.
2. **Always visible.** While a session lasts, the tray icon turns red and a floating panel shows who is connected, the permissions (changeable live), the chat and **Disconnect**.
3. **Least privilege.** Every permission (view, control, audio, clipboard, files) is shown and can be revoked during the session. The defaults are minimal.
4. **signalhub is not trusted.** It only relays signaling. Media and input are encrypted end to end (DTLS/SRTP). For linked browsers the host proves itself first (HMAC over a nonce), and only then does the browser show its token.
5. **No third parties where sessions run.** The app's pages (`/web`, `/c/…`, `/link/…`, `/d/…`) load no third-party script and are served with a strict Content Security Policy; fonts are self-hosted everywhere. Only the public pages (`/`, `/docs`, `/privacy`) may load Google Analytics 4, and only after the visitor accepts the cookie notice. Once it is loaded, moving to an app page is a full page load (`src/analytics.ts`), so Google's script never shares a page with a remote session; there the server's CSP header, which leaves Google out, applies on top of the page's own.
6. **The macOS app runs with the hardened runtime and no code-signing exceptions** (its only entitlements are the camera and the microphone, for video calls), and is notarized for release.

## Threat model

### A malicious or compromised signaling server

It can see who connects to which room, and the session codes. It can drop or reorder messages, and inject guests.

- It cannot see or change the session: media and DataChannels are DTLS-encrypted end to end, with the fingerprints exchanged in the SDP. **Residual risk:** a server that rewrites the SDP of both sides can sit in the middle of a session it relays. Consent still applies: the person at the host sees the request and the name, and decides.
- It cannot get a linked browser's token: the host must prove it knows the token's hash first, over the encrypted DataChannel.
- It cannot start a session without the person at the host accepting (or an unattended link that person made).

### A malicious viewer

- Before it is accepted, it gets nothing: no offer, no media, no channels. Its offers, answers and candidates are ignored, and so is everything it sends after it is turned down or its session ends.
- It cannot flood the person at the host with requests: one knock per join, one request on screen at a time (others get `busy`), and signalhub limits `join` to 5 a minute per IP.
- Its declared name and browser are cut to 40 and 60 printable characters, without line breaks or control and format characters (no right-to-left overrides), so they cannot pass for the dialog's own text.
- Guessing codes: 10⁹ codes, and signalhub limits `join` to 5 attempts per minute per IP. A code only exists while a host shows it.
- Sessions with a code have no password of their own: the code changes on demand (New code), and every viewer is accepted by a person. The password is for unattended access, below.
- Its declared `name` and browser are shown as declared, never as verified, and the dialog says so.
- Input it sends is limited by the permissions granted, and stops when they are revoked. Control is off unless the person at the computer ticks it in the request, and can be taken back at any moment from the session panel; every key and button held is let go at that moment.
- Its input packets are parsed with fixed sizes and limits (text at most 1024 bytes, display indexes checked, unknown keycodes dropped); a malformed packet is ignored.

### A compromised release host or download

- The app installs an update only when the release's `SHA256SUMS` carries the update key's Ed25519 signature and the downloaded file has the hash it lists. The private key is on the release computer only, never in the repository or CI, so someone who takes over the GitHub account or the release's files cannot push an update to the apps. On Linux, root's package manager gets a copy that root made and checked against that hash, so nothing running as the user can swap the package while the password prompt waits. The Linux installer (`install.sh`) checks the same signature with OpenSSL 3 before it trusts `SHA256SUMS`, also when `GO_LINK_DESK_DOWNLOAD` points elsewhere. **Residual risk:** the release computer itself; and a first install by hand from the releases page, which the person checks against `SHA256SUMS` (the Mac's is also notarized).
- On macOS the new app must also carry a valid signature by the running app's developer team.
- A file that fails a check is deleted and nothing is installed. Updates never run in the middle of a session without the person choosing **Update**.

### The clipboard, the sound and the chat

- The clipboard is shared only with its permission, and only what is copied after it is given: a password copied before the session never leaves. Taking the permission back stops it at once.
- Clipboard and chat messages are bounded (16 KiB a piece, 1 MiB a text, 2000 characters a chat message), checked as UTF-8, and a text with a missing piece is dropped. A viewer's paste is pressed on the host only if it may also control.
- The sound is sent only with its permission: it can carry a call or a meeting on the host. The host's own sounds are left out.
- The host's log never records clipboard text or chat words.

### Files

- Files move only with their permission; taking it back stops the transfers.
- A viewer can read only the host's home folder (no `..`, no absolute paths, no symbolic links out), and never a hidden file or folder (`.ssh`, `.aws`…, and on Windows whatever has the hidden attribute), `~/Library` (keychains, cookies, mail) or, on Windows, `AppData` (the browsers' passwords and cookies, the DPAPI keys) and `NTUSER.DAT`: they are neither listed nor downloadable, also through a symbolic link with a plain name, a short name (`APPDAT~1`), trailing dots or spaces, or a stream (`AppData::$INDEX_ALLOCATION`).
- What a viewer sends lands only in Downloads, under a cleaned name, never over a file, at most 20 GB; an incomplete file is deleted.
- Every file sent or received is logged with the viewer's id, its name and size.

### The video call

- A viewer cannot turn on the host's camera or microphone: they open only when the person at the host takes a call, and close when it ends, the session ends, or they untick them.
- A viewer's camera and microphone are dropped by the host unless a call is active, so a viewer cannot push pictures or sound onto the host's screen or speakers.
- The macOS app has the camera and audio-input entitlements (the hardened runtime blocks the devices otherwise); macOS still asks the person the first time.

### The screen

- The screen is recorded only during a session: the capture starts with the first viewer and stops with the last, and macOS shows its recording indicator meanwhile.
- The latency beacon a viewer can flip is a small square drawn for four seconds in a corner of the screen; it lets clicks through and carries no information.

### Screenshots and recordings

- A viewer saves pictures and recordings of the screen on its own side (the browser, or the apps: the Mac app into Downloads, iPhone and iPad into Photos, Android into its gallery, each encoded by the device), from the picture it already gets: nothing more is sent, and the files never pass through go-link Desk's servers or the host.
- Only with the **Take screenshots and record the screen** permission, which is never ticked by default, not even when the connection request ticks everything else. A linked device keeps it only if it was ticked.
- The viewer asks first (`screenshot`, `recording` on `control`), and saves or starts recording only after the host answered yes; the host answers once the session panel shows it, in red while recording, and the main window and the tray say so. Unticking the permission stops a recording at once. The host's log keeps when each started and stopped.
- **What this does not stop:** a viewer's browser receives the picture, so a dishonest viewer can record it anyway (a screen recorder, another app, a modified page). The permission and the notice are for honest viewers, so the person at the computer always knows; whoever must not be recorded should not let that viewer see the screen.

### A host without anyone in front of it

- A host without a window (headless, or a build without the GUI) has nobody to ask, so it turns every knock down, including a browser that asks to be linked. Unattended access (below) is the only way in without a person accepting, and its links are made with a person accepting.
- `--test-auto-accept` exists for the end-to-end tests. The host refuses it unless the source is the test card, and logs a warning when it is on. Only a measurement build (`-tags desk_test_screen`, never a release: its target refuses the tag) takes it with the real screen.

### Unattended access

- Off by default; turned on in Settings, away from the main window, which never shows the pairing code.
- A browser is linked only with the pairing code **and** the person at the host accepting it, in a request window that says it is a link and that the browser will come back without asking, with the permissions ticked then. Turning unattended access off turns every linked browser down (`off`) without forgetting the links.
- Anyone may `reach` a `device_id` (signalhub proves nothing). Such a browser is connected gated: it gets no picture, sound, pointer, display list or even the computer's name, and nothing it sends reaches the session, until it proves its link; it has 30 seconds. The host proves itself first (an HMAC of the browser's nonce keyed with SHA-256(token)), so a fake signaling server or a fake host never gets a token. The proof is bound to the connection too (`proof2`, over the DTLS fingerprints both sides see), so a signaling server that rewrites the SDP and relays the real host's proof is caught; a viewer keeps that a link gave `proof2` and refuses it without from then on. **Residual risk:** a link to a host older than 1.7, until the first session after the host updates; and then the password is still needed. The browser, in turn, ignores everything a linked computer says until it proved itself and took the token: a password request before that is not shown, an acceptance before that lets nothing in, and an `unlinked`/`auth_failed` before that is ignored (so a fake signaling server cannot wipe the saved link to force a trip back to the host), so a fake host never gets the password, shows a screen of its own, or makes the browser forget a working link.
- Tokens are 32 random bytes; the host keeps only SHA-256(token) in `device.json` (mode 0600), and compares in constant time.
- **Unattended access always has a password.** It cannot be turned on without one; removing it turns it off; and a configuration without one (from before this rule) lets no linked device in. It is typed twice when it is set. The iPhone and iPad app keeps it, once typed, with the link's token in the keychain, on that device only and behind Face ID (or the device's passcode), and sends it by itself; a wrong one (changed on the computer) is forgotten and asked again. The website asks for it every time.
- The unattended password is asked after the token, over the encrypted DataChannel, never through signalhub. It is kept as an argon2id hash (never in clear, never logged), and the PIN gate's limits apply: 5 tries per browser (then it is hung up on), and 20 wrong passwords in 10 minutes lock every try for 10 minutes. A password sent before the token counts for nothing.
- Settings lists the linked browsers with their declared name, when they were linked and their last use; each one's permissions can be changed and each can be revoked. A revoked link is refused at once, also mid-session (the session ends with `revoked`).
- An unattended session is a session like any other: the red tray icon, the floating panel (which says it is unattended) and **Disconnect** are there for its whole length.
- The browser keeps its links, recent sessions and a small thumbnail of each computer's screen in its own `localStorage`; nothing of it goes to a server. Whoever uses that browser profile can use its links, as with any saved login: Forget removes one there, and Revoke on the host removes it for good.
- **Residual risk — the link token in the browser.** A link's token is a long-lived bearer credential kept in `localStorage`. Its secrecy rests on the browser's same-origin isolation and on the app's strict Content Security Policy (no third-party script on any app page), which together keep another site or an injected script from reading it; the mobile apps instead keep the token in the keychain behind Face ID. It is still readable by script running on the app's own origin (a cross-site-scripting hole or a malicious browser extension) or by someone with the browser profile's files. A stronger design — a non-extractable WebCrypto key that signs the host's challenge, so the raw token never sits readable and never leaves the browser — is planned; it changes the link handshake on the host and every client and is tracked as a follow-up. Until then: revoke a link on the host to kill it everywhere, and treat a browser profile as you would its saved logins.

### The app as a viewer

- It trusts a linked computer exactly as the browser does: the computer proves its link first, the token goes only after the proof, and a password request or an acceptance before `auth_ok` counts for nothing. Whoever cannot prove it is the linked computer gets nothing, and the session says so.
- Its links (with their tokens), recent sessions and thumbnails are in `remotes.json`, mode 0600, next to `device.json`; nothing of it goes to a server. Forget removes a link here; Revoke on the other computer removes it for good.
- This computer's clipboard goes to the other one only when the viewer presses Cmd+V there (with the clipboard permission); what the other computer copies lands here only with that permission.
- Files from the other computer land in Downloads under a name without folders, control characters or leading dots, never over an existing file; the other computer may only list its own home folder (its rules).
- Connecting to this computer's own code is refused.
- A signaling server chosen in Settings must be `wss://` outside this computer: Settings warns about `ws://`, which is not encrypted. The media and control stay end-to-end encrypted (DTLS) whatever the server.

### A remote terminal (go-link-desk-terminal)

- Nothing listens: like the app, it only connects out to signalhub and to its viewers; there is no SSH port to scan.
- Only browsers linked by someone at the computer come in: `go-link-desk-terminal link` asks in the terminal that runs it, and nobody else is ever asked (a session code is turned down). Every later visit needs the link's token, the computer's proof and the password (argon2id, the PIN gate's limits), and `link` refuses until a password is set.
- The commands' socket is the user's alone (0600, in a 0700 folder, and `SO_PEERCRED` on Linux); it never reaches the network.
- It refuses to run as root unless told (`--allow-root`). A session is the service user's shell, with that user's rights: whoever is linked can do what that user can.
- The shell's environment is rebuilt, not inherited from the service; the shell is hung up when the session ends.

### AI assistants (MCP)

- Off by default, and per computer: an assistant reaches only the linked computers whose unattended password the user saved in **Settings › Assistants**. The password lives in the system's keychain; the assistant, its model and its chat history never see it, and the activity log never records it.
- On the computer it is an ordinary linked viewer: named after the assistant, with that link's permissions and notices, the screenshot request shown, and the computer's password lockout.
- Assistants only look unless the user turns on **Let them type and run commands**; even then they type into a terminal of their own, named *assistant*, visible in the tab strip, and on a desktop their clicks and keys need the link's mouse and keyboard permission, as any viewer's. The settings are read on every call.
- A prompt injection (text on a screen or in a command's output that steers the assistant) can make an assistant with **Act** run commands as that user. The docs say so; the activity log (`assistant-activity.log`) records every call for review.
- The app edits the assistants' own settings files only when turned on, only for assistants that are installed, keeping the rest of each file, atomically. Details: [assistants.md](assistants.md).

### A port opened in the router

- The fixed UDP port and opening it in the router (Settings › Direct connection) are off by default, and opening it is a box of its own. Nothing else is ever asked of the router; the connection test only reads (its outside address).
- The port answers only ICE checks with a connection's short-lived credentials, exchanged through the signaling server for a session the person accepted, and then DTLS with the fingerprint from the same exchange (as a random port does): an open port lets nobody in by itself.
- The mapping names only this computer's own address, never another; the router is found only on the local network (a UPnP description elsewhere, or not plain HTTP, is ignored). It is leased for an hour and removed on quitting, so a crash leaves it at most an hour.

### A session left open

- Settings can end a session after 5 minutes to an hour without input from the viewer (`idle`), and lock the screen when any session ends.
- The display is kept awake only while a session is on.

### The scammer on the phone

The person at the computer is being told what to do by someone they trust.

- The first time the host shows a code, it shows a warning: nobody legitimate calls you asking for access to your computer.
- The consent dialog says plainly what the viewer will be able to do. For a session its boxes (control, sound, clipboard, files) come ticked, so whoever asks for support accepts with one click (a request to link a browser for unattended access, which is for good, comes with none ticked); the person at the computer unticks what they do not want, there or in the session panel at any time. This weighs ease against the phone scam, where the scammer wants every box ticked; the scam warning, Reject and the always-visible session panel are what stand against it. Settings › Sessions › "A request starts with: Only the screen" turns the ticks off ahead of time, chosen calmly by whoever owns the computer (for someone likely to be called by a scammer), not in the middle of a call.
- The red tray icon, the floating panel and **Disconnect** stay visible for the whole session.
- Unattended access is off by default and lives in Settings, away from the main window.

## Checks in CI

- `gitleaks` over the whole history; fake tokens in tests carry a `gitleaks:allow` comment.
- `govulncheck` for the Go module and `npm audit --omit=dev` for the website.
- `scripts/check-deps.sh` on the macOS and Linux builds: no library outside the operating system.
- `.env` files are ignored by git; only `.env.example` templates are committed.
