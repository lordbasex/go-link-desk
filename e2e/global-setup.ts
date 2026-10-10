// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync, spawn } from "node:child_process";
import { closeSync, mkdirSync, openSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { PORTS, SIGNAL_URL } from "./ports";
import { STATE_FILE, sessionCode, type Stack } from "./stack";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");

async function waitFor(what: string, ok: () => Promise<boolean>, ms = 60_000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (await ok().catch(() => false)) return;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`timed out waiting for ${what}`);
}

/**
 * Builds and starts the test stack: a signalhub and a headless host of
 * their own, the host with a throwaway HOME (its config and logs never mix
 * with the developer's).
 */
export default async function globalSetup() {
  const bin = join(here, ".bin");
  const state = join(here, ".state");
  rmSync(state, { recursive: true, force: true });
  mkdirSync(bin, { recursive: true });
  mkdirSync(join(state, "home"), { recursive: true });

  // signalhub is its own repository, used unchanged: next to this one, or
  // where SIGNALING_DIR says.
  const signaling = process.env.SIGNALING_DIR || join(root, "..", "signaling");
  execFileSync("go", ["build", "-o", join(bin, "signal"), "./cmd/signal"], { cwd: signaling, stdio: "inherit" });
  execFileSync("go", ["build", "-tags", "headless", "-o", join(bin, "desk"), "./cmd/desk"], {
    cwd: join(root, "backend-device"),
    stdio: "inherit",
  });

  const signalLog = openSync(join(state, "signal.log"), "a");
  const signal = spawn(join(bin, "signal"), [], {
    env: {
      ...process.env,
      ADDR: `127.0.0.1:${PORTS.signal}`,
      ALLOWED_ORIGINS: [`http://localhost:${PORTS.web}`, `http://127.0.0.1:${PORTS.web}`].join(","),
      // The production server lists both apps; the test one only needs ours.
      ALLOWED_APPS: "go-link-desk",
      RATE_LIMIT_PER_MIN: "1000",
      OWNER_RATE_PER_MIN: "1000",
      // Every test connects from 127.0.0.1: the per-IP cap must not throttle the suite.
      MAX_CONNS_PER_IP: "500",
      LOG_LEVEL: "warn",
    },
    stdio: ["ignore", signalLog, signalLog],
    detached: true,
  });
  closeSync(signalLog);
  await waitFor("signalhub", async () => (await fetch(`http://127.0.0.1:${PORTS.signal}/healthz`)).ok);

  const config = join(state, "device.json");
  const hostLog = join(state, "host.log");
  const out = openSync(hostLog, "a");
  const host = spawn(
    join(bin, "desk"),
    [
      "--headless",
      "--config", config,
      "--server-signaling", SIGNAL_URL,
      "--web-url", `http://localhost:${PORTS.web}`,
      "--source", "testpattern",
      ...(process.env.E2E_HOST_DEBUG === "1" ? ["--debug"] : []),
    ],
    { env: { ...process.env, HOME: join(state, "home") }, stdio: ["ignore", out, out], detached: true },
  );
  closeSync(out);
  const s: Stack = { signalPid: signal.pid!, hostPid: host.pid!, hostLog, config };
  writeFileSync(STATE_FILE, JSON.stringify(s));
  await waitFor("the host's session code", async () => sessionCode(s) !== "");
  signal.unref();
  host.unref();
}
