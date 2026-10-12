// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// Strict Content Security Policy for the production build. connect-src
// allows any wss: host because users may pick their own signaling server.
// Google Analytics' hosts are here for the public pages (with consent,
// see src/analytics.ts); on the app's pages the server's CSP header leaves
// them out, and both policies apply. It is not applied in dev: Vite's hot
// reload needs inline scripts.
const GA = "https://*.googletagmanager.com";
const GA_HITS = "https://*.google-analytics.com https://*.analytics.google.com";
const CSP = [
  "default-src 'self'",
  `script-src 'self' ${GA}`,
  "style-src 'self'",
  "font-src 'self'",
  `img-src 'self' data: blob: ${GA_HITS} ${GA}`,
  `connect-src 'self' wss: ${GA_HITS} ${GA}`,
  "media-src 'self' blob:",
  "base-uri 'none'",
  "form-action 'self'",
  "object-src 'none'",
].join("; ");

function contentSecurityPolicy(): Plugin {
  return {
    name: "content-security-policy",
    apply: "build",
    transformIndexHtml(html) {
      // The build's version, to check which one a server is serving
      // (curl -s https://desk.go-link.org/ | grep go-link-desk-version).
      const version = (process.env.VITE_APP_VERSION || "dev").replace(/[^\w.+-]/g, "");
      return html.replace(
        "<head>",
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${CSP}" />\n    <meta name="go-link-desk-version" content="${version}" />`,
      );
    },
  };
}

export default defineConfig({
  plugins: [react(), contentSecurityPolicy()],
  server: {
    // The same port as go-link's web: the shared signalhub already allows
    // this origin for local development.
    port: 5180,
    strictPort: true,
  },
  // Source maps stay off the published files.
  build: {
    sourcemap: "hidden",
    // Small fonts would be inlined as data: URLs, which the CSP's
    // font-src 'self' blocks: they stay files.
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
  },
});
