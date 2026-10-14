// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { hostLogged, sessionCode, type HostName } from "../stack";

async function join(page: Page, host: HostName, name: string) {
  await page.goto(`/c/${sessionCode(host)}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.querySelector("video")?.videoWidth ?? 0), { timeout: 20_000 }).toBeGreaterThan(0);
}

test("a viewer allowed to record saves a screenshot and a recording, and the host is told first", async ({ page, browserName }) => {
  test.skip(browserName !== "chromium", "MediaRecorder's WebM is checked in Chromium");
  // The browser keeps the recording and saves it at the end (no file picker to click).
  await page.addInitScript(() => delete (window as { showSaveFilePicker?: unknown }).showSaveFilePicker);
  await join(page, "control", "Recorder");
  const menu = page.getByRole("button", { name: "Screenshot and recording" });

  await menu.click();
  const shot = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "Take a screenshot" }).click();
  const png = await shot;
  expect(png.suggestedFilename()).toMatch(/^go-link-desk-[^ ]+\.png$/);
  expect(readFileSync((await png.path())!).subarray(1, 4).toString()).toBe("PNG");
  await expect.poll(() => hostLogged(/msg="the viewer saved a screenshot"/, "control")).toBe(true);

  await menu.click();
  await page.getByRole("menuitem", { name: "Record the screen" }).click();
  const pill = page.getByRole("button", { name: /^Recording \d+:\d\d, click to stop$/ });
  await expect(pill).toBeVisible();
  await expect.poll(() => hostLogged(/msg="the viewer is recording the screen"/, "control")).toBe(true);
  await page.waitForTimeout(2500);
  const rec = page.waitForEvent("download");
  await pill.click();
  const webm = await rec;
  expect(webm.suggestedFilename()).toMatch(/^go-link-desk-[^ ]+\.webm$/);
  const bytes = readFileSync((await webm.path())!);
  expect(bytes.subarray(0, 4).toString("hex")).toBe("1a45dfa3"); // EBML: a WebM file
  expect(bytes.length).toBeGreaterThan(10_000);
  await expect.poll(() => hostLogged(/msg="the viewer stopped recording the screen"/, "control")).toBe(true);
  await page.getByRole("button", { name: "Disconnect" }).click();
});

test("a viewer without the permission has no screenshot or recording", async ({ page }) => {
  await join(page, "accept", "Watcher");
  await expect(page.getByRole("button", { name: "Screenshot and recording" })).toHaveCount(0);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
