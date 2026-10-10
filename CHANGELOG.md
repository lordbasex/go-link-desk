# Changelog

All notable changes to go-link Desk. Newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added (host)

- **The host registers with signalhub and shows a session code:** `cmd/desk` registers as `app: "go-link-desk"`, opens an invite-only room and asks for its invitation; the 9-digit code and the `/c/<invite>` link show in a minimal window (Fyne, with a tray icon, in English, Spanish and Portuguese) or, with `--headless`, in the log. "New code" replaces the invitation and the old code stops working. The room and its code are opened again after every reconnect.
- **The generic pieces of go-link's device,** adapted where they named games: the signalhub client with reconnection, pairing and linked browsers (the remembered-browser proof now signs `go-link-desk link proof`, so a proof never passes between the two apps), the PIN gate, status, CPU and RAM metrics, update checks against this repository's releases, the VP8 and Opus encoders and the VideoToolbox H.264 encoder (called directly; no ffmpeg), the VP8 payloader and playout-delay helpers, telemetry storage, and the test card without its gamepad (it shows the frame rate and the remote pointer instead).
- **Flags:** `--config`, `--server-signaling`, `--web-url`, `--headless`, `--debug`, `--version` and `--source screen|testpattern` (the source is used from the next phase on).

### Added (web)

- **desk.go-link.org's first pages:** the landing, `/c` with the code form (the code groups as `915 355 636` while typed; a pasted `/c/<invite>` link works too) and `/c/<code or invite>`, which for now only confirms the code. Strict CSP, self-hosted fonts, light and dark themes, English, Spanish and Portuguese.
- **The shared package** (`@go-link-desk/shared`): go-link's signalhub client, signaling URL helpers, code editing, pure TypeScript SHA-256 and HMAC, linked-browser storage and the design tokens, under go-link-desk names (`APP = "go-link-desk"`, `/c/` links, `go-link-desk.*` storage keys).

### Added (build and CI)

- **`make check`** runs what CI runs: gofmt, `go vet` (with and without the window), `go test -race`, govulncheck, typecheck, Vitest, the web build and gitleaks.
- **A portable macOS app** (`make device-darwin-arm64`, `-amd64`, `-universal`): macOS 13 or newer, libvpx and libopus built from source and linked in, hardened runtime with no entitlement exceptions.
- **`scripts/check-deps.sh`:** fails when a binary needs a library that does not come with the operating system (`otool -L` on macOS, `ldd` on Linux). CI checks the Linux build (Docker, Debian stable) and the macOS build, including the encoders.
- **End-to-end tests** (`make e2e`): a signalhub and a headless host of their own; the code opens the host's invite-only room, a wrong code opens nothing, and the website takes the code.
- **Cross-language test vectors** in `protocol/vectors/`, read by both the Go and the TypeScript tests (the linked-browser proof first).

### Docs

- Architecture, protocol (including the proposed input channels), security and threat model, provenance of every file taken from go-link, and the rules for signalhub proposals.
