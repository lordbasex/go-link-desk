# Contributing to go-link Desk

Thanks for wanting to help! Every contribution is welcome: a bug report, an idea, a translation, docs, tests or code.

## Ways to help

- **Try it and report what happens.** Open an [issue](https://github.com/lordbasex/go-link-desk/issues/new) with what you did, what you expected and what you saw. Your system (macOS, Windows, Linux), the browser and the version (`go-link-desk --version`) help a lot.
- **Translations:** the website's texts live in `frontend/apps/web/src/i18n/`. `en.ts` is the reference; each language is one file with the same shape (see `es.ts` and `pt.ts`). The host window's texts are in `backend-device/internal/gui/i18n_*.go`.
- **Code, docs and tests:** see below.

## Getting started

Requirements: Go (the version in `backend-device/go.mod`; `GOTOOLCHAIN=auto` fetches it), Node.js (the version in `frontend/.nvmrc`), `pkg-config`, libvpx and Opus (on macOS: `brew install libvpx opus pkg-config`; the release builds them from source and links them in).

```bash
# A signaling server of your own: signalhub, next to this repository
git clone https://github.com/lordbasex/signalhub ../signaling
make -C ../signaling up          # ws://127.0.0.1:8090/ws; the first run writes ../signaling/deploy/.env
# In that .env, add http://localhost:5180 to ALLOWED_ORIGINS (and go-link-desk to
# ALLOWED_APPS if you set a list), then run `make -C ../signaling up` again.

# The host
cd backend-device
go run ./cmd/desk --server-signaling ws://127.0.0.1:8090/ws --web-url http://localhost:5180

# The website
cd frontend
npm install
VITE_SIGNAL_URL=ws://127.0.0.1:8090/ws npm run dev    # http://localhost:5180
```

Open `http://localhost:5180/c` and type the code shown in the host window: the host asks whether to let you in, and then the browser shows the session (for now, the test card). Without a window (`--headless`) the host turns every viewer down, because nobody can accept; to try a session without one, add `--source testpattern --test-auto-accept` (tests only: every viewer gets in without asking). On macOS the host shows the real screen once it may record it (System Settings › Privacy & Security › Screen Recording; the window has a button for it). macOS ties that permission to the app's signature: sign development builds with a stable identity (`make device-darwin-arm64 CODESIGN_IDENTITY="..."`) so it survives rebuilds. To measure the real screen without someone accepting, build with `GO_TAGS=desk_test_screen` (never for a release). [docs/architecture.md](docs/architecture.md) explains how the pieces fit.

## Before you open a pull request

- **One change per pull request,** with a short description of what and why. For something big, open an issue first so we can agree on the approach.
- **Everything in the repository is in English:** code, comments, identifiers, messages, commit messages and docs. The only exceptions are the translated strings in the `i18n` files.
- **Every source file starts with the copyright line** used in the other files of its folder.
- **Run the checks** that CI runs: `make check` (gofmt, `go vet`, `go test -race`, govulncheck, typecheck, Vitest, the web build and gitleaks). `make e2e` runs the end-to-end tests.
- **Keys are listed once,** in `protocol/keycodes.json`. After changing it, regenerate both tables: `go generate ./pkg/keymap` in `backend-device` and `npm run gen:keycodes -w @go-link-desk/shared` in `frontend`. The tests fail when a table is behind.
- **Keep the docs in sync:** the developer docs (`docs/`) and `CHANGELOG.md` under `[Unreleased]`.
- **Code taken from go-link** is listed in [docs/provenance.md](docs/provenance.md) with its origin, so fixes can travel both ways. Add a line when you copy something new.
- **signalhub is not changed from here.** If go-link Desk needs something new from it, write it as a generic proposal in [docs/signalhub-proposals.md](docs/signalhub-proposals.md).

## Code of conduct

Be kind and patient. Harassment or disrespect of any kind is not welcome.

## License

By contributing you agree that your contribution is released under the [MIT license](LICENSE) of this repository.
