// node tools/test-socket.mjs: the app's side of the socket, on a stand-in for the SDK's companion.
import assert from 'node:assert';
import { createSocket } from '../engine/socket.js';

let listen = null, ready = false, gone = 0;
const sent = [], got = [];
const t = createSocket({ on: (fn) => { listen = fn; }, send: (m) => { if (ready) sent.push(m); return ready; } }, (m) => got.push(m), () => gone++);
t.send({ t: 'a' }); t.send({ t: 'b' });
listen({ companion: 'started', code: '' });
assert.equal(got.length, 0);
ready = true;
await new Promise((r) => setTimeout(r, 120));
assert.deepEqual(sent.map((m) => m.t), ['a', 'b']);       // waited, in order
listen({ t: 'state', set: { version: '1' } });
assert.equal(got[0].t, 'state');
listen({ companion: 'refused', code: 'sig.fileModified' });
assert.equal(gone, 1);
t.send({ t: 'c' });
assert.equal(sent.length, 2);
console.log('socket ok');
