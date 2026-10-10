// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const STATE_FILE = join(dirname(fileURLToPath(import.meta.url)), ".state", "stack.json");

/** What the setup started, for the tests and the teardown. */
export interface Stack {
  signalPid: number;
  hostPid: number;
  hostLog: string;
  config: string;
}

export function stack(): Stack {
  return JSON.parse(readFileSync(STATE_FILE, "utf8")) as Stack;
}

function hostLog(s: Stack): string {
  try {
    return readFileSync(s.hostLog, "utf8");
  } catch {
    return "";
  }
}

/** The host's current session code (the last one in its log), digits only. */
export function sessionCode(s: Stack = stack()): string {
  const all = [...hostLog(s).matchAll(/msg="session code" code="(\d{3} \d{3} \d{3})"/g)];
  return all.length ? all[all.length - 1]![1]!.replace(/ /g, "") : "";
}

/** Whether the host's log has a line matching re. */
export function hostLogged(re: RegExp, s: Stack = stack()): boolean {
  return re.test(hostLog(s));
}
