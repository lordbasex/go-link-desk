// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync } from "node:fs";
import { STATE_FILE, type Stack } from "./stack";

/** Stops the hosts and the signalhub the setup started. */
export default async function globalTeardown() {
  let s: Stack;
  try {
    s = JSON.parse(readFileSync(STATE_FILE, "utf8")) as Stack;
  } catch {
    return;
  }
  for (const pid of [s.hosts.accept.pid, s.hosts.reject.pid, s.signalPid]) {
    try {
      process.kill(pid, "SIGTERM");
    } catch {
      // already gone
    }
  }
}
