// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { connect, type Socket } from "node:net";
import { dirname, join } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { BIN, spawnTerminal, stopHost } from "../host";
import type { HostProc } from "../stack";

const PASSWORD = "correct horse";

function log(h: HostProc): string {
  try {
    return readFileSync(h.log, "utf8");
  } catch {
    return "";
  }
}

/** The service's commands' socket, played by the test: one JSON line each way. */
class Commands {
  private buf = "";
  private lines: Record<string, unknown>[] = [];
  private constructor(private readonly sock: Socket) {
    sock.setEncoding("utf8");
    sock.on("data", (d: string) => {
      this.buf += d;
      let i;
      while ((i = this.buf.indexOf("\n")) >= 0) {
        this.lines.push(JSON.parse(this.buf.slice(0, i)) as Record<string, unknown>);
        this.buf = this.buf.slice(i + 1);
      }
    });
  }
  static open(h: HostProc): Promise<Commands> {
    return new Promise((resolve, reject) => {
      const sock = connect(join(dirname(h.config), "control.sock"), () => resolve(new Commands(sock)));
      sock.on("error", reject);
    });
  }
  send(v: unknown) {
    this.sock.write(JSON.stringify(v) + "\n");
  }
  async next(has: string): Promise<Record<string, unknown>> {
    await expect.poll(() => this.lines.some((l) => has in l), { timeout: 30_000 }).toBe(true);
    const i = this.lines.findIndex((l) => has in l);
    return this.lines.splice(i, 1)[0]!;
  }
  close() {
    this.sock.destroy();
  }
}

/** What the terminal shows, through its menu's "Copy what is shown". */
async function shown(page: Page): Promise<string> {
  await page.getByRole("button", { name: "Terminals" }).click();
  await page.getByRole("menuitem", { name: "Copy what is shown" }).click();
  return page.evaluate(() => navigator.clipboard.readText());
}

test("go-link-desk-terminal: linked in its own terminal, terminals with names in the browser, and back to them, like screen", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  test.setTimeout(120_000);
  const host = spawnTerminal("terminal");
  try {
    await expect.poll(() => /msg="pairing code"/.test(log(host)), { timeout: 30_000 }).toBe(true);
    // The password first: without it, link refuses.
    execFileSync(join(BIN, "terminal"), ["password", "--stdin", "--config", host.config], { input: `${PASSWORD}\n` });

    // "go-link-desk-terminal link": the code, then the question, answered here.
    const cmd = await Commands.open(host);
    cmd.send({ cmd: "link" });
    const code = String((await cmd.next("code")).code);
    expect(code).toMatch(/^\d{9}$/);
    // The code goes in /web's usual box: no session has it, so the page
    // tries it as a linking code by itself.
    await page.goto("/web");
    await page.locator("#ios-code").fill(code);
    await page.locator("#ios-code").press("Enter");
    await page.getByLabel("Your name").fill("Ana");
    await page.getByRole("button", { name: "Connect" }).click();
    const ask = (await cmd.next("ask")).ask as { name: string };
    expect(ask.name).toBe("Ana");
    cmd.send({ answer: true });
    expect(String((await cmd.next("linked")).linked)).toContain("Ana");
    cmd.close();

    // The shell, in the browser, in the session's own toolbar.
    await expect(page.getByRole("toolbar", { name: "Terminal of pi" })).toBeVisible();
    await expect(page.getByRole("tab", { name: /shell/ })).toHaveAttribute("aria-selected", "true");
    await page.locator(".terminal-box").click();
    await page.keyboard.type("echo hola-$((40+2)); stty size\n");
    await expect.poll(() => shown(page), { timeout: 15_000 }).toContain("hola-42");
    await expect.poll(() => /msg="terminal opened"/.test(log(host))).toBe(true);
    const devices = await page.evaluate(() => JSON.parse(localStorage.getItem("go-link-desk.devices") ?? "[]") as { deviceId: string; terminal?: boolean }[]);
    expect(devices).toHaveLength(1);
    expect(devices[0]!.terminal).toBe(true);

    // The text size, in the monitor's menu, as a screen's settings: it stays open.
    await page.getByRole("button", { name: "Screen", exact: true }).click();
    await expect(page.getByText("Text size: 14")).toBeVisible();
    await page.getByRole("menuitem", { name: /Bigger text/ }).click();
    await page.getByRole("menuitem", { name: /Bigger text/ }).click();
    await expect(page.getByText("Text size: 18")).toBeVisible();
    await page.screenshot({ path: "test-results/terminal-text-size.png" });
    await page.getByRole("menuitem", { name: "Normal size" }).click();
    await expect(page.getByText("Text size: 14")).toBeVisible();
    await page.keyboard.press("Escape");

    // A second terminal; exit closes only it.
    await page.getByRole("button", { name: "New terminal" }).click();
    await expect(page.getByRole("tab", { name: /shell 2/ })).toHaveAttribute("aria-selected", "true");
    await page.locator(".terminal-box").click();
    await page.keyboard.type("exit 3\n");
    await expect(page.getByRole("tab", { name: /shell 2/ })).toHaveCount(0);
    await expect(page.getByRole("toolbar", { name: "Terminal of pi" })).toBeVisible();

    // A name, by double click.
    await page.getByRole("tab", { name: /shell/ }).dblclick();
    await page.getByLabel("Terminal name").fill("build");
    await page.getByLabel("Terminal name").press("Enter");
    await expect(page.getByRole("tab", { name: /build/ })).toBeVisible();

    // Gone while a command runs: it goes on, like screen.
    await page.locator(".terminal-box").click();
    await page.keyboard.type("sleep 2; echo despues-$((1+1))\n");
    await page.getByRole("button", { name: "Disconnect" }).click();
    await expect.poll(() => /msg="terminals detached/.test(log(host))).toBe(true);
    expect(log(host)).not.toMatch(/msg="terminal closed"/);

    // Back from My computers: the link, then the password, and the same terminal.
    await page.goto("/web");
    await page.getByRole("link", { name: /^Connect to / }).first().click();
    await page.getByLabel("Password of that computer").fill(PASSWORD);
    await page.getByRole("button", { name: "Connect" }).click();
    await expect(page.getByRole("tab", { name: /build/ })).toBeVisible();
    await expect(page.getByText(/^Resumed: the terminal on pi was still open/)).toBeVisible();
    await expect.poll(() => shown(page), { timeout: 15_000 }).toContain("despues-2");
    await page.getByRole("button", { name: "Disconnect" }).click();
  } finally {
    await stopHost(host);
  }
});
