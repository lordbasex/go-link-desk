// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { defineConfig } from "@playwright/test";
import { PORTS, SIGNAL_URL } from "./ports";

// A real Chromium against a real signalhub, a real headless host and the
// website, all started by global-setup.ts on their own ports, so nothing of
// the developer's (signalhub, host) is touched. The steps depend on each
// other, so they run in order.
export default defineConfig({
  testDir: "tests",
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 20_000 },
  retries: 0,
  reporter: [["list"], ["html", { open: "never" }]],
  globalSetup: "./global-setup.ts",
  globalTeardown: "./global-teardown.ts",
  use: {
    baseURL: `http://localhost:${PORTS.web}`,
    locale: "en-US",
    trace: "retain-on-failure",
    launchOptions: {
      // A fake camera and microphone, allowed without asking, for the video call.
      args: ["--autoplay-policy=no-user-gesture-required", "--mute-audio", "--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"],
    },
  },
  projects: [{ name: "chromium" }],
  webServer: [
    {
      // The website in dev mode (no CSP, so ws:// to the local signalhub works).
      command: `npm --prefix ../frontend run dev -w @go-link-desk/web -- --port ${PORTS.web} --strictPort`,
      url: `http://localhost:${PORTS.web}`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { VITE_SIGNAL_URL: SIGNAL_URL },
    },
  ],
});
