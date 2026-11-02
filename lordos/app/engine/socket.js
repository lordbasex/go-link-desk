// The engine's socket on the app's side (ENGINE.md, Transport): u32 length, little-endian, then that many bytes
// of UTF-8 JSON, 64 KB at most. `conn` is what the broker hands over: { write(Uint8Array), onData(fn), onClose(fn) }.
export const MAX_FRAME = 64 * 1024;

export function encodeFrame(msg) {
  const body = new TextEncoder().encode(JSON.stringify(msg));
  if (body.length > MAX_FRAME) throw new Error('frame too long');
  const out = new Uint8Array(4 + body.length);
  new DataView(out.buffer).setUint32(0, body.length, true);
  out.set(body, 4);
  return out;
}

// Bytes in, messages out, across any cut. A frame too long or not JSON is the end of the connection.
export function frameReader(onMessage, onBad) {
  let buf = new Uint8Array(0);
  const dec = new TextDecoder();
  return (chunk) => {
    const next = new Uint8Array(buf.length + chunk.length);
    next.set(buf);
    next.set(chunk, buf.length);
    buf = next;
    while (buf.length >= 4) {
      const n = new DataView(buf.buffer, buf.byteOffset).getUint32(0, true);
      if (n > MAX_FRAME) { onBad(); return; }
      if (buf.length < 4 + n) return;
      let m;
      try { m = JSON.parse(dec.decode(buf.subarray(4, 4 + n))); } catch { onBad(); return; }
      buf = buf.subarray(4 + n);
      onMessage(m);
    }
  };
}

export function createSocket(conn, deliver, onGone) {
  let open = true;
  const gone = () => { if (open) { open = false; onGone(); } };
  conn.onData(frameReader(deliver, gone));
  conn.onClose(gone);
  return { send(m) { if (open) conn.write(encodeFrame(m)); } };
}
