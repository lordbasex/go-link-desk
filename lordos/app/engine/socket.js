// The engine's socket on the app's side (ENGINE.md, Transport): one JSON value per message. `conn` is the SDK's
// `companion`: on(fn), send(value). { companion: 'started' | 'refused' | 'ended' } are the system's, never the engine's.
export function createSocket(conn, deliver, onGone) {
  let open = true;
  const queue = [];
  const gone = () => { if (open) { open = false; queue.length = 0; onGone(); } };
  const drain = () => {
    while (open && queue.length && conn.send(queue[0])) queue.shift();
    if (open && queue.length) setTimeout(drain, 50);      // not running yet, or it can't take it now
  };
  conn.on((m) => {
    if (m && m.companion) { if (m.companion !== 'started') gone(); return; }
    deliver(m);
  });
  return { send(m) { if (open) { queue.push(m); if (queue.length === 1) drain(); } } };
}
