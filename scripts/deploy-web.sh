#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Publishes the website's build (frontend/apps/web/dist) to desk.go-link.org:
# an S3 bucket behind CloudFront. Hashed assets are cached for a year; the
# pages are revalidated on every visit, so a deploy shows at once.
#
#   scripts/deploy-web.sh                 (after make web-build)
#
# Settings (environment): AWS_PROFILE (go-link-desk, a user that may only
# write this bucket and invalidate this distribution), DESK_BUCKET,
# DESK_DISTRIBUTION.
set -euo pipefail

DIST="${1:-frontend/apps/web/dist}"
PROFILE="${AWS_PROFILE:-go-link-desk}"
BUCKET="${DESK_BUCKET:-desk.go-link.org}"
DISTRIBUTION="${DESK_DISTRIBUTION:-E17PYCRG29SFOD}"

[ -f "$DIST/index.html" ] || { echo "no build in $DIST: run make web-build first" >&2; exit 1; }
if find "$DIST" -name '*.map' | grep -q .; then
  echo "source maps in $DIST: make web-build removes them" >&2
  exit 1
fi

aws() { command aws --profile "$PROFILE" "$@"; }

# Assets first (their names change with their content), then the pages
# that point to them, so no page ever points to an asset not yet there.
aws s3 sync "$DIST/assets" "s3://$BUCKET/assets" --cache-control "public, max-age=31536000, immutable" --only-show-errors
aws s3 sync "$DIST" "s3://$BUCKET" --exclude "assets/*" --cache-control "no-cache" --only-show-errors
# Old assets stay a while for pages already open; the rest goes.
# demo.json is the demonstration host's, not the build's (deploy/demo).
aws s3 sync "$DIST" "s3://$BUCKET" --delete --exclude "assets/*" --exclude "demo.json" --only-show-errors

id=$(aws cloudfront create-invalidation --distribution-id "$DISTRIBUTION" --paths "/*" --query Invalidation.Id --output text)
echo "published to https://$BUCKET (invalidation $id)"
