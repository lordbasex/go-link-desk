// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { expect, test } from "@playwright/test";
import { SIGNAL_URL } from "../ports";
import { hostLogged, sessionCode } from "../stack";

type Envelope = { type: string; [k: string]: unknown };

/**
 * A plain signalhub client (Node's WebSocket sends no Origin, like a
 * native app): collects every message it gets.
 */
async function connect(): Promise<{ ws: WebSocket; next: (type: string) => Promise<Envelope> }> {
  const ws = new WebSocket(`${SIGNAL_URL}?v=1`);
  const inbox: Envelope[] = [];
  const waiters: (() => void)[] = [];
  ws.addEventListener("message", (e) => {
    inbox.push(JSON.parse(String(e.data)) as Envelope);
    waiters.splice(0).forEach((w) => w());
  });
  await new Promise<void>((ok, fail) => {
    ws.addEventListener("open", () => ok(), { once: true });
    ws.addEventListener("error", () => fail(new Error("cannot reach signalhub")), { once: true });
  });
  const next = async (type: string): Promise<Envelope> => {
    const end = Date.now() + 10_000;
    for (;;) {
      const i = inbox.findIndex((m) => m.type === type || m.type === "error");
      if (i >= 0) return inbox.splice(i, 1)[0]!;
      if (Date.now() > end) throw new Error(`no ${type} from signalhub`);
      await new Promise<void>((r) => {
        waiters.push(r);
        setTimeout(r, 250);
      });
    }
  };
  return { ws, next };
}

test("the hosts register and show a 9-digit session code", () => {
  expect(sessionCode("accept")).toMatch(/^\d{9}$/);
  expect(sessionCode("reject")).toMatch(/^\d{9}$/);
  expect(hostLogged(/msg="session room open"/)).toBe(true);
});

test("the code opens the host's invite-only room on signalhub", async () => {
  const code = sessionCode();
  const { ws, next } = await connect();
  try {
    await next("hello");
    ws.send(JSON.stringify({ type: "join", app: "go-link-desk", code }));
    const joined = await next("joined");
    expect(joined.type, JSON.stringify(joined)).toBe("joined");
    expect(joined.room_id).toBeTruthy();
    await expect.poll(() => hostLogged(/msg="viewer joined the session room"/)).toBe(true);
  } finally {
    ws.close();
  }
});

test("a wrong code opens nothing", async () => {
  const code = sessionCode();
  const wrong = code === "000000000" ? "111111111" : "000000000";
  const { ws, next } = await connect();
  try {
    await next("hello");
    ws.send(JSON.stringify({ type: "join", app: "go-link-desk", code: wrong }));
    expect((await next("joined")).type).toBe("error");
  } finally {
    ws.close();
  }
});

test("the website takes the code and opens its session page", async ({ page }) => {
  const code = sessionCode();
  await page.goto("/");
  await page.getByRole("link", { name: "Connect" }).click();
  await expect(page).toHaveURL(/\/c$/);
  await page.getByLabel("Code or link").fill(code);
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page).toHaveURL(new RegExp(`/c/${code}$`));
  const grouped = `${code.slice(0, 3)} ${code.slice(3, 6)} ${code.slice(6)}`;
  await expect(page.getByRole("heading", { name: `Connect to ${grouped}` })).toBeVisible();
});
