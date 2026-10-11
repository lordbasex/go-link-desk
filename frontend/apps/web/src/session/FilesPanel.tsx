// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useCallback, useEffect, useRef, useState } from "react";
import { FilesError, memorySink, type DownloadSink, type FileEntry, type FilesClient } from "@go-link-desk/shared";
import { t } from "../i18n";

/** A file moving, as the panel lists it. */
interface Job {
  key: number;
  up: boolean;
  name: string;
  size: number;
  done: number;
  state: "queued" | "moving" | "done" | "failed";
  note?: string; // the saved name, or why it failed
  abort?: AbortController;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
}

/** Where a download goes: a file the person picks (Chromium), else memory, saved at the end. */
async function sinkFor(name: string): Promise<DownloadSink | null> {
  const picker = (window as unknown as { showSaveFilePicker?: (o: unknown) => Promise<{ createWritable(): Promise<{ write(c: Uint8Array): Promise<void>; close(): Promise<void>; abort(): Promise<void> }> }> }).showSaveFilePicker;
  if (picker) {
    try {
      const handle = await picker({ suggestedName: name });
      const w = await handle.createWritable();
      return { write: (c) => w.write(c), close: () => w.close(), abort: () => w.abort() };
    } catch (e) {
      if ((e as Error).name === "AbortError") return null; // the person cancelled
    }
  }
  return memorySink((blob) => {
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 60_000);
  });
}

let jobKeys = 0;

/** The session's files: send to the host, and browse and download the host's home folder. */
export function FilesPanel({ files, dropped, onClose }: { files: FilesClient; dropped: File[]; onClose: () => void }) {
  const [tab, setTab] = useState<"send" | "host">("send");
  const [jobs, setJobs] = useState<Job[]>([]);
  const [path, setPath] = useState("");
  const [entries, setEntries] = useState<FileEntry[] | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const queue = useRef<{ file: File; key: number }[]>([]);
  const busy = useRef(false);
  const pickRef = useRef<HTMLInputElement>(null);

  const update = (key: number, patch: Partial<Job>) => setJobs((js) => js.map((j) => (j.key === key ? { ...j, ...patch } : j)));

  const pump = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    while (queue.current.length) {
      const { file, key } = queue.current.shift()!;
      const abort = new AbortController();
      update(key, { state: "moving", abort });
      let last = 0;
      try {
        const saved = await files.upload(
          file,
          (n) => {
            const now = performance.now();
            if (now - last > 150 || n === file.size) {
              last = now;
              update(key, { done: n });
            }
          },
          abort.signal,
        );
        update(key, { state: "done", done: file.size, note: saved });
      } catch (e) {
        update(key, { state: "failed", note: e instanceof FilesError && e.code === "aborted" ? t.files.stopped : (e as Error).message });
      }
    }
    busy.current = false;
  }, [files]);

  const send = useCallback(
    (list: File[]) => {
      if (!list.length) return;
      setTab("send");
      const added = list.map((file) => ({ file, key: ++jobKeys }));
      setJobs((js) => [...js, ...added.map(({ file, key }): Job => ({ key, up: true, name: file.name, size: file.size, done: 0, state: "queued" }))]);
      queue.current.push(...added);
      void pump();
    },
    [pump],
  );

  // Files dropped on the screen arrive here.
  useEffect(() => send(dropped), [dropped, send]);

  const open = useCallback(
    async (p: string) => {
      setListError(null);
      try {
        setEntries(await files.list(p));
        setPath(p);
      } catch (e) {
        setListError((e as Error).message);
      }
    },
    [files],
  );
  useEffect(() => {
    if (tab === "host" && entries === null) void open("");
  }, [tab, entries, open]);

  const download = async (e: FileEntry) => {
    const full = path ? `${path}/${e.name}` : e.name;
    const sink = await sinkFor(e.name); // first, while the click still counts
    if (!sink) return;
    const key = ++jobKeys;
    const abort = new AbortController();
    setJobs((js) => [...js, { key, up: false, name: e.name, size: e.size, done: 0, state: "moving", abort }]);
    try {
      const d = await files.openDownload(full);
      let last = 0;
      await files.download(
        d,
        sink,
        (n) => {
          const now = performance.now();
          if (now - last > 150 || n === d.size) {
            last = now;
            update(key, { done: n });
          }
        },
        abort.signal,
      );
      update(key, { state: "done", done: d.size });
    } catch (err) {
      await sink.abort().catch(() => undefined);
      update(key, { state: "failed", note: err instanceof FilesError && err.code === "aborted" ? t.files.stopped : (err as Error).message });
    }
  };

  const crumbs = path ? path.split("/") : [];

  return (
    <aside className="files-panel" role="dialog" aria-label={t.files.title}>
      <header className="chat-head">
        <strong>{t.files.title}</strong>
        <button type="button" className="button button-secondary button-small" onClick={onClose}>
          {t.session.close}
        </button>
      </header>
      <div className="files-tabs" role="tablist">
        <button type="button" role="tab" aria-selected={tab === "send"} className={tab === "send" ? "is-on" : ""} onClick={() => setTab("send")}>
          {t.files.sendTab}
        </button>
        <button type="button" role="tab" aria-selected={tab === "host"} className={tab === "host" ? "is-on" : ""} onClick={() => setTab("host")}>
          {t.files.hostTab}
        </button>
      </div>
      {tab === "send" ? (
        <div
          className="files-drop"
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault();
            send([...e.dataTransfer.files]);
          }}
        >
          <p>{t.files.dropHint}</p>
          <button type="button" className="button button-primary button-small" onClick={() => pickRef.current?.click()}>
            {t.files.choose}
          </button>
          <input
            ref={pickRef}
            type="file"
            multiple
            hidden
            aria-label={t.files.choose}
            onChange={(e) => {
              send([...(e.target.files ?? [])]);
              e.target.value = "";
            }}
          />
          <p className="small muted">{t.files.whereHint}</p>
        </div>
      ) : (
        <div className="files-browser">
          <nav className="files-crumbs" aria-label={t.files.hostTab}>
            <button type="button" onClick={() => void open("")}>
              {t.files.home}
            </button>
            {crumbs.map((c, i) => (
              <span key={i}>
                {" › "}
                <button type="button" onClick={() => void open(crumbs.slice(0, i + 1).join("/"))}>
                  {c}
                </button>
              </span>
            ))}
          </nav>
          {listError && <p className="files-error">{listError}</p>}
          <ul className="files-list">
            {entries?.length === 0 && <li className="muted small">{t.files.empty}</li>}
            {entries?.map((e) => (
              <li key={e.name}>
                {e.dir ? (
                  <button type="button" className="files-dir" onClick={() => void open(path ? `${path}/${e.name}` : e.name)}>
                    📁 {e.name}
                  </button>
                ) : (
                  <>
                    <span className="files-name" title={e.name}>
                      {e.name}
                    </span>
                    <span className="files-size mono">{formatBytes(e.size)}</span>
                    <button type="button" className="button button-secondary button-small" onClick={() => void download(e)} aria-label={`${t.files.download} ${e.name}`}>
                      {t.files.download}
                    </button>
                  </>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      {jobs.length > 0 && (
        <ol className="files-jobs" aria-label={t.files.transfers}>
          {jobs.map((j) => (
            <li key={j.key} className={`files-job is-${j.state}`}>
              <span className="files-name" title={j.name}>
                {j.up ? "↑" : "↓"} {j.state === "done" && j.note ? j.note : j.name}
              </span>
              <progress max={Math.max(j.size, 1)} value={j.done} aria-label={j.name} />
              <span className="files-size mono">
                {j.state === "failed" ? j.note : j.state === "done" ? t.files.done : `${formatBytes(j.done)} / ${formatBytes(j.size)}`}
              </span>
              {(j.state === "moving" || j.state === "queued") && j.abort && (
                <button type="button" className="button button-secondary button-small" onClick={() => j.abort?.abort()}>
                  {t.files.stop}
                </button>
              )}
            </li>
          ))}
        </ol>
      )}
    </aside>
  );
}
