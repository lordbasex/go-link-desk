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
# (~/.local, no root). SHA256SUMS counts only with the update key's
# signature (SHA256SUMS.sig), checked with OpenSSL 3. Options:
#
#   --terminal        go-link-desk-terminal instead: a remote shell for a
#                     computer without a desktop (a Raspberry Pi, a server),
#                     also on ARMv7; run it as the user whose shell sessions get
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
TERMINAL=""
VERSION="${GO_LINK_DESK_VERSION:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --terminal) TERMINAL=1; NAME="go-link-desk-terminal" ;;
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

# The terminal's user service: started for this user, and kept running
# without a login (a Raspberry Pi with nobody at it).
terminal_start() {
  if [ "$(id -u)" -eq 0 ]; then
    say "As the user whose shell sessions get (not root): systemctl --user enable --now $NAME"
    return
  fi
  if have systemctl && systemctl --user daemon-reload 2>/dev/null && systemctl --user enable --now "$NAME"; then
    loginctl enable-linger "$USER" 2>/dev/null || { [ "$SUDO" = sudo ] && sudo loginctl enable-linger "$USER"; } ||
      say "To keep it running without a login: sudo loginctl enable-linger $USER"
  else
    say "Start it from a login session: systemctl --user enable --now $NAME"
  fi
}

user_uninstall() {
  if [ -n "$TERMINAL" ]; then
    have systemctl && systemctl --user disable --now "$NAME" >/dev/null 2>&1 || true
    rm -f "$HOME/.local/bin/$NAME" "${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/$NAME.service"
    return
  fi
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
  say "$NAME was removed (its settings stay in ~/.config/$NAME)."
  exit 0
fi

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64; DEBARCH=amd64; RPMARCH=x86_64; PACARCH=x86_64 ;;
  aarch64 | arm64) ARCH=arm64; DEBARCH=arm64; RPMARCH=aarch64; PACARCH=aarch64 ;;
  armv7l | armv8l)
    [ -n "$TERMINAL" ] || fail "no desktop build for $(uname -m): for a remote terminal on this computer, add --terminal"
    ARCH=armv7; DEBARCH=armhf; RPMARCH=armv7hl; PACARCH=armv7h ;;
  *) fail "no build for $(uname -m) yet (x86-64 and ARM64; ARMv7 with --terminal)" ;;
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
    apt) FILE="${NAME}_${NUM}_${DEBARCH}.deb" ;;
    dnf | yum | zypper) FILE="$NAME-$NUM-1.$RPMARCH.rpm" ;;
    pacman) FILE="$NAME-$NUM-1-$PACARCH.pkg.tar.zst" ;;
  esac
fi

# The update key, the one the app's updater checks (internal/updater), as
# a public key file: SHA256SUMS counts only with its signature, so a
# changed release, or GO_LINK_DESK_DOWNLOAD pointing elsewhere, cannot
# slip a package in. Its private half is on the release computer only.
UPDATE_KEY="MCowBQYDK2VwAyEAFhUt69Sg5H3ZLX+Fpnzb98eG2EDRGKzHxNE+iT2nyJI=" # gitleaks:allow (a public key)

need_openssl() {
  have openssl || fail "openssl is needed to check the release's signature: install it (apt, dnf or zypper install openssl; pacman -S openssl)"
  openssl pkeyutl -help 2>&1 | grep -q rawin ||
    fail "OpenSSL 3 or newer is needed to check the release's signature; or download the package from https://github.com/$REPO/releases and install it yourself"
}

verify_sums() {
  printf -- '-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----\n' "$UPDATE_KEY" >"$TMP/update.pem"
  tr -d ' \r\n' <"$TMP/SHA256SUMS.sig" >"$TMP/sig.b64"
  openssl base64 -d -A -in "$TMP/sig.b64" -out "$TMP/sig.bin" 2>/dev/null || fail "the release's SHA256SUMS.sig is not a signature; nothing was installed"
  openssl pkeyutl -verify -pubin -inkey "$TMP/update.pem" -rawin -in "$TMP/SHA256SUMS" -sigfile "$TMP/sig.bin" >/dev/null 2>&1 ||
    fail "the release's SHA256SUMS is not signed with go-link Desk's update key; nothing was installed"
}

need_openssl
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
say "Downloading $NAME $VERSION ($FILE)…"
get "$BASE/$FILE" "$TMP/$FILE" || fail "could not download $BASE/$FILE"
get "$BASE/SHA256SUMS" "$TMP/SHA256SUMS" || fail "could not download the release's SHA256SUMS"
get "$BASE/SHA256SUMS.sig" "$TMP/SHA256SUMS.sig" || fail "could not download the release's SHA256SUMS.sig"
verify_sums
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
  # The package carries no RPM signature: the signed SHA256SUMS vouches for it.
  *:zypper) $SUDO zypper --non-interactive install --allow-unsigned-rpm "$TMP/$FILE" ;;
  *:pacman) $SUDO pacman -U --noconfirm "$TMP/$FILE" ;;
esac
if [ -n "$TERMINAL" ]; then
  terminal_start
  say "$NAME $VERSION is installed. Next, as this user:"
  say "  $NAME password   the password linked browsers must give"
  say "  $NAME link       a code to link your browser at desk.go-link.org"
  exit 0
fi
say "go-link Desk $VERSION is installed: open it from the applications menu, or run $NAME."
