// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Page } from "@playwright/test";
import { hostLogCount, hostLogged, sessionCode, type HostName } from "../stack";

/** Opens a host's code, gives a name and knocks. */
async function knock(page: Page, host: HostName, name: string) {
  await page.goto(`/c/${sessionCode(host)}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
}

/** The frames the browser decoded so far, as the session details show them. */
async function framesDecoded(page: Page): Promise<number> {
  const details = page.getByRole("button", { name: "Session details" });
  if ((await details.getAttribute("aria-expanded")) !== "true") await details.click();
  const text = await page.locator(".stream-info-row", { hasText: "Frames decoded" }).locator("dd").textContent();
  return Number(text) || 0;
}

const ended = /msg="session ended" .*reason=/;

test("an accepted viewer sees the host's screen", async ({ page }) => {
  const before = hostLogCount(ended);
  await knock(page, "accept", "E2E viewer");
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  await expect(page.getByText("Screen of ")).toBeVisible();
  await expect.poll(() => framesDecoded(page), { timeout: 20_000 }).toBeGreaterThan(0);
  expect(hostLogged(/msg="session started" .*name="E2E viewer"/)).toBe(true);

  // Disconnect ends it on both sides.
  await page.getByRole("button", { name: "Disconnect" }).click();
  await expect(page.getByText("You left the session.")).toBeVisible();
  await expect.poll(() => hostLogCount(ended)).toBe(before + 1);
});

test("one session at a time: a second viewer is turned away", async ({ browser }) => {
  const first = await browser.newPage();
  const second = await browser.newPage();
  try {
    await knock(first, "accept", "First");
    await expect.poll(() => framesDecoded(first), { timeout: 20_000 }).toBeGreaterThan(0);
    await knock(second, "accept", "Second");
    await expect(second.getByText("That computer is in another session right now.")).toBeVisible();
  } finally {
    await first.close();
    await second.close();
  }
});

test("a viewer that leaves ends its session (peer_left)", async ({ browser }) => {
  const before = hostLogCount(/msg="session ended" .*reason=viewer/);
  const page = await browser.newPage();
  await knock(page, "accept", "Leaver");
  await expect.poll(() => framesDecoded(page), { timeout: 20_000 }).toBeGreaterThan(0);
  // Closing the tab says nothing: signalhub tells the host the viewer left.
  await page.close();
  await expect.poll(() => hostLogCount(/msg="session ended" .*reason=viewer/)).toBe(before + 1);

  // The host is free again.
  const next = await browser.newPage();
  try {
    await knock(next, "accept", "Next");
    await expect.poll(() => framesDecoded(next), { timeout: 20_000 }).toBeGreaterThan(0);
  } finally {
    await next.close();
  }
});

test("a host with nobody to ask turns the viewer down", async ({ page }) => {
  await knock(page, "reject", "Mallory");
  await expect(page.getByText("Nobody can accept sessions at that computer right now.")).toBeVisible();
  expect(hostLogged(/msg="viewer turned down" .*reason=unavailable/, "reject")).toBe(true);
  expect(hostLogged(/msg="session started"/, "reject")).toBe(false);
  // And the page offers to try again or use another code.
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
});

test("the session details measure the latency with the beacon", async ({ page }) => {
  await knock(page, "accept", "Latency");
  await expect.poll(() => framesDecoded(page), { timeout: 20_000 }).toBeGreaterThan(0);
  // The test card draws the beacon itself: the whole path but the screen capture.
  await page.getByRole("button", { name: "Measure latency" }).click();
  const cell = page.locator(".stream-info-row", { hasText: "Screen latency" }).locator("dd");
  await expect(cell).toHaveText(/^\d+ ms$/, { timeout: 30_000 });
  const ms = Number((await cell.textContent())!.replace(/\D/g, ""));
  expect(ms).toBeLessThan(1000);
  await expect(page.locator(".stream-info-row", { hasText: "Codec" }).locator("dd")).toHaveText(/^(H\.264|VP8)$/);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
