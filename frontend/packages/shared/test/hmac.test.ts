// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { hmacSha256, linkProof, linkProofBound, sameString, sdpFingerprint, sha256, utf8 } from "../src";
import vectors from "../../../../protocol/vectors/link-proof.json";

const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");

describe("sha256 and hmac", () => {
  it("matches the standard test vectors", () => {
    expect(hex(sha256(utf8("")))).toBe("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
    expect(hex(sha256(utf8("abc")))).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    // Two blocks.
    expect(hex(sha256(utf8("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")))).toBe(
      "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1",
    );
    // RFC 4231 case 2, and case 6 (a key longer than a block).
    expect(hex(hmacSha256(utf8("Jefe"), utf8("what do ya want for nothing?")))).toBe(
      "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
    );
    expect(hex(hmacSha256(new Uint8Array(131).fill(0xaa), utf8("Test Using Larger Than Block-Size Key - Hash Key First")))).toBe(
      "60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54",
    );
  });

  it("compares strings without an early exit", () => {
    expect(sameString("abc", "abc")).toBe(true);
    expect(sameString("abc", "abd")).toBe(false);
    expect(sameString("abc", "ab")).toBe(false);
  });

  it("proves a link exactly like the host does", () => {
    // The same vectors as backend-device/internal/services/link_proof_test.go.
    expect(vectors.cases.length).toBeGreaterThan(0);
    for (const c of vectors.cases) expect(linkProof(c.token, c.nonce)).toBe(c.proof);
  });

  it("binds the proof to the connection exactly like the host does", () => {
    expect(vectors.bound_cases.length).toBeGreaterThan(0);
    for (const c of vectors.bound_cases) expect(linkProofBound(c.token, c.nonce, c.host_fingerprint, c.viewer_fingerprint)).toBe(c.proof2);
    for (const c of vectors.fingerprint_cases) expect(sdpFingerprint(c.sdp)).toBe(c.fingerprint);
  });
});
