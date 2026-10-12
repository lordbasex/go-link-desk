#!/bin/sh
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Installs go-link Desk on Linux, the latest release:
#
#   curl -fsSL https://desk.go-link.org/install.sh | sh
#
# It takes the package for this system (.deb with apt, .rpm with dnf, yum or
# zypper, the Arch package with pacman), checks it against the release's
# SHA256SUMS and installs it with sudo. Without a known package manager or
# sudo, or with --user, it installs the .tar.gz for this user only
# (~/.local, no root). Options:
#
#   --user            install for this user only
#   --version vX.Y.Z  that version instead of the latest
#   --uninstall       remove it (the package, or the user install)
#
# GO_LINK_DESK_DOWNLOAD (the folder of the release files) can point it
# elsewhere, for testing.
set -eu

REPO="lordbasex/go-link-desk"
NAME="go-link-desk"

say() { printf '%s\n' "$*"; }
fail() { printf 'go-link Desk: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

MODE=auto
VERSION="${GO_LINK_DESK_VERSION:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --user) MODE=user ;;
    --uninstall) MODE=uninstall ;;
    --version) shift; VERSION="${1:-}" ;;
    *) fail "unknown option: $1" ;;
  esac
  shift
done

[ "$(uname -s)" = Linux ] || fail "this installer is for Linux; for macOS and the other ways see https://desk.go-link.org"

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  if have sudo; then SUDO="sudo"; else SUDO="none"; fi
fi

PM=""
if have apt-get && have dpkg; then PM=apt
elif have dnf; then PM=dnf
elif have yum; then PM=yum
elif have zypper; then PM=zypper
elif have pacman; then PM=pacman
fi

user_uninstall() {
  DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
  rm -f "$HOME/.local/bin/$NAME" "$DATA/applications/org.go-link.desk.desktop"
  for s in 48 64 128 256 512; do rm -f "$DATA/icons/hicolor/${s}x${s}/apps/org.go-link.desk.png"; done
}

if [ "$MODE" = uninstall ]; then
  user_uninstall
  if [ "$SUDO" != none ]; then
    case "$PM" in
      apt) dpkg -s "$NAME" >/dev/null 2>&1 && $SUDO apt-get remove -y "$NAME" ;;
      dnf | yum | zypper) rpm -q "$NAME" >/dev/null 2>&1 && $SUDO "$PM" remove -y "$NAME" ;;
      pacman) pacman -Q "$NAME" >/dev/null 2>&1 && $SUDO pacman -R --noconfirm "$NAME" ;;
    esac
  fi
  say "go-link Desk was removed (its settings stay in ~/.config/go-link-desk)."
  exit 0
fi

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64; RPMARCH=x86_64 ;;
  aarch64 | arm64) ARCH=arm64; RPMARCH=aarch64 ;;
  *) fail "no build for $(uname -m) yet (x86-64 and ARM64 only)" ;;
esac

if have curl; then
  get() { curl -fsSL "$1" -o "$2"; }
  get_text() { curl -fsSL "$1"; }
elif have wget; then
  get() { wget -qO "$2" "$1"; }
  get_text() { wget -qO- "$1"; }
else
  fail "curl or wget is needed"
fi

if [ -z "$VERSION" ] && [ -z "${GO_LINK_DESK_DOWNLOAD:-}" ]; then
  VERSION="$(get_text "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$VERSION" ] || fail "could not find the latest release (GitHub did not answer); try --version vX.Y.Z"
fi
VERSION="${VERSION:-v0.0.0}"
case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
NUM="${VERSION#v}"
BASE="${GO_LINK_DESK_DOWNLOAD:-https://github.com/$REPO/releases/download/$VERSION}"

if [ "$MODE" = auto ] && { [ -z "$PM" ] || [ "$SUDO" = none ]; }; then MODE=user; fi
if [ "$MODE" = user ]; then
  FILE="$NAME-$VERSION-linux-$ARCH.tar.gz"
else
  case "$PM" in
    apt) FILE="${NAME}_${NUM}_${ARCH}.deb" ;;
    dnf | yum | zypper) FILE="$NAME-$NUM-1.$RPMARCH.rpm" ;;
    pacman) FILE="$NAME-$NUM-1-$RPMARCH.pkg.tar.zst" ;;
  esac
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
say "Downloading go-link Desk $VERSION ($FILE)…"
get "$BASE/$FILE" "$TMP/$FILE" || fail "could not download $BASE/$FILE"
get "$BASE/SHA256SUMS" "$TMP/SHA256SUMS" || fail "could not download the release's SHA256SUMS"
want="$(grep " \*\{0,1\}$FILE\$" "$TMP/SHA256SUMS" | cut -d' ' -f1 | head -1)"
[ -n "$want" ] || fail "$FILE is not in SHA256SUMS"
if have sha256sum; then got="$(sha256sum "$TMP/$FILE" | cut -d' ' -f1)"; else got="$(shasum -a 256 "$TMP/$FILE" | cut -d' ' -f1)"; fi
[ "$got" = "$want" ] || fail "the download does not match its SHA-256; nothing was installed"

case "$MODE:$PM" in
  user:*)
    tar -xzf "$TMP/$FILE" -C "$TMP"
    "$TMP/$NAME-$VERSION-linux-$ARCH/install.sh"
    exit 0
    ;;
  *:apt) $SUDO apt-get install -y "$TMP/$FILE" ;;
  *:dnf | *:yum) $SUDO "$PM" install -y "$TMP/$FILE" ;;
  *:zypper) $SUDO zypper --non-interactive install --allow-unsigned-rpm "$TMP/$FILE" ;;
  *:pacman) $SUDO pacman -U --noconfirm "$TMP/$FILE" ;;
esac
say "go-link Desk $VERSION is installed: open it from the applications menu, or run $NAME."
