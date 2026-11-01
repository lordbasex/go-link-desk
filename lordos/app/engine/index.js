// The only door to the engine: every view reads engine state and sends engine messages through here,
// nothing else in the app knows what is behind it. ENGINE.md is the protocol. Stage 3 replaces
// createTransport with the socket on fd 3; until then the fake engine answers.
import { useEffect, useState } from 'react';
import { createFake } from './fake.js';

function createTransport(onMessage) {
  // stage 3: lord.broker.companion() → frames of JSON; each frame goes to onMessage
  return createFake(onMessage);
}
let state = {};
const watchers = new Set();
const pending = new Map();
let seq = 0;
function onMessage(m) {
  if (m.t === 'state') { state = { ...state, ...m.set }; for (const w of watchers) w(state); }
  else if (m.t === 'res') { const p = pending.get(m.id); if (p) { pending.delete(m.id); if (m.err) p.reject(new Error(m.err)); else p.resolve(m.data); } }
}
const transport = createTransport(onMessage);
export const engine = {
  get: () => state,
  subscribe(fn) { watchers.add(fn); return () => watchers.delete(fn); },
  send(t, data = {}) { transport.send({ t, ...data }); },
  ask(t, data = {}) { const id = ++seq; return new Promise((resolve, reject) => { pending.set(id, { resolve, reject }); transport.send({ t, id, ...data }); }); },
  scene(name) { if (transport.scene) transport.scene(name); },   // the fake's states, to drive every view
};
export function useEngine() {
  const [s, set] = useState(state);
  useEffect(() => { set(engine.get()); return engine.subscribe(set); }, []);
  return s;
}
