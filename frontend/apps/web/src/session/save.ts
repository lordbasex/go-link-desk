// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { memorySink, type DownloadSink } from "@go-link-desk/shared";

/** A place to save to; inMemory: the browser keeps it all until the end. */
export type SaveSink = DownloadSink & { inMemory: boolean };

type Picker = (o: unknown) => Promise<{ createWritable(): Promise<{ write(c: Uint8Array): Promise<void>; close(): Promise<void>; abort(): Promise<void> }> }>;

/**
 * Where a download goes: a file the person picks (Chromium; call it from
 * the click), else memory, saved at the end. Null: the person cancelled.
 */
export async function sinkFor(name: string): Promise<SaveSink | null> {
  const picker = (window as unknown as { showSaveFilePicker?: Picker }).showSaveFilePicker;
  if (picker) {
    try {
      const handle = await picker({ suggestedName: name });
      const w = await handle.createWritable();
      return { write: (c) => w.write(c), close: () => w.close(), abort: () => w.abort(), inMemory: false };
    } catch (e) {
      if ((e as Error).name === "AbortError") return null; // the person cancelled
    }
  }
  return { ...memorySink((blob) => downloadBlob(blob, name)), inMemory: true };
}

/** Saves a Blob the browser's way (the Downloads folder, or it asks). */
export function downloadBlob(blob: Blob, name: string): void {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 60_000);
}
