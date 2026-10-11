// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const STATE_FILE = join(dirname(fileURLToPath(import.meta.url)), ".state", "stack.json");

/** One headless host the setup started. */
export interface HostProc {
  pid: number;
  log: string;
  config: string;
}

/** What the setup started, for the tests and the teardown. */
export interface Stack {
  signalPid: number;
  hosts: {
    /** Accepts every viewer (--test-auto-accept). */
    accept: HostProc;
    /** Nobody to ask: turns every viewer down. */
    reject: HostProc;
    /**
     * Accepts every viewer with everything (control, sound, clipboard),
     * logs its input instead of injecting it, and keeps its clipboard in
     * clipboard.txt next to its log.
     */
    control: HostProc;
  };
}

export type HostName = keyof Stack["hosts"];

export function stack(): Stack {
  return JSON.parse(readFileSync(STATE_FILE, "utf8")) as Stack;
}

/** A host's whole log so far. */
export function hostLog(host: HostName = "accept", s: Stack = stack()): string {
  try {
    return readFileSync(s.hosts[host].log, "utf8");
  } catch {
    return "";
  }
}

/** A host's current session code (the last one in its log), digits only. */
export function sessionCode(host: HostName = "accept", s: Stack = stack()): string {
  const all = [...hostLog(host, s).matchAll(/msg="session code" code="(\d{3} \d{3} \d{3})"/g)];
  return all.length ? all[all.length - 1]![1]!.replace(/ /g, "") : "";
}

/** Whether a host's log has a line matching re. */
export function hostLogged(re: RegExp, host: HostName = "accept", s: Stack = stack()): boolean {
  return re.test(hostLog(host, s));
}

/** How many lines of a host's log match re. */
export function hostLogCount(re: RegExp, host: HostName = "accept", s: Stack = stack()): number {
  return hostLog(host, s).split("\n").filter((l) => re.test(l)).length;
}

/** The control host's clipboard file (--test-clipboard-file). */
export function hostClipboardFile(s: Stack = stack()): string {
  return join(dirname(s.hosts.control.log), "clipboard.txt");
}

/** A host's home folder (the setup sandboxes HOME per host). */
export function hostHome(host: HostName, s: Stack = stack()): string {
  return join(dirname(s.hosts[host].log), "home");
}
