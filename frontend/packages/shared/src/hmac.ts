// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// SHA-256 and HMAC-SHA256 in plain TypeScript. The browser's own crypto
// (crypto.subtle) only exists on https pages, and a device's local web
// panel is served over plain http on the LAN, so the proofs that keep
// secrets off the wire are computed here instead.

const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

/** SHA-256 of bytes. */
export function sha256(data: Uint8Array): Uint8Array {
  return new Sha256().update(data).digest();
}

/**
 * SHA-256 fed piece by piece, for data too big to hold at once (a
 * recording downloaded in chunks).
 */
export class Sha256 {
  private h = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]);
  private w = new Uint32Array(64);
  private block = new Uint8Array(64);
  private view = new DataView(this.block.buffer);
  private used = 0; // bytes waiting in block
  private length = 0; // bytes hashed so far

  update(data: Uint8Array): this {
    this.length += data.length;
    let i = 0;
    if (this.used > 0) {
      const n = Math.min(64 - this.used, data.length);
      this.block.set(data.subarray(0, n), this.used);
      this.used += n;
      i = n;
      if (this.used < 64) return this;
      this.compress(this.view, 0);
      this.used = 0;
    }
    const whole = new DataView(data.buffer, data.byteOffset, data.byteLength);
    for (; i + 64 <= data.length; i += 64) this.compress(whole, i);
    if (i < data.length) {
      this.block.set(data.subarray(i));
      this.used = data.length - i;
    }
    return this;
  }

  /** The hash of everything given; the object is spent afterwards. */
  digest(): Uint8Array {
    // Padding: 0x80, zeros, then the length in bits as 64 bits big endian.
    const bitLen = this.length * 8;
    const tail = new Uint8Array(this.used + 9 <= 64 ? 64 - this.used : 128 - this.used);
    tail[0] = 0x80;
    const tv = new DataView(tail.buffer);
    tv.setUint32(tail.length - 8, Math.floor(bitLen / 2 ** 32));
    tv.setUint32(tail.length - 4, bitLen >>> 0);
    this.length -= tail.length; // the padding is not part of the message
    this.update(tail);
    const out = new Uint8Array(32);
    const ov = new DataView(out.buffer);
    for (let i = 0; i < 8; i++) ov.setUint32(i * 4, this.h[i]!);
    return out;
  }

  private compress(view: DataView, off: number): void {
    const { h, w } = this;
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4);
    for (let i = 16; i < 64; i++) {
      const a = w[i - 15]!;
      const b = w[i - 2]!;
      const s0 = ((a >>> 7) | (a << 25)) ^ ((a >>> 18) | (a << 14)) ^ (a >>> 3);
      const s1 = ((b >>> 17) | (b << 15)) ^ ((b >>> 19) | (b << 13)) ^ (b >>> 10);
      w[i] = (w[i - 16]! + s0 + w[i - 7]! + s1) >>> 0;
    }
    let [a, b, c, d, e, f, g, hh] = [h[0]!, h[1]!, h[2]!, h[3]!, h[4]!, h[5]!, h[6]!, h[7]!];
    for (let i = 0; i < 64; i++) {
      const S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
      const ch = (e & f) ^ (~e & g);
      const t1 = (hh + S1 + ch + K[i]! + w[i]!) >>> 0;
      const S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const t2 = (S0 + maj) >>> 0;
      hh = g;
      g = f;
      f = e;
      e = (d + t1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (t1 + t2) >>> 0;
    }
    h[0] = (h[0]! + a) >>> 0;
    h[1] = (h[1]! + b) >>> 0;
    h[2] = (h[2]! + c) >>> 0;
    h[3] = (h[3]! + d) >>> 0;
    h[4] = (h[4]! + e) >>> 0;
    h[5] = (h[5]! + f) >>> 0;
    h[6] = (h[6]! + g) >>> 0;
    h[7] = (h[7]! + hh) >>> 0;
  }
}

/** HMAC-SHA256 (RFC 2104). */
export function hmacSha256(key: Uint8Array, message: Uint8Array): Uint8Array {
  const block = 64;
  const k = key.length > block ? sha256(key) : key;
  const ipad = new Uint8Array(block + message.length);
  const opad = new Uint8Array(block + 32);
  for (let i = 0; i < block; i++) {
    const b = k[i] ?? 0;
    ipad[i] = b ^ 0x36;
    opad[i] = b ^ 0x5c;
  }
  ipad.set(message, block);
  opad.set(sha256(ipad), block);
  return sha256(opad);
}

export const utf8 = (s: string): Uint8Array => new TextEncoder().encode(s);

/** Bytes as unpadded base64url. */
export function base64url(bytes: Uint8Array): string {
  let bin = "";
  bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** Random bytes as unpadded base64url (a nonce). */
export function randomNonce(bytes = 32): string {
  const b = new Uint8Array(bytes);
  crypto.getRandomValues(b);
  return base64url(b);
}

/** Compares two strings without stopping at the first difference. */
export function sameString(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

/** An SDP's first sha-256 certificate fingerprint as "sha-256 AB:CD:…", or "" without one. */
export function sdpFingerprint(sdp: string): string {
  for (const line of sdp.split("\n")) {
    const v = line.trim();
    if (!v.startsWith("a=fingerprint:")) continue;
    const [alg, hex] = v.slice("a=fingerprint:".length).trim().split(/\s+/, 2);
    if (alg?.toLowerCase() === "sha-256" && hex) return `sha-256 ${hex.toUpperCase()}`;
  }
  return "";
}
