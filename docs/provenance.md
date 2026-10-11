# Provenance

Every file that came from [go-link](https://github.com/lordbasex/go-link), with its origin, so a fix can travel both ways. The source paths are relative to go-link's repository; all were taken at commit **`664502f`**. When you copy something new, add it here.

- **verbatim:** identical except for the Go module path (`github.com/lordbasex/go-link/backend-device` → `github.com/lordbasex/go-link-desk/backend-device`). A fix in go-link applies as is.
- **adapted:** go-link's file with changes, listed. A fix in go-link needs a look.
- **rewritten:** a new file at the same path; only the mechanism or the pattern came from go-link.

Files not listed here are go-link Desk's own: in phase 2, among others, `pkg/{capture,cursor,beacon,permission,mainloop}`, `internal/services/{video,screen_source,congestion}.go` and the session page's `LiveSession.tsx` and `latency.ts`; in phase 3, `protocol/keycodes.json`, `pkg/{keymap,inputproto,inject}`, `internal/services/input_service.go` and the viewer's `input.ts` (go-link's `pkg/input` is a gamepad's and was not used); in phase 4, `pkg/clipboard`, `internal/services/{audio,clipboard_service,chat_service,file_board}.go`, the session panel's chat, the website's `ChatPanel.tsx`, `hostClipboard.ts`, `packages/shared/src/clipboard.ts` and `scripts/deploy-web.sh`; in phase 4b, `pkg/{camera,voice,decoder}`, `internal/services/{call_service,call_tracks,call_media}.go`, the host's call window and the website's `CallPanel.tsx`; in phase 5, `internal/services/files_service.go` (its protocol follows go-link's `upload_service.go` and `download_service.go`, rewritten for a home folder and Downloads instead of the ROM library and recordings), `packages/shared/src/files.ts` and `FilesPanel.tsx`; in phase 6b, `internal/viewer` (the website's `session.ts`, `stream.ts`, `files.ts` and `input.ts` written again in Go), `pkg/remoteview`, `internal/gui/{home,prefs,remote,remote_panels}.go`, `internal/services/remotes_service.go` and `internal/repositories/remotes_file.go`.

## Host (`backend-device/`)

The same path in both repositories unless noted.

| File | How | Changes |
|---|---|---|
| `go.mod` | adapted | Module path; only the requirements Desk uses (`go mod tidy`). |
| `pkg/signalclient/{client,envelope,client_test}.go` | verbatim | |
| `pkg/ids/*`, `pkg/instancelock/*`, `pkg/jsonkeys/*`, `pkg/sysinfo/*` | verbatim | |
| `pkg/watermark/*` | verbatim | |
| `pkg/encoder/{vp8_test,opus,opus_test}.go` | verbatim | |
| `pkg/encoder/vp8.go` | adapted | `Config` gains the H.264 tuning (`Baseline`, `NoLowLatency`, `MaxQP`). |
| `pkg/encoder/{vt_darwin,vt_other}.go` | adapted | Pixel buffers in without a copy, presentation times out, Constrained High and the low-latency rate control (without it where refused), bitrate changes while encoding, whether it runs on hardware. |
| `pkg/encoder/h264.go` | rewritten | VideoToolbox only, called directly: no ffmpeg subprocess, no `kind` argument, `HasH264`. go-link's x264/ffmpeg path, `FFmpegPath` and `ProbeH264` are gone. |
| `pkg/encoder/vt_darwin_test.go` | adapted | `NewH264` without the encoder kind; checks the presentation times. |
| `pkg/testpattern/{tone,tone_test}.go` | verbatim | |
| `pkg/testpattern/pattern.go`, `pattern_test.go` | adapted | No gamepad: the bottom box shows the frame rate, and a crosshair shows the remote pointer (`Controls{PointerOn, PointerX, PointerY, Pressed, FPS}`). No dependency on `pkg/input`. The latency beacon in the top-left corner. |
| `internal/models/config.go` | adapted | `App = "go-link-desk"`, `DefaultWebURL = https://desk.go-link.org`; no ROM, room, thumbnail, panel, picture or video-quality fields. |
| `internal/models/status.go` | adapted | No library, ROM or game-room types; `SessionStatus` (room, code, invite, viewers) replaces `RoomStatus`. The code and invite are never serialized. |
| `internal/repositories/config_file.go` | adapted | `<config dir>/go-link-desk/device.json`; no migration from go-link's former name. |
| `internal/repositories/config_file_test.go` | adapted | Round trip with `language` instead of `roms_dir` and `video_quality`. |
| `internal/telemetry/*` | verbatim | |
| `internal/services/{dispatcher,ice_store,pin_gate,room_opener,routing_sender,sim_loss,playout_delay}.go` | verbatim | |
| `internal/services/pairing_service.go` | adapted | Phase 6: the browsers that came with the pairing code or `reach` go to the session (they knock like any viewer) instead of a data-only connection; their signals reach the session through the dispatcher. |
| `internal/services/{playout_delay,routing_sender,sim_loss,vp8_payloader}_test.go` | verbatim | |
| `internal/services/vp8_payloader.go` | adapted | Track stream ID `go-link-desk`. |
| `internal/services/link_service.go` | adapted | The proof signs `go-link-desk link proof\n<nonce>`. Phase 6: 30 s to authenticate, `OnAuthenticated`, `ByCode`, `LinkOf`, `SetLink` (name and permissions per link) and `Revoke`. |
| `internal/services/link_service_test.go` | adapted | `NewStatusService` without the ROM folder. |
| `internal/services/link_proof_test.go` | adapted | Reads the shared vectors in `protocol/vectors/link-proof.json`. |
| `internal/services/pin_gate_test.go` | adapted | Only the gate's own tests. The unattended password (phase 6, `password.go`) is new code that reuses the gate's limits. |
| `internal/services/services_test.go` | adapted | `NewStatusService` without the ROM folder. |
| `internal/services/status_service.go` | adapted | No ROM folder, library, game rooms, saves or video quality; `SetSession`. |
| `internal/services/metrics_service.go`, `_test.go` | adapted | `device_status` carries `session` (never its code) instead of the library and rooms. |
| `internal/services/update_service.go`, `_test.go` | adapted | Checks `lordbasex/go-link-desk` releases; user agent `go-link-desk/<version>`. |
| `internal/services/session_service.go` | rewritten | Desk's own; the room-and-invite pattern, the guest bookkeeping and the PIN-gate style of signal payloads of go-link's `internal/services/test_room_service.go`. |
| `internal/services/stream_service.go` | adapted | From go-link's `internal/services/stream_service.go`: `NewWebRTCAPI`, the offer and candidate handling, `announcedCandidates`, `logPath`, pings, `sendText` and `HandleSignal` as they were. No seats, voices, HUD, gamepads, linked-browser kind, files, recordings, H.264 subprocess or video tiers. Each viewer has its own track and VP8 encoder (per-viewer keyframes); new: ICE restart, `Hangup`, `welcome` with the host's name, `stream_stats` per viewer, `client_report` driving the playout ladder, control messages routed to `internal/controllers`. |
| `internal/services/testcard_source.go` | adapted | No tone (audio is phase 4), no pause; rests while nobody watches; frames with presentation times. |
| `internal/services/announce_test.go` | verbatim | |
| `test/stream_test.go` | adapted | go-link's Pion browser (`browserPeer`, `relay`, `multiRelay`, `waitFor`, `waitForMsg`); Desk's own cases (welcome, stream_stats, round trip, routing, hang-up, a track per viewer). |
| `internal/gui/i18n.go` | adapted | `GO_LINK_DESK_LANG` instead of `GO_LINK_LANG`. |
| `internal/gui/gui.go`, `gui_test.go`, `consent.go`, `session.go`, `i18n_es.go`, `i18n_pt.go` | rewritten | The window, the connection request, the session panel and the tray; the app-loop and tray patterns of go-link's `internal/gui/gui.go`. |
| `internal/gui/float_darwin.{m,go}`, `float_other.go` | rewritten | The Cocoa-by-window-title technique of go-link's `internal/gui/panel_darwin.m`; keeps a window floating on every Space. |
| `pkg/trayicon/trayicon.go`, `_test.go` | adapted | Verbatim plus `LivePNG`, the red icon shown during a session. |
| `cmd/desk/*`, `internal/controllers/*` | rewritten | From go-link's `cmd/device/main.go` (`run()`'s wiring, `resolveSignalURL`, `isLoopback`, `hostName`), `setup.go` (`loadConfig`, `logOutput`, `wantsHeadless`) and `ui*.go`. |
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
| `packages/shared/src/signal-url.ts` | adapted | `go-link-desk.*` storage keys. |
| `packages/shared/src/device-link.ts` | rewritten | Phase 6: a list of linked computers (name, label, favorite, last use, thumbnail) and recent sessions instead of go-link's single link; `parseAuthOk` and the validation rules are go-link's. |
| `packages/shared/src/tokens.css` | adapted | No player, test-card, arcade-frame or Willy Maker tokens. |
| `packages/shared/src/index.ts`, `package.json`, `tsconfig.json` | adapted | Only the modules above; `@go-link-desk/shared`; JSON vectors included. |
| `packages/shared/test/{signal-client,signal-url}.test.ts` | verbatim | |
| `packages/shared/test/{code-input,hmac}.test.ts` | adapted | `/c/` links, `go-link-desk.*` keys, the shared proof vectors. |
| `packages/shared/test/device-link.test.ts` | rewritten | Phase 6: the list of computers and the recent sessions. |
| `apps/web/src/components/ui/Select.tsx`, `Select.test.tsx` | verbatim | |
| `apps/web/src/i18n/index.ts` | adapted | `go-link-desk.lang`. |
| `apps/web/src/theme.ts`, `public/theme.js` | adapted | `go-link-desk.theme`. |
| `apps/web/vite.config.ts` | adapted | One site (no site/play roles), `go-link-desk-version` meta, no `wasm-unsafe-eval`. |
| `apps/web/src/styles/global.css` | adapted | go-link's base, button, field and select rules; Desk's own layout. |
| `apps/web/src/components/JoinForm.tsx` | adapted | Code or link only (no PIN, no terms); opens `/c/<target>`. |
| `apps/web/src/components/Icons.tsx` | adapted | go-link's icon conventions; Desk's icons. |
| `apps/web/{index.html,package.json,tsconfig.json}`, `src/{main.tsx,config.ts,vite-env.d.ts}` | adapted | Desk names, no splash, intro, panel, demo data or roles. |
| `apps/web/src/i18n/{en,es,pt}.ts`, `src/App.tsx`, `src/App.test.tsx` | rewritten | Desk's texts and routes; go-link's shape. |
| `packages/shared/src/stream.ts` | adapted | go-link's `HostStream`: offer, answer and candidates, the control channel with pings and pending messages, and `stats()` (plus frames decoded and size). No gamepad, microphone, voices, files or downloads; new: `knock`, `accepted`/`rejected`, `hangUp`, signals applied strictly in order, ICE restarts on the same connection. |
| `apps/web/src/components/StreamInfo.tsx` | adapted | The (i) button and panel; rows for what Desk measures (frames decoded, size, bitrate, loss), no picture renderer or refresh rate. |
| `apps/web/src/components/useFullscreen.ts` | adapted | No landscape lock or quiet full screen (a gamepad's). |
| `apps/web/src/session/{session,useSession}.ts` | rewritten | The join, backlog and host-left handling of go-link's `apps/web/src/signal/useJoinRoom.ts`, plus the knock; one signaling connection per visit. |
| `apps/web/src/session/test-utils.ts` | rewritten | A fake `SignalClient`, in the spirit of go-link's `test-utils.tsx` `FakeSocket`. |

## iOS app (`mobile/ios/`)

From go-link's player app (`mobile/ios/`), taken at commit **`bf3ca43`** (later than the rest: the player app came after `664502f`).

| File | How | Changes |
|---|---|---|
| `DeskCore/Sources/DeskCore/{Json,IceServers,CodeInput,SignalClient}.swift` | adapted | Comments only (no Android app, no rooms); `SignalClient` uses `DeskProtocol`. |
| `DeskCore/Sources/DeskCore/{Protocol,SignalUrl}.swift` | adapted | `app = "go-link-desk"`; `Envelopes.join(code:)` only (no invitations). |
| `DeskCore/Sources/DeskCore/RtcSignal.swift` | rewritten | Desk's signals: knock, accepted (with the permissions), rejected, offer, candidate. No PIN, tokens or voice. |
| `DeskCore/Tests/DeskCoreTests/{CodeInputTests,SignalUrlTests,SignalTestTests,Support}.swift` | adapted | `@testable import DeskCore`; the fake peer has the pointer channel; no invitation parser. |
| `DeskViewer/Net/WebSockets.swift` | verbatim | `import DeskCore`. |
| `DeskViewer/Rtc/DeskAudioDevice.swift` (`GoLinkAudioDevice.swift`) | adapted | Renamed; this app never turns the microphone on. |
| `DeskViewer/Rtc/DeskRtcPeer.swift` (`IOSRtcPeer.swift`) | adapted | The pointer channel; no microphone line or voices; a video call's tracks are left alone; the control channel is listened to before the main queue gets to it. |
| `DeskViewer/Rtc/WebRtcEngine.swift`, `StatsReader.swift` (`LiveStatsReader.swift`) | adapted | No microphone track; only the inbound video and the candidate pair. |
| `DeskViewer/PrivacyInfo.xcprivacy`, `Config/{Debug,Release}.xcconfig`, `.gitignore` | adapted / verbatim | Texts. |
| `project.yml`, `Makefile`, `Config/{Base,Local.xcconfig.example}.xcconfig` | adapted | Names and bundle id `org.go-link.desk.ios`; no UI tests or Universal Links yet; the device may be an iPad. |
| `scripts/asc.py` | verbatim | The App Store Connect API client (bundle id in its help text). |

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
| `internal/services/{stream_telemetry,encode_load}.go` | 8 (session telemetry, with the history) |
| `internal/services/history_service.go` | 8 (the session history, with the session telemetry); phase 6 brought the settings without it |
| `internal/services/{upload_service,download_service}.go` | 5 (files) |
| `internal/services/{recorder,recording_service}.go` | 8 (recording) |
| `frontend/apps/web/src/components/{CopyButton,QrCode}.tsx`, `screenshot.ts` | 8 (screenshots with the tools menu); the host's QR code is not needed yet: the linking code is typed |
| `internal/services/{video_tiers,quality_ladder}.go` | not taken: one viewer at a time and Google Congestion Control replace go-link's size tiers and loss ladder |
| `frontend/apps/web/src/components/ui/{PageHero,Skeleton,useInfiniteList}` | not taken: My computers is a short local list, read at once |
