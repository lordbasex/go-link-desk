// node tools/test-fake.mjs: the Files rule on the fake engine (ENGINE.md, Files) and the view's parent path.
import assert from 'node:assert';
import { createFake } from '../engine/fake.js';
const out = []; const f = createFake((m) => out.push(m));
const ask = (t, d) => { const id = out.length + 1000; f.send({ t, id, ...d }); return out.filter((m) => m.t === 'res' && m.id === id).pop(); };
for (const p of ['/', '/etc', '..', '../..', 'Documents/../..', '~/x', '~']) assert.equal(ask('files.list', { path: p }).err, 'outside_home', p);
assert.ok(ask('files.list', { path: '' }).data.entries.length);
assert.equal(ask('files.upload', { name: '../../evil.txt' }).data.saved, 'evil.txt');
const parentOf = (p) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '');
assert.equal(parentOf('Documents/invoices'), 'Documents'); assert.equal(parentOf('Documents'), ''); assert.equal(parentOf(''), '');
console.log('files rule ok'); process.exit(0);
