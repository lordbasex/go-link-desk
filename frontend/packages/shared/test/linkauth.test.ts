// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { describe, expect, it } from "vitest";
import { hostProofMessage, hostUnlinkMessage, importPublic, linkProofMessage, newLinkKey, sign, verify } from "../src";
import vectors from "../../../../protocol/vectors/link-proof.json";

describe("link auth with ECDSA keys", () => {
  // The same vectors as backend-device (link_proof_test.go) and the mobile
  // viewers. ECDSA is randomized, so each side verifies the signatures here
  // against the given public keys rather than reproducing the bytes.
  it("verifies the host and link proofs in the shared vector", async () => {
    expect(await verify(vectors.host_pub, hostProofMessage(vectors.nonce, vectors.host_fingerprint, vectors.viewer_fingerprint), vectors.host_proof_sig)).toBe(true);
    expect(await verify(vectors.link_pub, linkProofMessage(vectors.nonce, vectors.host_fingerprint, vectors.viewer_fingerprint), vectors.link_proof_sig)).toBe(true);
    expect(await verify(vectors.host_pub, hostUnlinkMessage(vectors.nonce, vectors.host_fingerprint, vectors.viewer_fingerprint), vectors.host_unlink_sig)).toBe(true);
  });

  it("rejects a proof for the wrong nonce, fingerprints, key or message", async () => {
    const { host_pub, link_pub, nonce, host_fingerprint: h, viewer_fingerprint: v, host_proof_sig } = vectors;
    expect(await verify(host_pub, hostProofMessage("other", h, v), host_proof_sig)).toBe(false);
    expect(await verify(host_pub, hostProofMessage(nonce, v, h), host_proof_sig)).toBe(false);
    expect(await verify(host_pub, linkProofMessage(nonce, h, v), host_proof_sig)).toBe(false);
    expect(await verify(link_pub, hostProofMessage(nonce, h, v), host_proof_sig)).toBe(false);
  });

  it("makes a non-extractable key that signs what it can verify", async () => {
    const key = await newLinkKey();
    expect(key.privateKey.extractable).toBe(false);
    const msg = linkProofMessage("n", "sha-256 AA", "sha-256 BB");
    const sig = await sign(key.privateKey, msg);
    expect(await verify(key.publicKey, msg, sig)).toBe(true);
    expect(await verify(key.publicKey, linkProofMessage("n2", "sha-256 AA", "sha-256 BB"), sig)).toBe(false);
  });

  it("never throws on malformed input", async () => {
    expect(await importPublic("not base64url !!!")).toBeNull();
    expect(await importPublic("AAAA")).toBeNull();
    expect(await verify("AAAA", new TextEncoder().encode("x"), "AAAA")).toBe(false);
    expect(await verify(vectors.host_pub, new TextEncoder().encode("x"), "nope !!!")).toBe(false);
  });
});
