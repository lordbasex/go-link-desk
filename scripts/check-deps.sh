#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Fails when the host binary needs a library that does not come with the
# operating system. go-link Desk is portable: download and run, nothing to
# install, so libvpx, libopus and anything else of ours must be linked in.
#
#   darwin: otool -L may only list /System/Library and /usr/lib.
#   linux:  ldd may only list glibc, the C++ runtime, and the desktop's X11,
#           Wayland and OpenGL libraries.
#
# Usage: scripts/check-deps.sh darwin|linux BINARY
set -euo pipefail

OS="${1:?usage: check-deps.sh darwin|linux BINARY}"
BIN="${2:?usage: check-deps.sh darwin|linux BINARY}"
[[ -f "$BIN" ]] || { echo "no such binary: $BIN" >&2; exit 2; }

case "$OS" in
  darwin)
    # The first line names the binary itself (or one per architecture).
    libs="$(otool -L "$BIN" | grep -v ':$' | awk '{print $1}')"
    allowed='^(/System/Library/|/usr/lib/)'
    ;;
  linux)
    libs="$(ldd "$BIN" | awk '{print $1}')"
    allowed='^(linux-vdso\.so|linux-gate\.so|/lib(64)?/ld-linux|ld-linux|lib(c|m|dl|rt|pthread|resolv|util|gcc_s|stdc\+\+)\.so|libX[A-Za-z0-9]*\.so|libxcb[a-z0-9-]*\.so|libxkbcommon\.so|libGL[A-Za-z]*\.so|libEGL\.so|libOpenGL\.so|libwayland-[a-z]+\.so|libffi\.so|libbsd\.so|libmd\.so)'
    ;;
  *)
    echo "unknown OS: $OS (darwin or linux)" >&2
    exit 2
    ;;
esac

bad=0
while IFS= read -r lib; do
  [[ -z "$lib" ]] && continue
  if [[ ! "$lib" =~ $allowed ]]; then
    echo "✗ not part of the operating system: $lib" >&2
    bad=1
  fi
done <<< "$libs"

if [[ "$bad" -ne 0 ]]; then
  echo "✗ $BIN depends on libraries a clean $OS does not have; link them statically" >&2
  exit 1
fi
echo "✓ $BIN only needs the operating system's libraries"
