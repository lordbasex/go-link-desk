#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
"""Uploads the App Store screenshots of a version, replacing what is there.

    screenshots.py VERSION_ID DIR

DIR holds {iphone,ipad}-{en,es,pt}-{1,2,3}-*.png (Home, a session, the
guide). The key comes from the environment, as asc.py's.
"""

import glob
import hashlib
import json
import os
import sys
import urllib.request

sys.path.insert(0, os.path.dirname(__file__))
from asc import call  # noqa: E402

LOCALES = {"en": "en-US", "es": "es-MX", "pt": "pt-BR"}
# 6.9" iPhone (1320 x 2868) and 13" iPad (2064 x 2752).
DISPLAYS = {"iphone": "APP_IPHONE_67", "ipad": "APP_IPAD_PRO_3GEN_129"}


def j(method, path, body=None):
    status, text = call(method, path, json.dumps(body) if body else None)
    if status >= 400:
        sys.exit(f"{method} {path}: {status} {text[:400]}")
    return json.loads(text) if text.strip() else {}


def upload(set_id, path):
    data = open(path, "rb").read()
    shot = j("POST", "/v1/appScreenshots", {"data": {"type": "appScreenshots",
        "attributes": {"fileName": os.path.basename(path), "fileSize": len(data)},
        "relationships": {"appScreenshotSet": {"data": {"type": "appScreenshotSets", "id": set_id}}}}})["data"]
    for op in shot["attributes"]["uploadOperations"]:
        part = data[op["offset"]:op["offset"] + op["length"]]
        req = urllib.request.Request(op["url"], data=part, method=op["method"])
        for h in op["requestHeaders"]:
            req.add_header(h["name"], h["value"])
        urllib.request.urlopen(req).read()
    j("PATCH", f"/v1/appScreenshots/{shot['id']}", {"data": {"type": "appScreenshots", "id": shot["id"],
        "attributes": {"uploaded": True, "sourceFileChecksum": hashlib.md5(data).hexdigest()}}})


def main():
    version, folder = sys.argv[1], sys.argv[2]
    locs = {l["attributes"]["locale"]: l["id"] for l in j("GET", f"/v1/appStoreVersions/{version}/appStoreVersionLocalizations")["data"]}
    for short, locale in LOCALES.items():
        for device, display in DISPLAYS.items():
            files = sorted(glob.glob(f"{folder}/{device}-{short}-*.png"))
            if not files:
                continue
            sets = j("GET", f"/v1/appStoreVersionLocalizations/{locs[locale]}/appScreenshotSets")["data"]
            existing = [s for s in sets if s["attributes"]["screenshotDisplayType"] == display]
            if existing:
                set_id = existing[0]["id"]
                for old in j("GET", f"/v1/appScreenshotSets/{set_id}/appScreenshots")["data"]:
                    j("DELETE", f"/v1/appScreenshots/{old['id']}")
            else:
                set_id = j("POST", "/v1/appScreenshotSets", {"data": {"type": "appScreenshotSets",
                    "attributes": {"screenshotDisplayType": display},
                    "relationships": {"appStoreVersionLocalization": {"data": {"type": "appStoreVersionLocalizations", "id": locs[locale]}}}}})["data"]["id"]
            for f in files:
                upload(set_id, f)
            print(locale, display, len(files))


if __name__ == "__main__":
    main()
