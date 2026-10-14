// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { createHash } from "node:crypto";
import { createReadStream, mkdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { expect, test, type Page } from "@playwright/test";
import { hostHome, hostLogged, sessionCode, type HostName } from "../stack";

// The file sent: 32 MB in CI; E2E_BIG_FILE_MB=1024 checks the phase's 1 GB.
const SIZE_MB = Number(process.env.E2E_BIG_FILE_MB ?? 32);

async function join_(page: Page, host: HostName, name: string) {
  await page.goto(`/c/${sessionCode(host)}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.querySelector<HTMLVideoElement>("video.screen")?.videoWidth ?? 0), { timeout: 20_000 }).toBeGreaterThan(0);
}

const sha256 = (path: string) =>
  new Promise<string>((resolve, reject) => {
    const h = createHash("sha256");
    createReadStream(path).on("data", (d) => h.update(d)).on("end", () => resolve(h.digest("hex"))).on("error", reject);
  });

test("a big file goes to the host while the session keeps moving, and never over another", async ({ page }) => {
  test.setTimeout(Math.max(120_000, SIZE_MB * 1000));
  const src = join(tmpdir(), `go-link-desk-e2e-${SIZE_MB}mb.bin`);
  const chunk = Buffer.alloc(1 << 20);
  for (let i = 0; i < chunk.length; i++) chunk[i] = (i * 31) & 0xff;
  writeFileSync(src, Buffer.alloc(0));
  for (let i = 0; i < SIZE_MB; i++) writeFileSync(src, chunk, { flag: "a" });

  await join_(page, "control", "Sender");
  await page.getByRole("button", { name: "Files", exact: true }).click();
  // The picture's frames while the file moves: the session must not freeze.
  await page.evaluate(() => {
    const v = document.querySelector<HTMLVideoElement>("video.screen")!;
    const w = window as unknown as { frames_: number[] };
    w.frames_ = [];
    const tick = (now: number) => {
      w.frames_.push(now);
      v.requestVideoFrameCallback(tick);
    };
    v.requestVideoFrameCallback(tick);
  });
  const started = Date.now();
  await page.getByLabel("Choose files").setInputFiles(src);
  await expect(page.locator(".files-job.is-done")).toHaveCount(1, { timeout: Math.max(60_000, SIZE_MB * 900) });
  const seconds = (Date.now() - started) / 1000;
  const worstGap = await page.evaluate(() => {
    const f = (window as unknown as { frames_: number[] }).frames_;
    let worst = 0;
    for (let i = 1; i < f.length; i++) worst = Math.max(worst, f[i]! - f[i - 1]!);
    return { frames: f.length, worst };
  });
  console.log(`sent ${SIZE_MB} MB in ${seconds.toFixed(1)} s (${(SIZE_MB / seconds).toFixed(1)} MB/s); ${worstGap.frames} frames shown, longest gap ${Math.round(worstGap.worst)} ms`);
  // The test card sends 30 fps: no gap of a second or more while the file moved.
  expect(worstGap.worst).toBeLessThan(1000);

  const saved = join(hostHome("control"), "Downloads", `go-link-desk-e2e-${SIZE_MB}mb.bin`);
  expect(statSync(saved).size).toBe(SIZE_MB << 20);
  expect(await sha256(saved)).toBe(await sha256(src));
  await expect.poll(() => hostLogged(/msg="file received from the viewer"/, "control")).toBe(true);

  // The same name again: saved beside it, the first one untouched.
  const small = join(tmpdir(), "informe.txt");
  writeFileSync(small, "hola");
  await page.getByLabel("Choose files").setInputFiles(small);
  await page.getByLabel("Choose files").setInputFiles(small);
  await expect(page.locator(".files-job.is-done")).toHaveCount(3);
  await expect(page.locator(".files-job.is-done").last()).toContainText("informe (2).txt");
  await page.getByRole("button", { name: "Disconnect" }).click();
});

test("the viewer browses the host's home and downloads a file", async ({ page }) => {
  // The download is saved through the browser (the folder picker needs a person).
  await page.addInitScript(() => delete (window as unknown as { showSaveFilePicker?: unknown }).showSaveFilePicker);
  const docs = join(hostHome("control"), "Documents");
  mkdirSync(docs, { recursive: true });
  const content = "una línea del host\n".repeat(20000);
  writeFileSync(join(docs, "reporte.txt"), content);
  writeFileSync(join(hostHome("control"), ".oculto"), "x");

  await join_(page, "control", "Downloader");
  await page.getByRole("button", { name: "Files", exact: true }).click();
  await page.getByRole("tab", { name: "The computer's files" }).click();
  await expect(page.locator(".files-list")).not.toContainText(".oculto");
  await page.getByRole("button", { name: /Documents/ }).click();
  const downloaded = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download reporte.txt" }).click();
  const d = await downloaded;
  expect(d.suggestedFilename()).toBe("reporte.txt");
  const path = await d.path();
  expect(statSync(path).size).toBe(Buffer.byteLength(content));
  await expect.poll(() => hostLogged(/msg="file sent to the viewer" peer_id=\S+ name=reporte.txt/, "control")).toBe(true);
  await page.getByRole("button", { name: "Disconnect" }).click();
});

test("a viewer without the files permission gets no files", async ({ page }) => {
  await join_(page, "accept", "Watcher");
  await expect(page.getByRole("button", { name: "Files", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
