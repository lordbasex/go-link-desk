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
#   windows: the .exe's import table (read with Go's debug/pe, on any
#           system) may only list DLLs that come with Windows 10 and 11.
#
# Usage: scripts/check-deps.sh darwin|linux|windows BINARY
set -euo pipefail

OS="${1:?usage: check-deps.sh darwin|linux|windows BINARY}"
BIN="${2:?usage: check-deps.sh darwin|linux|windows BINARY}"
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
  windows)
    abs="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"
    libs="$(cd "$(dirname "$0")/../backend-device" && GOOS= GOARCH= CGO_ENABLED=0 go run ./build/windows/imports "$abs" | tr 'A-Z' 'a-z')"
    # The universal C runtime's api-ms-win-crt-* sets and Windows' own
    # system DLLs; never libvpx, libopus, libwinpthread or a C++ runtime.
    allowed='^(api-ms-win-[a-z0-9-]+\.dll|(kernel32|user32|gdi32|shell32|advapi32|ole32|oleaut32|opengl32|ws2_32|winmm|ntdll|bcrypt|crypt32|secur32|iphlpapi|dnsapi|dwmapi|imm32|uxtheme|shcore|shlwapi|comctl32|comdlg32|version|userenv|powrprof|setupapi|cfgmgr32|hid|d3d11|dxgi|d2d1|dwrite|windowscodecs|mfplat|mf|mfreadwrite|evr|wtsapi32|propsys|avrt|ksuser|mmdevapi|ucrtbase|msvcrt|wintrust|netapi32|normaliz|winhttp|wininet|rpcrt4|sspicli|mswsock)\.dll)$'
    ;;
  *)
    echo "unknown OS: $OS (darwin, linux or windows)" >&2
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
