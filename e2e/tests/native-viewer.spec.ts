// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFile } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { BIN, STATE, spawnHost, stopHost } from "../host";
import { SIGNAL_URL } from "../ports";
import { sessionCode, type HostProc } from "../stack";

/** The app's own viewer, without a window: the binary with --test-viewer. */
interface ViewerRun {
  ok: boolean;
  log: string;
}

const VIEWER = join(STATE, "native-viewer");

function viewer(door: "join" | "claim" | "reach", extra: string[]): Promise<ViewerRun> {
  mkdirSync(join(VIEWER, "home"), { recursive: true });
  return new Promise((resolve) => {
    execFile(
      join(BIN, "desk"),
      ["--config", join(VIEWER, "device.json"), "--server-signaling", SIGNAL_URL, "--test-viewer", door, "--test-viewer-frames", "20", ...extra],
      { env: { ...process.env, HOME: join(VIEWER, "home") }, timeout: 90_000 },
      (err, stdout, stderr) => resolve({ ok: !err, log: stdout + stderr }),
    );
  });
}

function pairingCode(h: HostProc): string {
  const all = [...readFileSync(h.log, "utf8").matchAll(/msg="pairing code" code="(\d{3} \d{3} \d{3})"/g)];
  return all.at(-1)?.[1]?.replace(/ /g, "") ?? "";
}

test("the app's viewer joins a computer with its code and gets the picture", async () => {
  test.setTimeout(120_000);
  const run = await viewer("join", ["--test-viewer-code", sessionCode("accept")]);
  expect(run.ok, run.log).toBe(true);
  expect(run.log).toMatch(/msg="viewer frames" frames=20/);
  expect(run.log).toMatch(/status=ended reason=viewer/);
});

test("the app's viewer links with the pairing code, then comes back without one, with the password", async () => {
  test.setTimeout(180_000);
  rmSync(VIEWER, { recursive: true, force: true });
  const host = spawnHost("native-unattended", ["--test-auto-accept", "--test-unattended", "--test-unattended-password", "correct horse"]);
  try {
    await expect.poll(() => pairingCode(host), { timeout: 30_000 }).not.toBe("");

    // Linking: the person there (the test's auto-accept) accepts; the
    // link is kept in remotes.json, with this app's private key and the
    // host's pinned public key.
    const claim = await viewer("claim", ["--test-viewer-code", pairingCode(host)]);
    expect(claim.ok, claim.log).toBe(true);
    expect(claim.log).toMatch(/msg="viewer linked"/);
    const remotes = JSON.parse(readFileSync(join(VIEWER, "remotes.json"), "utf8")) as { devices: { device_id: string; key: string; host_key: string }[] };
    expect(remotes.devices).toHaveLength(1);
    expect(remotes.devices[0]!.key).toBeTruthy();
    expect(remotes.devices[0]!.host_key).toBeTruthy();
    // The host keeps only the browser's public key, never this app's private key.
    expect(readFileSync(host.config, "utf8")).not.toContain(remotes.devices[0]!.key);

    // Coming back without a code: a wrong password is refused...
    const wrong = await viewer("reach", ["--test-viewer-password", "wrong horse"]);
    expect(wrong.ok).toBe(false);
    expect(wrong.log).toMatch(/status=password reason="" password_error=wrong/);

    // ...the right one lets it in, nobody asked.
    const right = await viewer("reach", ["--test-viewer-password", "correct horse"]);
    expect(right.ok, right.log).toBe(true);
    expect(right.log).toMatch(/msg="viewer frames" frames=20/);
    expect(readFileSync(host.log, "utf8")).toMatch(/msg="unattended session started"/);
  } finally {
    await stopHost(host);
  }
});
