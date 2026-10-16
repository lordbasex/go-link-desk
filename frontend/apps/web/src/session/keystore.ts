// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// A link's private key is a non-extractable WebCrypto key: it can sign but
// never be read, not even by script on this page, so a cross-site-scripting
// hole or a rogue extension cannot steal a link the way it could a token in
// localStorage. Such a key cannot be serialized, so it lives in IndexedDB
// (which stores the key handle, not its bytes), keyed by the link id.

/** Keeps a link's non-extractable signing key. */
export interface KeyStore {
  save(linkId: string, key: CryptoKey): Promise<void>;
  get(linkId: string): Promise<CryptoKey | null>;
  remove(linkId: string): Promise<void>;
}

const DB = "go-link-desk";
const STORE = "link-keys";

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains(STORE)) req.result.createObjectStore(STORE);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function tx<T>(mode: IDBTransactionMode, run: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return open().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const req = run(db.transaction(STORE, mode).objectStore(STORE));
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
      }),
  );
}

/** The real store, in IndexedDB. */
export const indexedDbKeyStore: KeyStore = {
  async save(linkId, key) {
    await tx("readwrite", (s) => s.put(key, linkId));
  },
  async get(linkId) {
    try {
      return (await tx<CryptoKey | undefined>("readonly", (s) => s.get(linkId))) ?? null;
    } catch {
      return null;
    }
  },
  async remove(linkId) {
    try {
      await tx("readwrite", (s) => s.delete(linkId));
    } catch {
      // nothing to remove, or no store
    }
  },
};
