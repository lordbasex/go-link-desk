#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
"""Takes the Google Play screenshots on a running emulator.

    screenshots.py SERIAL KIND CODE DIR

SERIAL is the emulator (adb devices), KIND names the files (phone, tab7,
tab10), CODE is the session code of a test host started with
`desk --headless --source testpattern --test-auto-accept --test-input-log
--test-name "Studio Mac"`, so this machine's name never shows. It needs the
debug build installed (it seeds the app's preferences with run-as).

A phone is shot at 1080 x 1920 (Google Play takes at most 2:1).
For each language it writes DIR/KIND-LANG-{1,2,3}.png: Home with sample
computers, a session, and the guide.
"""

import json
import os
import subprocess
import sys
import time

ADB = os.path.expanduser("~/Library/Android/sdk/platform-tools/adb")
APP = "org.golink.desk"
NOW = int(time.time() * 1000)
DAY = 86_400_000

RECENTS = [
    {"code": "464786468", "name": "Studio Mac", "os": "darwin", "at": NOW - DAY // 8},
    {"code": "915355636", "name": "Office PC", "os": "windows", "at": NOW - DAY},
    {"code": "302118490", "name": "Lab", "os": "linux", "at": NOW - 3 * DAY},
]
LINKS = [
    {"deviceId": "sample-1", "linkId": "l1", "name": "Home Mac", "os": "darwin", "linkedAt": NOW - 20 * DAY, "lastUsed": NOW - DAY // 3},
    {"deviceId": "sample-2", "linkId": "l2", "name": "Workstation", "os": "windows", "linkedAt": NOW - 9 * DAY, "lastUsed": NOW - 2 * DAY},
]


def adb(serial, *args, check=True):
    return subprocess.run([ADB, "-s", serial, *args], check=check, capture_output=True, text=True).stdout


def prefs(values):
    """A SharedPreferences file with these strings and booleans."""
    rows = []
    for k, v in values.items():
        if isinstance(v, bool):
            rows.append(f'    <boolean name="{k}" value="{"true" if v else "false"}" />')
        else:
            text = v.replace("&", "&amp;").replace('"', "&quot;").replace("<", "&lt;")
            rows.append(f'    <string name="{k}">{text}</string>')
    return "<?xml version='1.0' encoding='utf-8' standalone='yes' ?>\n<map>\n" + "\n".join(rows) + "\n</map>\n"


def hang_up(serial):
    """Back ends a session (session_end): the host is free for the next one at once."""
    adb(serial, "shell", "input", "keyevent", "KEYCODE_BACK")
    time.sleep(2)
    adb(serial, "shell", "am", "force-stop", APP)


def seed(serial, guide_seen):
    adb(serial, "shell", "am", "force-stop", APP)
    files = {
        "desk.xml": prefs({"recents": json.dumps(RECENTS), "guide_seen": guide_seen, "name": ""}),
        "links.xml": prefs({"links": json.dumps(LINKS)}),
    }
    for name, body in files.items():
        subprocess.run([ADB, "-s", serial, "shell", f"run-as {APP} sh -c 'mkdir -p shared_prefs && cat > shared_prefs/{name}'"],
                       input=body, text=True, check=True)


def shot(serial, path):
    # A touch on an empty edge leaves the keyboard focus mode the keys put it in.
    adb(serial, "shell", "input", "tap", "4", "700")
    time.sleep(1)
    with open(path, "wb") as f:
        f.write(subprocess.run([ADB, "-s", serial, "exec-out", "screencap", "-p"], check=True, capture_output=True).stdout)
    print(path)


def demo_bar(serial):
    adb(serial, "shell", "settings", "put", "global", "sysui_demo_allowed", "1")
    for extra in (["-e", "command", "enter"], ["-e", "command", "clock", "-e", "hhmm", "0941"],
                  ["-e", "command", "battery", "-e", "level", "100", "-e", "plugged", "false"],
                  ["-e", "command", "network", "-e", "wifi", "show", "-e", "level", "4"],
                  ["-e", "command", "notifications", "-e", "visible", "false"]):
        adb(serial, "shell", "am", "broadcast", "-a", "com.android.systemui.demo", *extra)


def wait_live(serial, timeout=40):
    end = time.time() + timeout
    while time.time() < end:
        log = adb(serial, "logcat", "-d", "-s", "go-link-desk:I", check=False)
        if "status Live" in log:
            time.sleep(4)  # a few frames
            return "live"
        if "Rejected(reason=busy)" in log:
            return "busy"
        time.sleep(1)
    return "no"


def session(serial, code):
    for _ in range(8):
        adb(serial, "logcat", "-c")
        adb(serial, "shell", "am", "start", "-n", f"{APP}/.MainActivity", "--es", "code", code)
        state = wait_live(serial)
        if state == "live":
            return
        if state == "no":
            break
        # The host still ends the last session: try again in a moment.
        adb(serial, "shell", "am", "force-stop", APP)
        time.sleep(5)
    sys.exit("the session did not start: is the test host running with that code?")


def rotate(serial, turns):
    adb(serial, "shell", "wm", "user-rotation", "lock", str(turns))
    time.sleep(2)


def main():
    serial, kind, code, out = sys.argv[1:5]
    os.makedirs(out, exist_ok=True)
    phone = kind == "phone"
    # Google Play takes at most 2:1: a phone is shot at 9:16.
    if phone:
        adb(serial, "shell", "wm", "size", "1080x1920")
    demo_bar(serial)
    # No "Viewing full screen" hint, which a new device shows once.
    adb(serial, "shell", "settings", "put", "secure", "immersive_mode_confirmations", "confirmed")
    # The rotation is ours, whatever the emulator's sensor says.
    adb(serial, "shell", "wm", "fixed-to-user-rotation", "enabled")
    # The app's data folder is made on its first start.
    adb(serial, "shell", "am", "start", "-W", "-n", f"{APP}/.MainActivity")
    for lang in ("en", "es", "pt"):
        adb(serial, "shell", "cmd", "locale", "set-app-locales", APP, "--locales", lang)
        # 1: Home, upright on a phone.
        rotate(serial, 0)
        seed(serial, guide_seen=True)
        adb(serial, "shell", "am", "start", "-n", f"{APP}/.MainActivity")
        time.sleep(4)
        shot(serial, f"{out}/{kind}-{lang}-1-home.png")
        # 2: a session, sideways on a phone.
        if phone:
            rotate(serial, 1)
        seed(serial, guide_seen=True)
        session(serial, code)
        shot(serial, f"{out}/{kind}-{lang}-2-session.png")
        hang_up(serial)
        # 3: the guide, as it opens the first time (upright on a phone).
        rotate(serial, 0)
        seed(serial, guide_seen=False)
        session(serial, code)
        time.sleep(2)
        shot(serial, f"{out}/{kind}-{lang}-3-guide.png")
        # Back closes the guide, then ends the session.
        adb(serial, "shell", "input", "keyevent", "KEYCODE_BACK")
        hang_up(serial)
    adb(serial, "shell", "cmd", "locale", "set-app-locales", APP, "--locales", "")
    rotate(serial, 0)
    adb(serial, "shell", "wm", "user-rotation", "free")
    adb(serial, "shell", "wm", "fixed-to-user-rotation", "default")
    if phone:
        adb(serial, "shell", "wm", "size", "reset")
    adb(serial, "shell", "am", "broadcast", "-a", "com.android.systemui.demo", "-e", "command", "exit")


if __name__ == "__main__":
    main()
