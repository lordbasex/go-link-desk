#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Builds libvpx (VP8 only) and libopus from source as static libraries for
# one macOS architecture, for the oldest macOS the app supports. Homebrew's
# copies only exist for the Mac's own CPU and require the macOS they were
# built on, so a universal app that opens on macOS 13 needs its own.
#
# Usage: build/macos/static-libs.sh amd64|arm64 OUT_DIR
#
# OUT_DIR gets lib/*.a, include/ and lib/pkgconfig/*.pc. It is a cache: a
# finished build (OUT_DIR/.done-<versions>) is not built again. Works on an
# Intel Mac and on Apple silicon (clang builds both architectures); the
# x86_64 build needs nasm (brew install nasm).
set -euo pipefail

ARCH="${1:?usage: static-libs.sh amd64|arm64 OUT_DIR}"
OUT="${2:?usage: static-libs.sh amd64|arm64 OUT_DIR}"

MIN_MACOS="${MIN_MACOS:-13.0}"
OPUS_VERSION=1.6.1
OPUS_SHA256=6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1
VPX_VERSION=1.17.0
VPX_SHA256=1020f184046187baa2985dbde38e0691f49c44088bca7a1842b0236c6081dc0a

case "$ARCH" in
  amd64) CLANG_ARCH=x86_64; VPX_TARGET=x86_64-darwin21-gcc; HOST=x86_64-apple-darwin ;;
  arm64) CLANG_ARCH=arm64; VPX_TARGET=arm64-darwin21-gcc; HOST=aarch64-apple-darwin ;;
  *) echo "unknown architecture: $ARCH (amd64 or arm64)" >&2; exit 1 ;;
esac

DONE="$OUT/.done-opus$OPUS_VERSION-vpx$VPX_VERSION-macos$MIN_MACOS"
if [[ -f "$DONE" ]]; then
  exit 0
fi

mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
JOBS="$(sysctl -n hw.ncpu)"
SDK="$(xcrun --sdk macosx --show-sdk-path)"
FLAGS="-arch $CLANG_ARCH -mmacosx-version-min=$MIN_MACOS -isysroot $SDK"

fetch() { # url sha256 file
  curl -fsSL "$1" -o "$WORK/$3"
  echo "$2  $WORK/$3" | shasum -a 256 -c --quiet -
  tar xzf "$WORK/$3" -C "$WORK"
}

echo "▶ libopus $OPUS_VERSION for macOS $MIN_MACOS $CLANG_ARCH"
fetch "https://downloads.xiph.org/releases/opus/opus-$OPUS_VERSION.tar.gz" "$OPUS_SHA256" opus.tar.gz
(
  cd "$WORK/opus-$OPUS_VERSION"
  ./configure --host="$HOST" --prefix="$OUT" --enable-static --disable-shared \
    --disable-doc --disable-extra-programs \
    CC="$(xcrun -f clang)" CFLAGS="$FLAGS -O2" LDFLAGS="$FLAGS" >/dev/null
  make -j"$JOBS" >/dev/null
  make install >/dev/null
)

echo "▶ libvpx $VPX_VERSION (VP8) for macOS $MIN_MACOS $CLANG_ARCH"
fetch "https://github.com/webmproject/libvpx/archive/refs/tags/v$VPX_VERSION.tar.gz" "$VPX_SHA256" vpx.tar.gz
(
  cd "$WORK/libvpx-$VPX_VERSION"
  ./configure --target="$VPX_TARGET" --prefix="$OUT" --enable-static --disable-shared \
    --disable-examples --disable-tools --disable-docs --disable-unit-tests \
    --enable-vp8 --disable-vp9 \
    --extra-cflags="-mmacosx-version-min=$MIN_MACOS" --extra-cxxflags="-mmacosx-version-min=$MIN_MACOS" >/dev/null
  make -j"$JOBS" >/dev/null
  make install >/dev/null
)

rm -f "$OUT"/.done-*
touch "$DONE"
echo "✓ $OUT"
