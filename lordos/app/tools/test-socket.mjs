// node tools/test-socket.mjs [engine binary]: the socket's framing, and with a binary built with
// `-tags headless,lordos`, the app's side against the real engine on fd 3.
import assert from 'node:assert';
import net from 'node:net';
import os from 'node:os';
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { MAX_FRAME, encodeFrame, frameReader, createSocket } from '../engine/socket.js';

const got = []; let bad = 0;
const read = frameReader((m) => got.push(m), () => bad++);
const f = encodeFrame({ t: 'x', text: 'ñandú ✓' });
for (const b of f) read(Uint8Array.of(b));      // a byte at a time
read(new Uint8Array([...f, ...f]));             // two at once
assert.equal(got.length, 3); assert.equal(got[0].text, 'ñandú ✓'); assert.equal(bad, 0);
const long = new Uint8Array(4); new DataView(long.buffer).setUint32(0, MAX_FRAME + 1, true);
read(long); assert.equal(bad, 1);
assert.throws(() => encodeFrame({ t: 'x'.repeat(MAX_FRAME) }));
console.log('framing ok');

const exe = process.argv[2];
if (!exe) process.exit(0);
const home = fs.mkdtempSync(path.join(os.tmpdir(), 'engine-'));
const dir = path.join(home, '.local/share/lordos/appdata/go-link-desk'); fs.mkdirSync(dir, { recursive: true });
const sock = path.join(home, 's');
const server = net.createServer(); await new Promise((r) => server.listen(sock, r));
const peer = new Promise((r) => server.once('connection', r));
const mine = net.connect(sock); await new Promise((r) => mine.once('connect', r));
const theirs = await peer;
const child = spawn(exe, ['--config', path.join(dir, 'device.json')], { stdio: ['ignore', 'ignore', 'ignore', theirs], env: { HOME: home, PATH: process.env.PATH } });
theirs.destroy();
const state = {}; const asks = new Map(); let goneCalled = false; let seq = 0;
const conn = { write: (b) => mine.write(b), onData: (fn) => mine.on('data', fn), onClose: (fn) => mine.on('close', fn) };
const t = createSocket(conn, (m) => {
  if (m.t === 'state') Object.assign(state, m.set);
  else if (m.t === 'res') { asks.get(m.id)?.(m); asks.delete(m.id); }
}, () => { goneCalled = true; });
const ask = (kind, d) => new Promise((r) => { const id = ++seq; asks.set(id, r); t.send({ t: kind, id, ...d }); });
const until = async (fn) => { for (let i = 0; i < 300 && !fn(); i++) await new Promise((r) => setTimeout(r, 20)); assert.ok(fn()); };
await until(() => state.version);
assert.equal(state.config.unattended.enabled, false);
assert.equal((await ask('settings.set', { key: 'unattended', value: true })).err, 'password_first');
assert.equal((await ask('files.list', { remote: 'nope', path: '/etc' })).err, 'no_session');
const left = new Promise((r) => child.once('exit', r));
t.send({ t: 'quit' });
await left;
await until(() => goneCalled);
fs.rmSync(home, { recursive: true, force: true });
server.close();
console.log('real engine ok'); process.exit(0);
