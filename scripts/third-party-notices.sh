#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Writes THIRD_PARTY_NOTICES.md: the license of every third-party component
# that go-link Desk's binaries and website include (Go modules, the Go
# standard library, libvpx, libopus, the Linux host's Wayland clipboard
# protocol, npm packages and fonts). Libraries the Linux host only loads
# from the system at run time (libpipewire, libpulse-simple) are not
# included, so they are not listed. The BSD, MIT,
# Apache and OFL licenses ask for their notices to travel with the
# binaries, so the release packs this file next to them.
#
# Usage: scripts/third-party-notices.sh   (run after npm ci in frontend/)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/THIRD_PARTY_NOTICES.md"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Must match backend-device/build/macos/static-libs.sh.
OPUS_VERSION=1.6.1
VPX_VERSION=1.17.0
# Must match backend-device/build/windows.Dockerfile (the Linux build
# links Debian stable's, under the same license).
YUV_VERSION=0.0.1974.20261008

license_file() { # dir
  find "$1" -maxdepth 1 -type f \( -iname 'licen[cs]e*' -o -iname 'copying*' -o -iname 'ofl.txt' \) | sort | head -1
}

mit() { # copyright line
  cat <<MIT
$1

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
MIT
}

section() { # name version url file
  {
    printf '\n## %s %s\n\n' "$1" "$2"
    [[ -n "$3" ]] && printf '%s\n\n' "$3"
    printf '```\n'
    cat "$4"
    printf '\n```\n'
  } >> "$OUT"
}

cat > "$OUT" <<'EOF'
# Third-party notices

go-link Desk's own code is under the MIT license (see `LICENSE`). Its
binaries and website also include the third-party components below, each
under its own license, reproduced here as those licenses require.
EOF

echo "▶ Go standard library"
section "Go" "$(cd "$ROOT/backend-device" && go env GOVERSION)" "https://go.dev" "$(cd "$ROOT/backend-device" && go env GOROOT)/LICENSE"

echo "▶ Go modules"
(cd "$ROOT/backend-device" && go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}} {{.Module.Dir}}{{end}}' ./cmd/desk) \
  | sort -u | grep -v '^github.com/lordbasex/' | while read -r path version dir; do
    f="$(license_file "$dir")"
    [[ -n "$f" ]] || { echo "  no license file for $path" >&2; exit 1; }
    section "$path" "$version" "https://pkg.go.dev/$path" "$f"
  done

echo "▶ libvpx, libopus and libyuv (linked into the host)"
curl -fsSL "https://github.com/webmproject/libvpx/archive/refs/tags/v$VPX_VERSION.tar.gz" | tar xz -C "$WORK"
cat "$WORK/libvpx-$VPX_VERSION/LICENSE" "$WORK/libvpx-$VPX_VERSION/PATENTS" > "$WORK/vpx.txt"
section "libvpx" "$VPX_VERSION" "https://chromium.googlesource.com/webm/libvpx" "$WORK/vpx.txt"
curl -fsSL "https://downloads.xiph.org/releases/opus/opus-$OPUS_VERSION.tar.gz" | tar xz -C "$WORK"
section "libopus" "$OPUS_VERSION" "https://opus-codec.org" "$WORK/opus-$OPUS_VERSION/COPYING"
curl -fsSL "https://deb.debian.org/debian/pool/main/liby/libyuv/libyuv_$YUV_VERSION.orig.tar.xz" | tar xJ -C "$WORK"
cat "$WORK/libyuv-$YUV_VERSION/LICENSE" "$WORK/libyuv-$YUV_VERSION/PATENTS" > "$WORK/yuv.txt"
section "libyuv" "$YUV_VERSION" "https://chromium.googlesource.com/libyuv/libyuv" "$WORK/yuv.txt"

echo "▶ wlr-data-control (the Linux host's clipboard protocol, generated code)"
awk '/Copyright ©/{f=1} f{sub(/^ \* ?/, ""); print} /THIS SOFTWARE\./{exit}' \
  "$ROOT/backend-device/pkg/clipboard/wlr_data_control_linux.c" > "$WORK/wlr.txt"
section "wlr-data-control-unstable-v1" "1" "https://gitlab.freedesktop.org/wlroots/wlr-protocols" "$WORK/wlr.txt"

echo "▶ Website packages and fonts"
(cd "$ROOT/frontend" && node -e '
  const lock = require("./package-lock.json");
  for (const [k, v] of Object.entries(lock.packages)) {
    if (!k.includes("node_modules/") || v.dev || v.link) continue;
    console.log(k + " " + v.version);
  }') | sort -u | while read -r dir version; do
    name="${dir##*node_modules/}"
    f="$(license_file "$ROOT/frontend/$dir")"
    if [[ -z "$f" ]]; then
      # A package without a license file: the MIT text with the copyright
      # line from its sources.
      lic="$(node -p "require('$ROOT/frontend/$dir/package.json').license")"
      holder="$(grep -rhm1 -o 'Copyright (c) [0-9]* .*' "$ROOT/frontend/$dir" --include='*.js' | head -1)"
      [[ "$lic" == "MIT" && -n "$holder" ]] || { echo "  no license file for $name" >&2; exit 1; }
      f="$WORK/$(echo "$name" | tr '/' '_').txt"
      mit "$holder" > "$f"
    fi
    section "$name" "$version" "https://www.npmjs.com/package/$name" "$f"
  done

echo "✓ $OUT"
