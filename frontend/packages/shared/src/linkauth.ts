// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Asymmetric link authentication, the browser's half. Each link has an
// ECDSA P-256 key the browser makes and keeps non-extractable (its private
// half never leaves the browser, not even to script); the host stores only
// the public half. The browser also pins the host's device public key and
// checks every proof the host signs with it.
//
// WebCrypto needs a secure context (crypto.subtle), which the unattended
// (reach) web app always has; the local http panel never links, so it never
// calls this. A public key is the 65-byte uncompressed point, a signature is
// the 64-byte P1363 pair (R||S) WebCrypto produces, both base64url.

import { base64url } from "./hmac";

const ALG = { name: "ECDSA", namedCurve: "P-256" } as const;
const SIGN = { name: "ECDSA", hash: "SHA-256" } as const;

const enc = (s: string): Uint8Array => new TextEncoder().encode(s);

/** The message the host signs to prove itself on a reach. */
export function hostProofMessage(nonce: string, hostFingerprint: string, viewerFingerprint: string): Uint8Array {
  return enc(`go-link-desk host proof v1\n${nonce}\n${hostFingerprint}\n${viewerFingerprint}`);
}

/** The message the browser signs to prove its link on a reach. */
export function linkProofMessage(nonce: string, hostFingerprint: string, viewerFingerprint: string): Uint8Array {
  return enc(`go-link-desk link proof v1\n${nonce}\n${hostFingerprint}\n${viewerFingerprint}`);
}

/** The message the host signs to tell a browser its link is gone. */
export function hostUnlinkMessage(nonce: string, hostFingerprint: string, viewerFingerprint: string): Uint8Array {
  return enc(`go-link-desk host unlink v1\n${nonce}\n${hostFingerprint}\n${viewerFingerprint}`);
}

/** A link's key: the non-extractable private half, and the public half to send. */
export interface LinkKey {
  privateKey: CryptoKey;
  publicKey: string;
}

/** Makes a fresh link key. The private half cannot be exported. */
export async function newLinkKey(): Promise<LinkKey> {
  const pair = await crypto.subtle.generateKey(ALG, false, ["sign"]);
  const raw = new Uint8Array(await crypto.subtle.exportKey("raw", pair.publicKey));
  return { privateKey: pair.privateKey, publicKey: base64url(raw) };
}

/** Signs a message with a link's private key; returns the 64-byte R||S, base64url. */
export async function sign(key: CryptoKey, message: Uint8Array): Promise<string> {
  const sig = new Uint8Array(await crypto.subtle.sign(SIGN, key, message as BufferSource));
  return base64url(sig);
}

function fromBase64url(s: string): Uint8Array | null {
  try {
    const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  } catch {
    return null;
  }
}

/** Imports a public key from its 65-byte uncompressed point (base64url). */
export async function importPublic(pub: string): Promise<CryptoKey | null> {
  const raw = fromBase64url(pub);
  if (!raw || raw.length !== 65 || raw[0] !== 4) return null;
  try {
    return await crypto.subtle.importKey("raw", raw as BufferSource, ALG, false, ["verify"]);
  } catch {
    return null;
  }
}

/**
 * Verifies that sig (64-byte R||S, base64url) is pub's signature over the
 * message. Never throws: malformed input is just a false.
 */
export async function verify(pub: string, message: Uint8Array, sig: string): Promise<boolean> {
  const key = await importPublic(pub);
  const raw = fromBase64url(sig);
  if (!key || !raw || raw.length !== 64) return false;
  try {
    return await crypto.subtle.verify(SIGN, key, raw as BufferSource, message as BufferSource);
  } catch {
    return false;
  }
}
