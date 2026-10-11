// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Page } from "@playwright/test";
import { hostLogCount, hostLogged, sessionCode } from "../stack";

async function join(page: Page, name: string) {
  await page.goto(`/c/${sessionCode("control")}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.querySelector<HTMLVideoElement>("video.screen")?.videoWidth ?? 0), { timeout: 20_000 }).toBeGreaterThan(0);
}

test("a video call both ways: the host sees and hears the viewer, the viewer sees and hears the host", async ({ page, context }) => {
  await context.grantPermissions(["camera", "microphone"]);
  await join(page, "Caller");
  const started = hostLogCount(/msg="call started"/, "control");
  await page.getByRole("button", { name: "Video call" }).click();

  // The host (auto-accepting in tests) takes the call and starts its camera and microphone.
  await expect.poll(() => hostLogCount(/msg="call started"/, "control")).toBeGreaterThan(started);
  // The browser's fake camera reaches the host's decoder (VideoToolbox: macOS
  // hosts; Windows and Linux decoders come in phase 7), and its microphone
  // the speaker.
  if (process.platform === "darwin") {
    await expect.poll(() => hostLogged(/msg="call: the viewer's camera shows"/, "control"), { timeout: 15_000 }).toBe(true);
  }
  await expect.poll(() => hostLogged(/msg="call: the viewer's voice plays"/, "control"), { timeout: 15_000 }).toBe(true);

  // The host's camera (the test card) shows here, and its microphone (a tone) sounds.
  const remote = page.getByLabel("The other side's camera");
  await expect.poll(() => remote.evaluate((v: HTMLVideoElement) => v.videoWidth), { timeout: 15_000 }).toBe(640);
  const loud = await page.evaluate(async () => {
    const a = document.querySelector(".call-panel audio") as HTMLAudioElement;
    const stream = a.srcObject as MediaStream;
    const ctx = new AudioContext();
    const an = ctx.createAnalyser();
    ctx.createMediaStreamSource(stream).connect(an);
    let best = 0;
    for (let i = 0; i < 10 && best < 0.05; i++) {
      await new Promise((r) => setTimeout(r, 300));
      const buf = new Float32Array(an.fftSize);
      an.getFloatTimeDomainData(buf);
      best = Math.max(best, Math.sqrt(buf.reduce((s, v) => s + v * v, 0) / buf.length));
    }
    await ctx.close();
    return best;
  });
  expect(loud).toBeGreaterThan(0.05);

  // A click on the call's video is the page's, never a click on the host.
  const clicks = hostLogCount(/what=button/, "control");
  await remote.click();
  expect(hostLogCount(/what=button/, "control")).toBe(clicks);

  await page.getByRole("button", { name: "Hang up" }).click();
  await expect.poll(() => hostLogged(/msg="call ended"/, "control")).toBe(true);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
