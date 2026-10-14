#!/bin/sh
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Installs go-link-desk-terminal for this user, from this folder, without
# root: the program in ~/.local/bin and a systemd user service that runs it.
#
#   ./install.sh              install, enable and start it
#   ./install.sh --uninstall  stop it and remove it (device.json stays)
set -eu

NAME="go-link-desk-terminal"
HERE="$(cd "$(dirname "$0")" && pwd)"
BIN="$HOME/.local/bin/$NAME"
UNITS="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

say() { printf '%s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }

[ "$(id -u)" -ne 0 ] || { say "Run it as the user whose shell the sessions get, not root."; exit 1; }

if [ "${1:-}" = "--uninstall" ]; then
  have systemctl && systemctl --user disable --now "$NAME" >/dev/null 2>&1 || true
  rm -f "$BIN" "$UNITS/$NAME.service"
  have systemctl && systemctl --user daemon-reload >/dev/null 2>&1 || true
  say "$NAME was removed (its settings stay in ~/.config/$NAME)."
  exit 0
fi

mkdir -p "$HOME/.local/bin" "$UNITS"
install -m 0755 "$HERE/$NAME" "$BIN"
sed "s|@BIN@|$BIN|" "$HERE/$NAME.service" > "$UNITS/$NAME.service"
if have systemctl && systemctl --user daemon-reload 2>/dev/null; then
  systemctl --user enable --now "$NAME"
  # Without a login (a Raspberry Pi with nobody at it), user services
  # stop; lingering keeps this one running from boot.
  loginctl enable-linger "$USER" 2>/dev/null || say "Keep it running without a login: sudo loginctl enable-linger $USER"
  say "$NAME is running."
else
  say "No systemd user session here: run it with $BIN serve (or start the service from a login)."
fi
say ""
say "Next:"
say "  $NAME password   choose the password linked browsers must give"
say "  $NAME link       show a code to link your browser at desk.go-link.org"
