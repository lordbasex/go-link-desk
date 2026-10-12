// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { readFileSync, writeFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { hostClipboardFile, hostLogged, sessionCode, type HostName } from "../stack";

async function join(page: Page, host: HostName, name: string) {
  await page.goto(`/c/${sessionCode(host)}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.querySelector("video")?.videoWidth ?? 0), { timeout: 20_000 }).toBeGreaterThan(0);
}

/** The loudness of the page's sound element over half a second (RMS, 0..1). */
const loudness = (page: Page) =>
  page.evaluate(async () => {
    const audio = document.querySelector("audio");
    const stream = audio?.srcObject as MediaStream | null;
    if (!stream || stream.getAudioTracks().length === 0) return 0;
    const ctx = new AudioContext();
    const analyser = ctx.createAnalyser();
    ctx.createMediaStreamSource(stream).connect(analyser);
    await new Promise((r) => setTimeout(r, 500));
    const buf = new Float32Array(analyser.fftSize);
    analyser.getFloatTimeDomainData(buf);
    await ctx.close();
    return Math.sqrt(buf.reduce((s, v) => s + v * v, 0) / buf.length);
  });

test("a viewer allowed to hear gets the test card's tone, one that may not gets silence", async ({ page, browser }) => {
  await join(page, "control", "Listener");
  await expect(page.getByRole("button", { name: /Sound on/ })).toBeVisible();
  await expect.poll(() => loudness(page), { timeout: 15_000 }).toBeGreaterThan(0.05);
  // Muted here: the element is muted, the host keeps sending.
  await page.getByRole("button", { name: /Sound on/ }).click();
  await expect.poll(() => page.evaluate(() => document.querySelector("audio")?.muted)).toBe(true);
  await page.getByRole("button", { name: "Disconnect" }).click();

  const watcher = await browser.newPage();
  await join(watcher, "accept", "Watcher");
  await expect(watcher.getByRole("button", { name: /Sound on|Sound muted/ })).toHaveCount(0);
  expect(await loudness(watcher)).toBeLessThan(0.001);
  await watcher.getByRole("button", { name: "Disconnect" }).click();
  await watcher.close();
});

test("the clipboard goes both ways: copied on the host lands here, pasted here lands there", async ({ page, context, browserName }) => {
  test.skip(browserName !== "chromium", "Playwright grants clipboard access only in Chromium");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const file = hostClipboardFile();
  writeFileSync(file, "before the session");
  await join(page, "control", "Clipboard");

  // Copied on the host (its clipboard file changes): this page's clipboard gets it.
  writeFileSync(file, "copiado en el host ñ 日本");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText()), { timeout: 10_000 }).toBe("copiado en el host ñ 日本");

  // Pasted here: the host's clipboard gets it, and the host presses its paste
  // shortcut (V is macOS virtual key 9 in the log): Cmd+V on a Mac, Ctrl+V elsewhere.
  await page.evaluate(() => navigator.clipboard.writeText("pegado desde el navegador"));
  await page.locator("video").click();
  await page.keyboard.press("ControlOrMeta+KeyV");
  await expect.poll(() => readFileSync(file, "utf8"), { timeout: 10_000 }).toBe("pegado desde el navegador");
  const pasteMods = process.platform === "darwin" ? 8 : 2;
  await expect.poll(() => hostLogged(new RegExp(`what=key vk=9 down=true repeat=false mods=${pasteMods}`), "control")).toBe(true);
  // The host's own write does not come back.
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("pegado desde el navegador");
  await page.getByRole("button", { name: "Disconnect" }).click();
});

test("the viewer chats with the person at the host", async ({ page }) => {
  await join(page, "accept", "Chatter");
  await page.getByRole("button", { name: "Chat" }).click();
  const box = page.getByLabel("Write a message");
  await box.fill("hola, ¿me ves?");
  await box.press("Enter");
  await expect(page.locator(".chat-line.from-me .chat-text")).toHaveText("hola, ¿me ves?");
  await expect.poll(() => hostLogged(/msg="chat message from the viewer" peer_id=\S+ chars=14/, "accept")).toBe(true);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
