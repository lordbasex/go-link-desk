#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
"""A small App Store Connect API client: no packages, openssl signs the token.

    asc.py GET  /v1/apps?filter[bundleId]=org.go-link.desk.ios
    asc.py POST /v1/betaGroups '{"data": {...}}'

The key comes from the environment (`source ~/.go-link-signing/env.sh`):
ASC_KEY_ID, ASC_ISSUER_ID and ASC_KEY_PATH (the .p8 file). The token lives
20 minutes, as Apple allows. Prints the JSON reply; exits 1 on an HTTP error.
"""

import base64
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = "https://api.appstoreconnect.apple.com"


def b64(data):
    return base64.urlsafe_b64encode(data).rstrip(b"=")


def der_to_raw(der):
    """An ECDSA signature from openssl (DER) as JWT's ES256 wants it (r || s)."""
    assert der[0] == 0x30
    i = 2 if der[1] < 0x80 else 2 + (der[1] & 0x7F)
    parts = []
    for _ in range(2):
        assert der[i] == 0x02
        n = der[i + 1]
        parts.append(der[i + 2:i + 2 + n].lstrip(b"\0").rjust(32, b"\0"))
        i += 2 + n
    return parts[0] + parts[1]


def token():
    key_id, issuer, path = os.environ["ASC_KEY_ID"], os.environ["ASC_ISSUER_ID"], os.environ["ASC_KEY_PATH"]
    now = int(time.time())
    head = b64(json.dumps({"alg": "ES256", "kid": key_id, "typ": "JWT"}).encode())
    body = b64(json.dumps({"iss": issuer, "iat": now, "exp": now + 1200, "aud": "appstoreconnect-v1"}).encode())
    msg = head + b"." + body
    der = subprocess.run(["openssl", "dgst", "-sha256", "-sign", path], input=msg, capture_output=True, check=True).stdout
    return (msg + b"." + b64(der_to_raw(der))).decode()


def call(method, path, body=None):
    req = urllib.request.Request(API + path, method=method, data=body.encode() if body else None)
    req.add_header("Authorization", "Bearer " + token())
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    status, text = call(sys.argv[1], sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else None)
    print(text)
    sys.exit(0 if status < 400 else 1)
