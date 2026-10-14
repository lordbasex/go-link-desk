#!/bin/sh
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# The package cannot know whose shell the sessions should be: it says how.
cat <<'MSG'
go-link-desk-terminal is installed. As the user whose shell sessions get (not root):

  systemctl --user enable --now go-link-desk-terminal
  sudo loginctl enable-linger "$USER"     # keep it running without a login
  go-link-desk-terminal password
  go-link-desk-terminal link
MSG
