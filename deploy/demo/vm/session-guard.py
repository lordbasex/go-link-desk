#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# The demo computer's session guard (go-link-desk-demo-guard.service, as
# root): a visit lasts DEMO_MINUTES at most (5 by default), and after every
# visit, ended by the viewer or by the time, the computer starts over:
# go-link Desk stops (the viewer is cut off), everything of the user "demo"
# is closed and its files are deleted, the desktop logs in again with a new
# Minesweeper, what viewers sent is deleted, and go-link Desk starts again
# with new codes, which the demo page shows within 30 seconds. The linked
# computers' pairing and password stay the same.
#
# It follows the host's log: "session started" and "session ended" with
# the viewer's peer_id.

import os
import re
import select
import subprocess
import sys
import time

MINUTES = float(os.environ.get("DEMO_MINUTES", "5"))
HOST = "go-link-desk-demo.service"
STARTED = re.compile(r'msg="session started" peer_id=(\w+)')
ENDED = re.compile(r'msg="session ended" peer_id=(\w+)')


def log(msg):
    print(msg, flush=True)


def run(*cmd):
    subprocess.run(cmd, check=False)


def wait_for_x(timeout=60):
    end = time.time() + timeout
    while time.time() < end:
        if os.path.exists("/tmp/.X11-unix/X0"):
            return
        time.sleep(1)


def reset(why):
    log(f"starting over ({why})")
    run("systemctl", "stop", HOST)
    run("systemctl", "stop", "lightdm.service")
    run("pkill", "-KILL", "-u", "demo")
    # demo's home is a tmpfs: empty it, keep the mount.
    run("find", "/home/demo", "-mindepth", "1", "-delete")
    # Received exists once a viewer has sent a file.
    if os.path.isdir("/home/golink/files/Received"):
        run("find", "/home/golink/files/Received", "-mindepth", "1", "-delete")
    run("truncate", "-s", "0", "/home/golink/.config/go-link-desk-demo/clipboard.txt")
    run("systemctl", "start", "lightdm.service")
    wait_for_x()
    # The session's autostart opens the display to golink (xhost) a moment
    # after the X server is up.
    time.sleep(4)
    run("systemctl", "start", HOST)
    log("ready")


def follow():
    return subprocess.Popen(
        ["journalctl", "-u", HOST, "-f", "-n", "0", "-o", "cat"],
        stdout=subprocess.PIPE, bufsize=0)


def main():
    log(f"guarding the demo: {MINUTES:g} minutes a visit")
    journal = follow()
    fd = journal.stdout.fileno()
    pending = b""
    active = set()
    deadline = None
    while True:
        timeout = None if deadline is None else max(0.0, deadline - time.time())
        ready, _, _ = select.select([fd], [], [], timeout)
        if not ready:
            # The visit's time is over.
            reset("time is over")
            active.clear()
            deadline = None
            continue
        chunk = os.read(fd, 65536)
        if not chunk:
            log("the journal ended")
            sys.exit(1)
        pending += chunk
        *lines, pending = pending.split(b"\n")
        for raw in lines:
            line = raw.decode("utf-8", "replace")
            m = STARTED.search(line)
            if m:
                active.add(m.group(1))
                if deadline is None:
                    deadline = time.time() + MINUTES * 60
                log(f"a visit started ({len(active)} viewing)")
                continue
            m = ENDED.search(line)
            if m and m.group(1) in active:
                active.discard(m.group(1))
                if not active:
                    reset("the viewer left")
                    deadline = None

if __name__ == "__main__":
    main()
