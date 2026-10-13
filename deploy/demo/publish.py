#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
"""Publishes the demonstration host's code file as desk.go-link.org/demo.json.

The host (--test-code-file) rewrites the file whenever its code changes;
systemd runs this then (go-link-desk-demo-publish.path). It PUTs the file
to the website's bucket with an AWS key that may write that one object
and nothing else (IAM user go-link-desk-demo). Python's standard library
only: Signature Version 4 by hand, so the computer needs no AWS CLI.

Usage: publish.py FILE (the key in AWS_ACCESS_KEY_ID and
AWS_SECRET_ACCESS_KEY, from the unit's EnvironmentFile).
"""
import datetime
import hashlib
import hmac
import json
import os
import sys
import urllib.request

BUCKET = os.environ.get("DEMO_BUCKET", "desk.go-link.org")
REGION = os.environ.get("DEMO_REGION", "us-east-1")
KEY = "demo.json"


def sign(key: bytes, msg: str) -> bytes:
    return hmac.new(key, msg.encode(), hashlib.sha256).digest()


def put(body: bytes) -> None:
    access, secret = os.environ["AWS_ACCESS_KEY_ID"], os.environ["AWS_SECRET_ACCESS_KEY"]
    # Path style: the bucket's name has dots, which the virtual host's
    # certificate (*.s3.<region>.amazonaws.com) does not cover.
    host = f"s3.{REGION}.amazonaws.com"
    path = f"/{BUCKET}/{KEY}"
    now = datetime.datetime.now(datetime.timezone.utc)
    amz_date, day = now.strftime("%Y%m%dT%H%M%SZ"), now.strftime("%Y%m%d")
    payload = hashlib.sha256(body).hexdigest()
    # The page reads it fresh: CloudFront keeps it a second at most.
    headers = {
        "cache-control": "no-cache, max-age=0",
        "content-type": "application/json",
        "host": host,
        "x-amz-content-sha256": payload,
        "x-amz-date": amz_date,
    }
    signed = ";".join(sorted(headers))
    canonical = "\n".join(
        ["PUT", path, "", "".join(f"{k}:{headers[k]}\n" for k in sorted(headers)), signed, payload]
    )
    scope = f"{day}/{REGION}/s3/aws4_request"
    to_sign = "\n".join(["AWS4-HMAC-SHA256", amz_date, scope, hashlib.sha256(canonical.encode()).hexdigest()])
    k = sign(("AWS4" + secret).encode(), day)
    for part in (REGION, "s3", "aws4_request"):
        k = sign(k, part)
    signature = hmac.new(k, to_sign.encode(), hashlib.sha256).hexdigest()
    headers["authorization"] = f"AWS4-HMAC-SHA256 Credential={access}/{scope}, SignedHeaders={signed}, Signature={signature}"
    req = urllib.request.Request(f"https://{host}{path}", data=body, method="PUT", headers=headers)
    with urllib.request.urlopen(req, timeout=20) as r:
        if r.status != 200:
            raise RuntimeError(f"S3 answered {r.status}")


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    with open(sys.argv[1], "rb") as f:
        body = f.read()
    json.loads(body)  # only a whole, valid file goes up
    put(body)
    print(f"published {sys.argv[1]} to https://{BUCKET}/{KEY}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
