go-link Desk for Linux
======================

Share this computer's screen with a 9-digit code, or use another computer.
https://desk.go-link.org

Install (for this user, no root):   ./install.sh
Remove:                             ./install.sh --uninstall
Or run it as it is:                 ./go-link-desk

Needs a desktop with glibc 2.34 or newer (Ubuntu 22.04, Debian 12,
Fedora 35, RHEL 9, Linux Mint 21 or newer), in an X11 (Xorg) or a Wayland
session:

- X11: any desktop.
- Wayland: one with the remote desktop portal and PipeWire, as GNOME and
  KDE Plasma have. The first time someone connects, the desktop asks you to
  share the screen; turn on "Allow Remote Interaction" so they can use the
  mouse and keyboard, and keep "Remember this selection" to not be asked
  again.

The computer's sound is shared through PulseAudio or PipeWire.

Free software under the MIT license (LICENSE); the third-party components
it includes are listed in THIRD_PARTY_NOTICES.md.
