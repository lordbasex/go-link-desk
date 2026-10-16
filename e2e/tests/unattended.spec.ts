// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync, writeFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { spawnHost, stopHost } from "../host";
import type { HostProc } from "../stack";

/** A host of this spec's own, which it restarts. */
const NAME = "unattended";
// Unattended access always has a password.
const unattended = ["--test-auto-accept", "--test-unattended", "--test-unattended-password", "correct horse"];

function log(h: HostProc): string {
  try {
    return readFileSync(h.log, "utf8");
  } catch {
    return "";
  }
}

/** How many pairing codes the host got so far: a new one means it is registered again. */
function codes(h: HostProc): string[] {
  return [...log(h).matchAll(/msg="pairing code" code="(\d{3} \d{3} \d{3})"/g)].map((m) => m[1]!.replace(/ /g, ""));
}

async function start(extra: string[], before = 0, name = NAME): Promise<HostProc> {
  const h = spawnHost(name, extra);
  try {
    await expect.poll(() => codes(h).length, { timeout: 30_000 }).toBeGreaterThan(before);
  } catch (err) {
    await stopHost(h);
    throw err;
  }
  return h;
}

async function framesDecoded(page: Page): Promise<number> {
  const details = page.getByRole("button", { name: "Session details" });
  if ((await details.getAttribute("aria-expanded")) !== "true") await details.click();
  const text = await page.locator(".stream-info-row", { hasText: "Frames decoded" }).locator("dd").textContent();
  return Number(text) || 0;
}

async function live(page: Page) {
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect.poll(() => framesDecoded(page), { timeout: 30_000 }).toBeGreaterThan(0);
}

test("a linked browser comes back without a code after the host restarts, with the password; a revoked one is turned away", async ({ page }) => {
  test.setTimeout(180_000);
  let host = await start(unattended);
  try {
    // Link this browser with the pairing code; the person there (here the
    // test's auto-accept) accepts, and the browser keeps its token.
    await page.goto(`/link/${codes(host).at(-1)}`);
    await page.getByLabel("Your name").fill("Linked viewer");
    await page.getByRole("button", { name: "Connect" }).click();
    await live(page);
    const devices = await page.evaluate(() => JSON.parse(localStorage.getItem("go-link-desk.devices") ?? "[]") as { deviceId: string; hostKey?: string }[]);
    expect(devices).toHaveLength(1);
    const deviceId = devices[0]!.deviceId;
    // The browser keeps the host's public key and no secret at all; its own
    // private key is a non-extractable WebCrypto key in IndexedDB.
    expect(devices[0]!.hostKey).toBeTruthy();
    // Only the browser's public key is on the host, never anything private.
    const saved = JSON.parse(readFileSync(host.config, "utf8")) as { links: { browser_key: string; permissions: { control: boolean } }[] };
    expect(saved.links).toHaveLength(1);
    expect(saved.links[0]!.browser_key).toBeTruthy();
    await page.getByRole("button", { name: "Disconnect" }).click();
    await expect(page.getByText("You left the session.")).toBeVisible();

    // The host restarts: My computers takes the browser back, no code,
    // nobody asked; the link alone is not enough, the password goes too.
    let seen = codes(host).length;
    await stopHost(host);
    host = await start(unattended, seen);
    await page.goto("/web");
    await page.getByRole("link", { name: /^Connect to / }).first().click();
    await expect(page).toHaveURL(new RegExp(`/d/${deviceId}$`));
    const password = page.getByLabel("Password of that computer");
    await password.fill("wrong guess");
    await page.getByRole("button", { name: "Connect" }).click();
    await expect(page.getByText("Wrong password. 4 tries left.")).toBeVisible();
    await password.fill("correct horse");
    await page.getByRole("button", { name: "Connect" }).click();
    await live(page);
    await page.getByRole("button", { name: "Disconnect" }).click();

    // Revoked on the host (Settings › Revoke drops it from device.json):
    // its token opens nothing, and this browser forgets it.
    seen = codes(host).length;
    await stopHost(host);
    const cfg = JSON.parse(readFileSync(host.config, "utf8")) as Record<string, unknown>;
    delete cfg.links;
    writeFileSync(host.config, JSON.stringify(cfg));
    host = await start(unattended, seen);
    await page.goto(`/d/${deviceId}`);
    await expect(page.getByText("That computer no longer has this browser linked.", { exact: false })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem("go-link-desk.devices"))).toBe("[]");
    expect(log(host).split("unattended session started").length - 1).toBe(1);
  } finally {
    await stopHost(host);
  }
});

test("without unattended access, a linking code links nothing", async ({ page }) => {
  const host = await start([], 0, "unattended-off");
  try {
    await page.goto(`/link/${codes(host).at(-1)}`);
    await page.getByLabel("Your name").fill("Nobody");
    await page.getByRole("button", { name: "Connect" }).click();
    await expect(page.getByText("That computer does not allow unattended access now.", { exact: false })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem("go-link-desk.devices"))).toBeNull();
  } finally {
    await stopHost(host);
  }
});
