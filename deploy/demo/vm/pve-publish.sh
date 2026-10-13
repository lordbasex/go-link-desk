#!/bin/sh
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Runs on the Proxmox host (pve-publish.timer): reads the demonstration
# computer's codes with a key that may only read that file (a forced
# command in its authorized_keys) and publishes them when they changed
# (publish.py, with the AWS key that may write desk.go-link.org/demo.json
# only). The demonstration computer keeps no credential.
set -eu
VM="${DEMO_VM:-deskadmin@172.17.0.42}"
KEY=/root/.ssh/desk-demo-pull
STATE=/var/lib/desk-demo
mkdir -p "$STATE"
ssh -i "$KEY" -o BatchMode=yes -o ConnectTimeout=5 "$VM" > "$STATE/new.json"
if ! cmp -s "$STATE/new.json" "$STATE/published.json" 2>/dev/null; then
  python3 /usr/local/lib/desk-demo/publish.py "$STATE/new.json"
  mv "$STATE/new.json" "$STATE/published.json"
fi
