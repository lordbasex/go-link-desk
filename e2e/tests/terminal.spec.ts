// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { connect, type Socket } from "node:net";
import { dirname, join } from "node:path";
import { expect, test } from "@playwright/test";
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

test("go-link-desk-terminal: linked in its own terminal, then a shell in the browser, and back with the password", async ({ page }) => {
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
    await page.goto(`/link/${code}`);
    await page.getByLabel("Your name").fill("Ana");
    await page.getByRole("button", { name: "Connect" }).click();
    const ask = (await cmd.next("ask")).ask as { name: string };
    expect(ask.name).toBe("Ana");
    cmd.send({ answer: true });
    expect(String((await cmd.next("linked")).linked)).toContain("Ana");
    cmd.close();

    // The shell, in the browser: what is typed runs there.
    await expect(page.getByRole("toolbar", { name: "Terminal of pi" })).toBeVisible();
    const screen = page.locator(".terminal-box .xterm-rows");
    await page.locator(".terminal-box").click();
    await page.keyboard.type("echo hola-$((40+2)); stty size\n");
    await expect(screen).toContainText("hola-42", { timeout: 15_000 });
    await expect.poll(() => /msg="terminal opened"/.test(log(host))).toBe(true);
    const devices = await page.evaluate(() => JSON.parse(localStorage.getItem("go-link-desk.devices") ?? "[]") as { deviceId: string; terminal?: boolean }[]);
    expect(devices).toHaveLength(1);
    expect(devices[0]!.terminal).toBe(true);

    // exit ends the session, and says how.
    await page.keyboard.type("exit 3\n");
    await expect(page.getByText("The shell ended (exit code 3).")).toBeVisible();

    // Back from My computers: the link, then the password.
    await page.goto("/web");
    await page.getByRole("link", { name: /^Connect to / }).first().click();
    await page.getByLabel("Password of that computer").fill(PASSWORD);
    await page.getByRole("button", { name: "Connect" }).click();
    await expect(page.getByRole("toolbar", { name: "Terminal of pi" })).toBeVisible();
    await page.locator(".terminal-box").click();
    await page.keyboard.type("echo again-$((1+1))\n");
    await expect(screen).toContainText("again-2", { timeout: 15_000 });
    await page.getByRole("button", { name: "Disconnect" }).click();
    await expect.poll(() => /msg="terminal closed"/.test(log(host))).toBe(true);
  } finally {
    await stopHost(host);
  }
});
