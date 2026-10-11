# Security

Remote desktop software is a favorite tool of "tech support" scammers. go-link Desk is built so that a session can only exist with the consent of the person at the computer, is always visible while it lasts, and can be ended at any moment.

## Rules

1. **No session without consent.** Either the person at the host accepts it (a dialog with the viewer's declared name and browser, the permissions, and a 60-second timeout), or it comes from an unattended link that person enabled. Unattended access is off by default.
2. **Always visible.** While a session lasts, the tray icon turns red and a floating panel shows who is connected, the permissions (changeable live), the chat and **Disconnect**.
3. **Least privilege.** Every permission (view, control, audio, clipboard, files) is shown and can be revoked during the session. The defaults are minimal.
4. **signalhub is not trusted.** It only relays signaling. Media and input are encrypted end to end (DTLS/SRTP). For linked browsers the host proves itself first (HMAC over a nonce), and only then does the browser show its token.
5. **No third parties on the website.** Strict Content Security Policy, no third-party scripts, no analytics, self-hosted fonts.
6. **The macOS app runs with the hardened runtime and no entitlement exceptions,** and is notarized for release.

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
- A session password (optional) is checked by the host with go-link's PIN gate: 5 tries per viewer, and 20 failures in 10 minutes lock the password path for 10 minutes. Viewers already in are not locked out.
- Its declared `name` and browser are shown as declared, never as verified, and the dialog says so.
- Input it sends is limited by the permissions granted, and stops when they are revoked. Control is off unless the person at the computer ticks it in the request, and can be taken back at any moment from the session panel; every key and button held is let go at that moment.
- Its input packets are parsed with fixed sizes and limits (text at most 1024 bytes, display indexes checked, unknown keycodes dropped); a malformed packet is ignored.

### The screen

- The screen is recorded only during a session: the capture starts with the first viewer and stops with the last, and macOS shows its recording indicator meanwhile.
- The latency beacon a viewer can flip is a small square drawn for four seconds in a corner of the screen; it lets clicks through and carries no information.

### A host without anyone in front of it

- A host without a window (headless, or a build without the GUI) has nobody to ask, so it turns every knock down. Unattended access, in phase 6, is the only way in without a person accepting.
- `--test-auto-accept` exists for the end-to-end tests. The host refuses it unless the source is the test card, and logs a warning when it is on. Only a measurement build (`-tags desk_test_screen`, never a release: its target refuses the tag) takes it with the real screen.

### A stolen linked-browser token

- The host keeps only SHA-256(token) in `device.json` (mode 0600).
- The host lists linked browsers with their name and last use, and can revoke each one; a revoked token is refused.
- An optional extra password for unattended access is checked by the host over the DataChannel (scrypt or argon2id hash, PIN gate limits), never through signalhub.

### The scammer on the phone

The person at the computer is being told what to do by someone they trust.

- The first time the host shows a code, it shows a warning: nobody legitimate calls you asking for access to your computer.
- The consent dialog says plainly what the viewer will be able to do.
- The red tray icon, the floating panel and **Disconnect** stay visible for the whole session.
- Unattended access is off by default and lives in Settings, away from the main window.

## Checks in CI

- `gitleaks` over the whole history; fake tokens in tests carry a `gitleaks:allow` comment.
- `govulncheck` for the Go module and `npm audit --omit=dev` for the website.
- `scripts/check-deps.sh` on the macOS and Linux builds: no library outside the operating system.
- `.env` files are ignored by git; only `.env.example` templates are committed.
