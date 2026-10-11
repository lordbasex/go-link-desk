// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/**
 * Puts what was copied on the host on this computer's clipboard. Chromium
 * writes it while the page has the focus; Safari and Firefox only during a
 * click or a key press, so the text waits for the next one (or for the
 * Copy button the page shows meanwhile).
 */
export class HostClipboard {
  private pending: string | null = null;

  constructor(private readonly onPending: (waiting: boolean) => void) {}

  /** A text copied on the host. */
  offer(text: string): void {
    this.pending = text;
    void this.write();
  }

  /** Called from a click or key press: the moment Safari and Firefox let a page write the clipboard. */
  gesture(): void {
    if (this.pending !== null) void this.write();
  }

  /** Drops what is waiting. */
  clear(): void {
    this.pending = null;
    this.onPending(false);
  }

  private async write(): Promise<void> {
    const text = this.pending;
    if (text === null) return;
    try {
      await navigator.clipboard.writeText(text);
      if (this.pending === text) this.clear();
    } catch {
      this.onPending(true);
    }
  }
}
