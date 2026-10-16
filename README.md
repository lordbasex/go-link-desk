<div align="center">

# go-link Desk

**Remote desktop, peer to peer.**

The computer you control runs go-link Desk and shows a code. Whoever controls it uses a browser, the iPhone and iPad app, the Android app, or go-link Desk on their own Mac: type the code, and see and drive the screen. Video, audio and control travel peer to peer over WebRTC, encrypted end to end. A Raspberry Pi or a server without a desktop gets its terminals the same way, and AI assistants (Claude, Claude Code, Cursor) can use the computers you link, through MCP.

[![CI](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml/badge.svg)](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

## Get it

| Where | What |
|---|---|
| **macOS** 13 or newer (Intel and Apple silicon) | [The latest release](https://github.com/lordbasex/go-link-desk/releases/latest): `go-link-desk-<version>-macos-universal.dmg`, signed and notarized. It shares this Mac's screen and connects to other computers. |
| **Windows** 10 and 11, x86-64 and ARM64 | From [the latest release](https://github.com/lordbasex/go-link-desk/releases/latest): `go-link-desk-<version>-windows-<arch>-setup.exe`, which installs for this user with no administrator rights, or the same program in a portable `.zip`. Until the first release signed through SignPath (see the [code signing policy](docs/code-signing.md)), Windows SmartScreen asks once (More info, Run anyway). It shares this PC's screen; Connect opens the other computers in the browser. |
| **Linux**, x86-64 and ARM64, glibc 2.34 or newer (Ubuntu 22.04, Debian 12, Fedora 35 or newer) | `curl -fsSL https://desk.go-link.org/install.sh \| sh` (it checks the release's signature with OpenSSL 3), or from [the latest release](https://github.com/lordbasex/go-link-desk/releases/latest): `.deb`, `.rpm`, Arch package, or `.tar.gz` (unpack it and run `./install.sh`, for this user, no root). It shares the screen of an X11 or Wayland desktop (GNOME, KDE Plasma and others with the remote desktop portal and PipeWire); to connect to other computers from Linux, use the browser. |
| **A Raspberry Pi or a server without a desktop**, Linux x86-64, ARM64 and ARMv7, any distribution | `curl -fsSL https://desk.go-link.org/install.sh \| sh -s -- --terminal` installs **go-link-desk-terminal**: a remote shell in the browser or the Mac app (no SSH, nothing listening): terminals with names and tabs that keep running between sessions, like screen, files both ways, and recordings as video or `.cast`, for devices linked there with a password. See [docs/terminal.md](docs/terminal.md). |
| **iPhone and iPad**, iOS and iPadOS 26 or newer | go-link Desk on the App Store: it connects to other computers (with a code, or linked ones with Face ID). |
| **Android** phones and tablets, Android 11 or newer | go-link Desk for Android ([mobile/android](mobile/android/README.md)): the iPhone and iPad app on Android (a code, or linked computers with the fingerprint, face or screen lock). |
| **Any browser** | [desk.go-link.org](https://desk.go-link.org): connect to a computer with its code, nothing to install. |

What changed in each version: [CHANGELOG.md](CHANGELOG.md).

## How it works

- **The host** is one portable program (macOS, Windows and Linux): no administrator rights, nothing else to install. It shows a 9-digit code.
- **The viewer** is a browser, the iPhone and iPad app, the Android app, or go-link Desk on a Mac. Type the code; the person at the other computer accepts, and you see and control its screen.
- **Nothing in between sees the session.** The two meet through [signalhub](https://github.com/lordbasex/signalhub), the same signaling server as [go-link](https://github.com/lordbasex/go-link), and then talk peer to peer, encrypted end to end. A TURN relay is only used when there is no direct path.
- **Raspberry Pi and servers.** go-link-desk-terminal, a small CGO-free service for a computer without a desktop, gives linked browsers and the Mac app its shells, with tabs that outlive the session.
- **AI assistants (MCP).** The app is also an MCP server (`go-link-desk mcp`): Settings › Assistants adds it to Claude, Claude Code and Cursor, so they can run commands on your linked Raspberry Pis and servers, see and use your desktops (clicks, keys, text, clipboard), and move files. You pick the computers (their passwords stay in the system's keychain), commands are off until you allow them, and every call is logged. See [docs/assistants.md](docs/assistants.md) and [the guide](https://desk.go-link.org/docs/assistants).
- **Consent first.** No session starts unless the person at the computer accepts it (or turned on unattended access themselves), and a visible panel lets them end it at any moment.

## Docs

- [User documentation](https://desk.go-link.org/docs): installing, sharing, connecting, Linux, troubleshooting
- [Architecture](docs/architecture.md)
- [go-link-desk-terminal: Raspberry Pi and servers](docs/terminal.md)
- [AI assistants (MCP)](docs/assistants.md)
- [Network: ports, relay and router](docs/network.md)
- [Protocol](docs/protocol.md)
- [Security and threat model](docs/security.md)
- [Code signing policy](docs/code-signing.md): how the Windows files are signed (SignPath Foundation)
- [Provenance of the code taken from go-link](docs/provenance.md)
- [signalhub proposals](docs/signalhub-proposals.md)
- [Contributing](CONTRIBUTING.md): how to run the host, the website and the checks

## Code signing policy

Windows files: free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org). Roles, build and privacy: [docs/code-signing.md](docs/code-signing.md).

## License

MIT, see [LICENSE](LICENSE). Third-party components: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
