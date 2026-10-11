#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Wraps a built host binary (one architecture or universal) in
# go-link-desk.app: Info.plist, the icon and the signature. Ad hoc
# signature by default; with a Developer ID, the hardened runtime and its
# entitlements (the camera and the microphone, for video calls; no code
# exceptions).
#
# macOS ties Screen Recording and Accessibility permissions to the app's
# signature and bundle ID: an ad hoc signature changes with every build, so
# development builds lose the permissions each time. Sign with a stable
# identity (a Developer ID, or a self-signed development certificate) to
# keep them.
#
# Usage: build/macos/make-app.sh BINARY APP VERSION SHORT_VERSION [IDENTITY]
set -euo pipefail

BIN="${1:?usage: make-app.sh BINARY APP VERSION SHORT_VERSION [IDENTITY]}"
APP="${2:?usage: make-app.sh BINARY APP VERSION SHORT_VERSION [IDENTITY]}"
VERSION="${3:?version}"
SHORT_VERSION="${4:?short version}"
IDENTITY="${5:--}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEVICE="$(cd "$HERE/../.." && pwd)"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/go-link-desk"
cp "$DEVICE/../LICENSE" "$DEVICE/../THIRD_PARTY_NOTICES.md" "$APP/Contents/Resources/"
sed -e "s|@VERSION@|$VERSION|" -e "s|@SHORT_VERSION@|$SHORT_VERSION|" "$HERE/Info.plist" > "$APP/Contents/Info.plist"

ICONSET="$(mktemp -d)/go-link-desk.iconset"
(cd "$DEVICE" && GOOS= GOARCH= CGO_ENABLED=0 go run ./build/macos/appicon "$ICONSET")
iconutil -c icns -o "$APP/Contents/Resources/go-link-desk.icns" "$ICONSET"
rm -rf "$(dirname "$ICONSET")"

if [[ "$IDENTITY" == "-" ]]; then
  echo "⚠ Development build: ad hoc signature (set CODESIGN_IDENTITY to sign with a stable identity)"
  codesign --force --sign - "$APP"
else
  codesign --force --options runtime --timestamp \
    --entitlements "$HERE/entitlements.plist" --sign "$IDENTITY" "$APP"
fi
echo "✓ $APP ($(lipo -archs "$APP/Contents/MacOS/go-link-desk"))"
