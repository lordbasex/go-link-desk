go-link-desk-terminal
=====================

A remote terminal with go-link Desk, for a computer without a desktop (a
Raspberry Pi, a server): each session is a shell of its own, in the browser
at desk.go-link.org. Nothing listens on a port.

Install for this user (no root), as the user whose shell sessions get:

  ./install.sh

Then:

  go-link-desk-terminal password   the password linked browsers must give
  go-link-desk-terminal link       a code to link your browser; it asks here first
  go-link-desk-terminal status     the service and the linked browsers

Remove it: ./install.sh --uninstall

More: https://github.com/lordbasex/go-link-desk/blob/main/docs/terminal.md
