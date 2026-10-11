<!-- Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> -->
# Google Play

What the Play Console asks for go-link Desk, ready to paste. The developer account is a personal one, so the app goes through a closed test (at least 12 testers for 14 days in a row) before production.

## Store listing

- `listing.json`: the title (30 characters at most), the short description (80) and the full description (4000) in `en-US`, `es-419` and `pt-BR`.
- `graphics/icon-512.png`: the app icon (512 × 512).
- `graphics/feature-{en,es,pt}.png`: the feature graphic (1024 × 500), one per language.
- Screenshots: `scripts/screenshots.py` takes them on emulators (phone, 7" and 10" tablets; Home, a session and the guide; three languages) against a test host that shows a desktop picture:

  ```bash
  desk --headless --source testpattern --test-picture desktop.png --test-auto-accept --test-input-log --test-name "Studio Mac"
  python3 scripts/screenshots.py emulator-5554 phone <session code> <dir>
  ```

  A phone is shot at 1080 × 1920: Google Play takes at most 2:1.
- **Category:** Tools. **Contact email:** a public address for the listing. **Website:** https://desk.go-link.org. **Privacy policy:** https://desk.go-link.org/privacy.

## App content

**Data safety:**

| Question | Answer |
|---|---|
| Does the app collect or share any of the required user data types? | **No.** Nothing goes to the developer: there is no account, analytics or crash reporting. The picture, sound, input, clipboard, files and chat go directly between the user's devices (WebRTC, encrypted end to end); the signaling server only relays the connection setup. |
| Is all of the user data encrypted in transit? | **Yes** (DTLS-SRTP for media and SCTP over DTLS for data; WSS for signaling). |
| Can users request that their data be deleted? | Nothing is kept by the developer; on the device, recents and links are removed in the app (and with the app's data). |

Data that only moves between the user's own devices, end to end encrypted, is not "collected" in Google Play's sense.

**Content rating (IARC questionnaire):** category **Utility, Productivity, Communication or Other**.

- Violence, sexuality, language, controlled substances, gambling: **no**.
- **Users can interact or exchange content: yes.** A chat with the person at the computer the user connects to, who has accepted the session. There is no public chat, no matching with strangers, no user profiles.
- Shares the user's location with others: **no**. Digital purchases: **no**.

**Other declarations:**

- **Ads:** no. **Target audience:** 18 and over (a tool for computers; not for children).
- **Government app:** no. **Financial features:** none. **Health:** no. **News:** no.
- **Permissions:** `INTERNET`, `ACCESS_NETWORK_STATE`, `MODIFY_AUDIO_SETTINGS` (to play the computer's sound), `USE_BIOMETRIC` (to open linked computers). No camera, microphone, location, contacts or storage permission.
- **Foreground services, accessibility services:** none.

## Releases

1. `cd mobile/android && source ~/.go-link-signing/env.sh && make bundle VERSION=x.y.z`: the App Bundle, signed with the upload key (`DESK_SIGNING`). Google Play App Signing keeps the app's own key.
2. **Internal testing** (up to 100 testers, no review): upload the bundle, add the testers' emails, share the opt-in link.
3. **Closed testing:** 12 or more testers who stay opted in for 14 days; then apply for production access in the Dashboard.
4. **Production:** the same bundle, promoted.
