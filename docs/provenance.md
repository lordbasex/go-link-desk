# Provenance

Every file that came from [go-link](https://github.com/lordbasex/go-link), with its origin, so a fix can travel both ways. The source paths are relative to go-link's repository; all were taken at commit **`664502f`**. When you copy something new, add it here.

- **verbatim:** identical except for the Go module path (`github.com/lordbasex/go-link/backend-device` → `github.com/lordbasex/go-link-desk/backend-device`). A fix in go-link applies as is.
- **adapted:** go-link's file with changes, listed. A fix in go-link needs a look.
- **rewritten:** a new file at the same path; only the mechanism or the pattern came from go-link.

Files not listed here are go-link Desk's own.

## Host (`backend-device/`)

The same path in both repositories unless noted.

| File | How | Changes |
|---|---|---|
| `go.mod` | adapted | Module path; only the requirements Desk uses (`go mod tidy`). |
| `pkg/signalclient/{client,envelope,client_test}.go` | verbatim | |
| `pkg/ids/*`, `pkg/instancelock/*`, `pkg/jsonkeys/*`, `pkg/sysinfo/*` | verbatim | |
| `pkg/trayicon/*`, `pkg/watermark/*` | verbatim | The brand mark changes with the product's icon, in a later phase. |
| `pkg/encoder/{vp8,vp8_test,opus,opus_test,vt_darwin,vt_other}.go` | verbatim | |
| `pkg/encoder/h264.go` | rewritten | VideoToolbox only, called directly: no ffmpeg subprocess, no `kind` argument, `HasH264`. go-link's x264/ffmpeg path, `FFmpegPath` and `ProbeH264` are gone. |
| `pkg/encoder/vt_darwin_test.go` | adapted | `NewH264` without the encoder kind. |
| `pkg/testpattern/{tone,tone_test}.go` | verbatim | |
| `pkg/testpattern/pattern.go`, `pattern_test.go` | adapted | No gamepad: the bottom box shows the frame rate, and a crosshair shows the remote pointer (`Controls{PointerOn, PointerX, PointerY, Pressed, FPS}`). No dependency on `pkg/input`. |
| `internal/models/config.go` | adapted | `App = "go-link-desk"`, `DefaultWebURL = https://desk.go-link.org`; no ROM, room, thumbnail, panel, picture or video-quality fields. |
| `internal/models/status.go` | adapted | No library, ROM or game-room types; `SessionStatus` (room, code, invite, viewers) replaces `RoomStatus`. The code and invite are never serialized. |
| `internal/repositories/config_file.go` | adapted | `<config dir>/go-link-desk/device.json`; no migration from go-link's former name. |
| `internal/repositories/config_file_test.go` | adapted | Round trip with `language` instead of `roms_dir` and `video_quality`. |
| `internal/telemetry/*` | verbatim | |
| `internal/services/{dispatcher,ice_store,pairing_service,pin_gate,room_opener,routing_sender,sim_loss,playout_delay}.go` | verbatim | |
| `internal/services/{playout_delay,routing_sender,sim_loss,vp8_payloader}_test.go` | verbatim | |
| `internal/services/vp8_payloader.go` | adapted | Track stream ID `go-link-desk`. |
| `internal/services/link_service.go` | adapted | The proof signs `go-link-desk link proof\n<nonce>`. |
| `internal/services/link_service_test.go` | adapted | `NewStatusService` without the ROM folder. |
| `internal/services/link_proof_test.go` | adapted | Reads the shared vectors in `protocol/vectors/link-proof.json`. |
| `internal/services/pin_gate_test.go` | adapted | Only the gate's own tests; the test-room tests move to the session service (phase 1). |
| `internal/services/services_test.go` | adapted | `NewStatusService` without the ROM folder. |
| `internal/services/status_service.go` | adapted | No ROM folder, library, game rooms, saves or video quality; `SetSession`. |
| `internal/services/metrics_service.go`, `_test.go` | adapted | `device_status` carries `session` (never its code) instead of the library and rooms. |
| `internal/services/update_service.go`, `_test.go` | adapted | Checks `lordbasex/go-link-desk` releases; user agent `go-link-desk/<version>`. |
| `internal/services/session_service.go` | rewritten | Desk's own; the room-and-invite pattern of go-link's `internal/services/test_room_service.go`. |
| `internal/gui/i18n.go` | adapted | `GO_LINK_DESK_LANG` instead of `GO_LINK_LANG`. |
| `internal/gui/gui.go`, `gui_test.go`, `i18n_es.go`, `i18n_pt.go` | rewritten | A minimal window and tray; the app-loop and tray patterns of go-link's `internal/gui/gui.go`. |
| `cmd/desk/*` | rewritten | From go-link's `cmd/device/main.go` (`run()`'s wiring, `resolveSignalURL`, `isLoopback`, `hostName`), `setup.go` (`loadConfig`, `logOutput`, `wantsHeadless`) and `ui*.go`. |
| `build/macos/static-libs.sh` | adapted | `MIN_MACOS` 13.0. |
| `build/macos/make-app.sh` | adapted | `go-link-desk.app` and binary; no go-link HD engine library. |
| `build/macos/Info.plist` | adapted | `org.go-link.desk`, go-link Desk names, macOS 13, utilities category. |
| `build/macos/entitlements.plist` | rewritten | No exceptions (go-link's `disable-library-validation` was only for the libretro cores). |
| `build/macos/{make-dmg.sh,notarize.sh,dmg-background.swift}` | adapted | go-link Desk names. |
| `build/macos/appicon/main.go` | adapted | `go-link-desk.iconset`. |
| `build/linux.Dockerfile` | adapted | Builds `./cmd/desk` as `go-link-desk`; no go-link HD stage. |

## Website (`frontend/`)

| File | How | Changes |
|---|---|---|
| `tsconfig.base.json`, `vitest.setup.ts`, `.nvmrc` | verbatim | |
| `package.json` | adapted | Workspaces `packages/*` and `apps/*` only; no coverage package. |
| `vitest.config.mts` | adapted | No Willy Maker; allows `../protocol` for the shared vectors. |
| `packages/shared/src/{pairing-code,signal-client}.ts` | verbatim | |
| `packages/shared/src/protocol.ts` | adapted | `APP = "go-link-desk"`; invitation links are `/c/<invite>`. |
| `packages/shared/src/hmac.ts` | adapted | `linkProof` signs `go-link-desk link proof`; no `panelProof` (Desk has no local web panel). |
| `packages/shared/src/{device-link,signal-url}.ts` | adapted | `go-link-desk.*` storage keys. |
| `packages/shared/src/tokens.css` | adapted | No player, test-card, arcade-frame or Willy Maker tokens. |
| `packages/shared/src/index.ts`, `package.json`, `tsconfig.json` | adapted | Only the modules above; `@go-link-desk/shared`; JSON vectors included. |
| `packages/shared/test/{signal-client,signal-url}.test.ts` | verbatim | |
| `packages/shared/test/{code-input,device-link,hmac}.test.ts` | adapted | `/c/` links, `go-link-desk.*` keys, the shared proof vectors. |
| `apps/web/src/components/ui/Select.tsx`, `Select.test.tsx` | verbatim | |
| `apps/web/src/i18n/index.ts` | adapted | `go-link-desk.lang`. |
| `apps/web/src/theme.ts`, `public/theme.js` | adapted | `go-link-desk.theme`. |
| `apps/web/vite.config.ts` | adapted | One site (no site/play roles), `go-link-desk-version` meta, no `wasm-unsafe-eval`. |
| `apps/web/src/styles/global.css` | adapted | go-link's base, button, field and select rules; Desk's own layout. |
| `apps/web/src/components/JoinForm.tsx` | adapted | Code or link only (no PIN, no terms); opens `/c/<target>`. |
| `apps/web/src/components/Icons.tsx` | adapted | go-link's icon conventions; Desk's icons. |
| `apps/web/{index.html,package.json,tsconfig.json}`, `src/{main.tsx,config.ts,vite-env.d.ts}` | adapted | Desk names, no splash, intro, panel, demo data or roles. |
| `apps/web/src/i18n/{en,es,pt}.ts`, `src/App.tsx`, `src/App.test.tsx` | rewritten | Desk's texts and routes; go-link's shape. |

## Build, CI and tests

| File | How | Changes |
|---|---|---|
| `Makefile` | adapted | `check`, no website roles, panel, Willy Maker, Android or go-link HD; check-deps after each macOS build. |
| `scripts/third-party-notices.sh` | adapted | `./cmd/desk`; no go-link HD or Musashi. |
| `.github/workflows/ci.yml` | adapted | No mp4-wasm, Android or iOS jobs; `deps-linux` and `deps-macos` added. |
| `e2e/{global-setup,global-teardown,playwright.config,ports,stack}.ts`, `e2e/package.json` | adapted | One host, one site, Desk ports (8291, 5291); `ALLOWED_APPS=go-link-desk`; the session code instead of the panel. |
| `.gitignore`, `LICENSE` | adapted / verbatim | |

## Deferred to the phase that uses them

Planned in phase 0, moved because nothing uses them yet:

| go-link file | Phase |
|---|---|
| `internal/services/{stream_service,video_tiers,stream_telemetry,encode_load,quality_ladder}.go` | 1 (they are `StreamService` methods) |
| `internal/services/history_service.go` | 1 (session history instead of game history) |
| `internal/services/{upload_service,download_service}.go` | 5 (files) |
| `internal/services/{recorder,recording_service}.go` | 8 (recording) |
| `frontend/apps/web/src/components/{CopyButton,QrCode,useFullscreen,StreamInfo}.tsx`, `screenshot.ts` | 1 (session page) |
| `frontend/apps/web/src/components/ui/{PageHero,Skeleton,useInfiniteList}` | 6 (my computers) |
