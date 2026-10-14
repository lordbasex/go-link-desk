// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync, spawn } from "node:child_process";
import { closeSync, mkdirSync, openSync, rmSync, writeFileSync } from "node:fs";
import { BIN, STATE, spawnHost } from "./host";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { PORTS } from "./ports";
import { STATE_FILE, sessionCode, type HostProc, type Stack } from "./stack";

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
 * Builds and starts the test stack: a signalhub and two headless hosts of
 * their own, each with a throwaway HOME (their config and logs never mix
 * with the developer's).
 */
export default async function globalSetup() {
  const bin = BIN;
  const state = STATE;
  rmSync(state, { recursive: true, force: true });
  mkdirSync(bin, { recursive: true });
  mkdirSync(state, { recursive: true });

  // signalhub is its own repository, used unchanged: next to this one, or
  // where SIGNALING_DIR says.
  const signaling = process.env.SIGNALING_DIR || join(root, "..", "signaling");
  execFileSync("go", ["build", "-o", join(bin, "signal"), "./cmd/signal"], { cwd: signaling, stdio: "inherit" });
  execFileSync("go", ["build", "-tags", "headless", "-o", join(bin, "desk"), "./cmd/desk"], {
    cwd: join(root, "backend-device"),
    stdio: "inherit",
  });
  // go-link-desk-terminal: pure Go on Linux, as it ships (a Mac builds it with cgo).
  execFileSync("go", ["build", "-tags", "headless", "-o", join(bin, "terminal"), "./cmd/terminal"], {
    cwd: join(root, "backend-device"),
    stdio: "inherit",
    env: process.platform === "linux" ? { ...process.env, CGO_ENABLED: "0" } : process.env,
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

  // Three hosts: one accepts every viewer (--test-auto-accept, test card
  // only), one has no window and nobody to ask, so it turns every viewer
  // down, and one accepts everything and logs its input.
  const startHost = (name: keyof Stack["hosts"], extra: string[]): HostProc => spawnHost(name, extra);
  const s: Stack = {
    signalPid: signal.pid!,
    hosts: {
      accept: startHost("accept", ["--test-auto-accept"]),
      reject: startHost("reject", []),
      control: startHost("control", ["--test-auto-accept", "--test-input-log", "--test-clipboard-file", join(state, "control", "clipboard.txt")]),
    },
  };
  writeFileSync(STATE_FILE, JSON.stringify(s));
  await waitFor("the hosts' session codes", async () => sessionCode("accept", s) !== "" && sessionCode("reject", s) !== "" && sessionCode("control", s) !== "");
  signal.unref();
}
