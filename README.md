<div align="center">

# go-link Desk

**Remote desktop from the browser.**

The computer you control runs a small portable app. Whoever controls it only needs a browser: open [desk.go-link.org](https://desk.go-link.org), type the code, and see and drive the screen. Video, audio and control travel peer to peer over WebRTC.

[![CI](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml/badge.svg)](https://github.com/lordbasex/go-link-desk/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

> Work in progress: the host registers and shows its code; sessions arrive next. See [CHANGELOG.md](CHANGELOG.md).

## How it works

- **The host** is one portable program (macOS first, then Windows and Linux): no installer, no administrator rights, nothing else to install. It shows a 9-digit code.
- **The viewer** is a browser. Type the code at desk.go-link.org; the person at the other computer accepts, and you see and control its screen.
- **Nothing in between sees the session.** The two meet through [signalhub](https://github.com/lordbasex/signalhub), the same signaling server as [go-link](https://github.com/lordbasex/go-link), and then talk peer to peer, encrypted end to end. A TURN relay is only used when there is no direct path.
- **Consent first.** No session starts unless the person at the computer accepts it (or turned on unattended access themselves), and a visible panel lets them end it at any moment.

## Docs

- [Architecture](docs/architecture.md)
- [Protocol](docs/protocol.md)
- [Security and threat model](docs/security.md)
- [Provenance of the code taken from go-link](docs/provenance.md)
- [signalhub proposals](docs/signalhub-proposals.md)
- [Contributing](CONTRIBUTING.md): how to run the host, the website and the checks

## License

MIT, see [LICENSE](LICENSE). Third-party components: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
