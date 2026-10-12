#!/bin/sh
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Installs go-link Desk for this user (no root): the program in
# ~/.local/bin, its menu entry and its icon. Run it again to update;
# ./install.sh --uninstall removes them.
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
BIN="$HOME/.local/bin"
SIZES="48 64 128 256 512"

if [ "${1:-}" = "--uninstall" ]; then
  rm -f "$BIN/go-link-desk" "$DATA/applications/org.go-link.desk.desktop"
  for s in $SIZES; do rm -f "$DATA/icons/hicolor/${s}x${s}/apps/org.go-link.desk.png"; done
  echo "go-link Desk was removed (its settings stay in ~/.config/go-link-desk)."
  exit 0
fi

mkdir -p "$BIN" "$DATA/applications"
install -m 0755 "$HERE/go-link-desk" "$BIN/go-link-desk"
for s in $SIZES; do
  mkdir -p "$DATA/icons/hicolor/${s}x${s}/apps"
  install -m 0644 "$HERE/icons/hicolor/${s}x${s}/apps/org.go-link.desk.png" "$DATA/icons/hicolor/${s}x${s}/apps/"
done
sed "s|^Exec=.*|Exec=\"$BIN/go-link-desk\"|" "$HERE/org.go-link.desk.desktop" > "$DATA/applications/org.go-link.desk.desktop"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q "$DATA/applications" || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t "$DATA/icons/hicolor" 2>/dev/null || true

echo "go-link Desk is installed: open it from the applications menu, or run $BIN/go-link-desk."
