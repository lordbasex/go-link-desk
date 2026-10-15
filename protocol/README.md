# protocol

The single source of truth for what the host (Go) and the website (TypeScript) must agree on byte for byte. Both test suites read these files, so a change that breaks one side fails the other's tests too.

| File | What | Read by |
|---|---|---|
| `vectors/link-proof.json` | The linked-browser proof (`auth_proof`): token, nonce and expected `proof`; `proof2`, bound to the connection's DTLS fingerprints; and how a fingerprint is read from an SDP. | `backend-device/internal/services/link_proof_test.go`, `frontend/packages/shared/test/hmac.test.ts`, `mobile/ios/DeskCore/Tests/DeskCoreTests/LinkTests.swift`, `mobile/android/core/src/test/kotlin/org/golink/desk/core/LinkTest.kt` |
| `keycodes.json` | The keyboard table: each entry's index is the keycode in the input channel's key packet. | From phase 3: the generated Go and TypeScript tables. |

## keycodes.json

Each entry of `keys` (filled in phase 3):

```json
{ "code": "KeyA", "mac": 0, "win": 30, "x11": 97 }
```

- `code`: the browser's `KeyboardEvent.code` (the key's position, not its character).
- `mac`: the macOS virtual key code (`kVK_*`).
- `win`: the Windows scan code (with `0xE0` extended keys as `0xE0xx`).
- `x11`: the X11 keysym.

Indexes are permanent: a key is never removed or moved, so an older website and a newer host still agree. The Go table is generated with `go generate` and the TypeScript one with a script, both from this file; nobody writes the tables by hand.
