// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { spawn } from "node:child_process";
import { closeSync, mkdirSync, openSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { PORTS, SIGNAL_URL } from "./ports";
import type { HostProc } from "./stack";

const here = dirname(fileURLToPath(import.meta.url));
/** Where the setup builds the host, and where each host keeps its files. */
export const BIN = join(here, ".bin");
export const STATE = join(here, ".state");

/**
 * Starts a headless host of the test stack in .state/<name>, with a
 * throwaway HOME (its config and logs never mix with the developer's).
 * Starting it again with the same name keeps its device.json: the same
 * computer after a restart.
 */
export function spawnHost(name: string, extra: string[]): HostProc {
  const dir = join(STATE, name);
  mkdirSync(join(dir, "home"), { recursive: true });
  const config = join(dir, "device.json");
  const log = join(dir, "host.log");
  const out = openSync(log, "a");
  const host = spawn(
    join(BIN, "desk"),
    [
      "--headless",
      "--config", config,
      "--server-signaling", SIGNAL_URL,
      "--web-url", `http://localhost:${PORTS.web}`,
      "--source", "testpattern",
      ...extra,
      ...(process.env.E2E_HOST_DEBUG === "1" ? ["--debug"] : []),
    ],
    { env: { ...process.env, HOME: join(dir, "home") }, stdio: ["ignore", out, out], detached: true },
  );
  closeSync(out);
  host.unref();
  return { pid: host.pid!, log, config };
}

/** Stops a host and waits until it is gone (its lock and its signalhub session with it). */
export async function stopHost(h: HostProc): Promise<void> {
  try {
    process.kill(h.pid, "SIGTERM");
  } catch {
    return;
  }
  const end = Date.now() + 15_000;
  while (Date.now() < end) {
    try {
      process.kill(h.pid, 0);
    } catch {
      return;
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  process.kill(h.pid, "SIGKILL");
}
