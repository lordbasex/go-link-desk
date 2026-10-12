// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test, type Page } from "@playwright/test";
import { hostLogCount, hostLogged, sessionCode, type HostName } from "../stack";

async function join(page: Page, host: HostName, name: string) {
  await page.goto(`/c/${sessionCode(host)}`);
  await page.getByLabel("Your name").fill(name);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("button", { name: "Disconnect" })).toBeVisible();
  // The picture is there: something to click on.
  await expect.poll(() => page.evaluate(() => document.querySelector("video")?.videoWidth ?? 0), { timeout: 20_000 }).toBeGreaterThan(0);
}

const injected = (re: RegExp) => hostLogged(re, "control");

test("a viewer allowed to control clicks, types and scrolls on the host", async ({ page }) => {
  await join(page, "control", "Controller");
  const video = page.locator("video");
  await video.click(); // the middle: the picture, not its bars
  await expect.poll(() => injected(/what=button button=0 down=true/)).toBe(true);
  await expect.poll(() => injected(/what=button button=0 down=false/)).toBe(true);

  // Keys go by position: KeyA is macOS's virtual key 0.
  await page.keyboard.press("KeyA");
  await expect.poll(() => injected(/what=key vk=0 down=true repeat=false/)).toBe(true);
  await expect.poll(() => injected(/what=key vk=0 down=false/)).toBe(true);
  // Shortcuts are not swallowed by the browser: Ctrl stays a key (0x3B).
  await page.keyboard.press("Control+KeyC");
  await expect.poll(() => injected(/what=key vk=59 down=true/)).toBe(true);
  await expect.poll(() => injected(/what=key vk=8 down=true repeat=false mods=2/)).toBe(true);

  await video.hover();
  await page.mouse.wheel(0, 120);
  await expect.poll(() => injected(/what=scroll dx=0 dy=120 lines=false/)).toBe(true);

  // Text typed out, as a paste would be.
  await page.getByRole("button", { name: "Keys" }).click();
  await page.getByRole("menuitem", { name: "Type text…" }).click();
  await page.getByLabel(/Paste or write the text/).fill("hola ñandú");
  await page.getByRole("button", { name: "Type it" }).click();
  await expect.poll(() => injected(/what=text text="hola ñandú"/)).toBe(true);

  // Leaving the page lets go of everything on the host.
  const before = hostLogCount(/what=release_all/, "control");
  await page.evaluate(() => window.dispatchEvent(new Event("blur")));
  await expect.poll(() => hostLogCount(/what=release_all/, "control")).toBeGreaterThan(before);
  await page.getByRole("button", { name: "Disconnect" }).click();
});

test("a viewer that may only watch gets no keyboard and sends nothing", async ({ page }) => {
  await join(page, "accept", "Watcher");
  await expect(page.getByRole("button", { name: "Keys" })).toHaveCount(0);
  await expect(page.getByRole("menuitemcheckbox", { name: "Game mode" })).toHaveCount(0);
  await page.getByRole("button", { name: "Disconnect" }).click();
});
