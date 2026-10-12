<div align="center">

# go-link Desk

**Remote desktop, peer to peer.**

The computer you control runs go-link Desk and shows a code. Whoever controls it uses a browser, the iPhone and iPad app, the Android app, or go-link Desk on their own Mac: type the code, and see and drive the screen. Video, audio and control travel peer to peer over WebRTC, encrypted end to end.

[![CI](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml/badge.svg)](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

## Get it

| Where | What |
|---|---|
| **macOS** 13 or newer (Intel and Apple silicon) | [The latest release](https://github.com/lordbasex/go-link-desk/releases/latest): `go-link-desk-<version>-macos-universal.dmg`, signed and notarized. It shares this Mac's screen and connects to other computers. |
| **Linux**, x86-64 and ARM64, glibc 2.34 or newer (Ubuntu 22.04, Debian 12, Fedora 35 or newer) | `curl -fsSL https://desk.go-link.org/install.sh \| sh`, or from [the latest release](https://github.com/lordbasex/go-link-desk/releases/latest): `.deb`, `.rpm`, Arch package, or `.tar.gz` (unpack it and run `./install.sh`, for this user, no root). It shares the screen of an X11 or Wayland desktop (GNOME, KDE Plasma and others with the remote desktop portal and PipeWire); to connect to other computers from Linux, use the browser. |
| **iPhone and iPad**, iOS and iPadOS 26 or newer | go-link Desk on the App Store: it connects to other computers (with a code, or linked ones with Face ID). |
| **Android** phones and tablets, Android 11 or newer | go-link Desk for Android ([mobile/android](mobile/android/README.md)): the iPhone and iPad app on Android (a code, or linked computers with the fingerprint, face or screen lock). |
| **Any browser** | [desk.go-link.org](https://desk.go-link.org): connect to a computer with its code, nothing to install. |

What changed in each version: [CHANGELOG.md](CHANGELOG.md). The Windows host comes next.

## How it works

- **The host** is one portable program (macOS and Linux today, Windows next): no administrator rights, nothing else to install. It shows a 9-digit code.
- **The viewer** is a browser, the iPhone and iPad app, the Android app, or go-link Desk on a Mac. Type the code; the person at the other computer accepts, and you see and control its screen.
- **Nothing in between sees the session.** The two meet through [signalhub](https://github.com/lordbasex/signalhub), the same signaling server as [go-link](https://github.com/lordbasex/go-link), and then talk peer to peer, encrypted end to end. A TURN relay is only used when there is no direct path.
- **Consent first.** No session starts unless the person at the computer accepts it (or turned on unattended access themselves), and a visible panel lets them end it at any moment.

## Docs

- [User documentation](https://desk.go-link.org/docs): installing, sharing, connecting, Linux, troubleshooting
- [Architecture](docs/architecture.md)
- [Protocol](docs/protocol.md)
- [Security and threat model](docs/security.md)
- [Provenance of the code taken from go-link](docs/provenance.md)
- [signalhub proposals](docs/signalhub-proposals.md)
- [Contributing](CONTRIBUTING.md): how to run the host, the website and the checks

## License

MIT, see [LICENSE](LICENSE). Third-party components: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
