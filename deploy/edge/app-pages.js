// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// CloudFront viewer-request function on the app's pages (/web, /c, /link,
// /d/, /devices): they are routes of the single-page app, so they get
// index.html. Their cache behaviors keep the strict security headers (no
// Google Analytics), while the public pages allow it with consent.
function handler(event) {
  var request = event.request;
  request.uri = "/index.html";
  return request;
}
