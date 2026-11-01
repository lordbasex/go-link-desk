// A stand-in for go-link's engine, speaking ENGINE.md's messages: the code, a knock, a session, chat,
// files, recent sessions and sessions to other computers, in realistic states. Replaced in stage 3.
const NOW = () => Date.now();
const HOME_TREE = {
  '': [{ name: 'Desktop', dir: true }, { name: 'Documents', dir: true }, { name: 'Downloads', dir: true }, { name: 'notes.txt', size: 1843 }, { name: 'report.pdf', size: 2345000 }],
  Documents: [{ name: 'invoices', dir: true }, { name: 'budget.ods', size: 48211 }, { name: 'contract.pdf', size: 612000 }],
  'Documents/invoices': [{ name: 'october.pdf', size: 88120 }],
  Downloads: [{ name: 'driver.zip', size: 18400000 }],
  Desktop: [],
};
const FULL = { view: true, control: true, audio: true, clipboard: true, files: true, record: false };

// A small picture of a desktop, as a viewer's thumbnail would show (the real engine sends a JPEG's path).
function thumb(seed) {
  const w = 200, h = 150, data = new Uint8ClampedArray(w * h * 4);
  const hue = [[46, 92, 158], [120, 72, 140], [36, 124, 110], [150, 96, 52]][seed % 4];
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const o = (y * w + x) * 4, k = 0.55 + 0.45 * (1 - y / h);
    let r = hue[0] * k, g = hue[1] * k, b = hue[2] * k;
    const inWin = x > 28 + seed * 7 && x < 150 && y > 18 && y < 96;
    if (inWin) { r = g = b = y < 26 ? 62 : 228; }
    if (y > 138) { r = 20; g = 22; b = 28; }
    data[o] = r; data[o + 1] = g; data[o + 2] = b; data[o + 3] = 255;
  }
  return { width: w, height: h, data };
}
const dt = (d, h, m) => new Date(2026, 9, d, h, m).getTime();

function initial() {
  return {
    version: '1.8.0', hostname: 'lordos-office', localIP: '192.168.1.20', webURL: 'https://desk.go-link.org', signalURL: 'wss://signal.go-link.org/ws',
    signal: 'connecting', session: null, active: null, pairing: { code: '' },
    perms: [{ id: 'screen', granted: true }, { id: 'accessibility', granted: true }],
    config: { theme: '', app_icon: '', tray_icon: '', signal_url: '', web_url: '', udp_port: 0, map_udp_port: false, announce_ips: [], language: '', no_update_check: false, autostart: false,
      idle_minutes: 0, lock_on_end: false, request_ticks: '', viewer_name: '', session_windows: false, unattended: { enabled: false, has_password: false }, assistants: { enabled: false, act: false } },
    links: [], devices: [], recents: [],
    assistants: { clients: [], saved: {}, errs: {} }, ports: null,
    consent: null, callAsk: null, call: null, chat: [], typing: false, transfer: null,
    remotes: [], update: { state: 'idle' }, netTest: null, notes: {},
  };
}
export function createFake(deliver) {
  const s = initial();
  const emit = (set) => { Object.assign(s, set); deliver({ t: 'state', set }); };
  const later = (ms, fn) => setTimeout(fn, ms);
  const res = (m, data, err) => { if (m.id) deliver({ t: 'res', id: m.id, data, err }); };
  const code = () => String(100000000 + Math.floor(Math.random() * 899999999));
  const remote = (id) => s.remotes.find((r) => r.id === id);
  const setRemote = (id, patch) => emit({ remotes: s.remotes.map((r) => (r.id === id ? { ...r, ...patch } : r)) });
  const addRecent = (r) => emit({ recents: [r, ...s.recents.filter((x) => x.code !== r.code)].slice(0, 12) });

  function openRemote(door) {
    const id = `r${NOW()}`;
    emit({ remotes: [...s.remotes, { id, door, title: door.title || door.code, status: 'connecting', detail: '', host: null, perms: null, hud: false, sound: true, preset: 'balanced', retina: false,
      displays: [{ id: 1, name: 'Built-in display', width: 2560, height: 1440 }], current: 1, chat: [], files: { path: '', entries: null, status: '' }, sent: { codec: 'H264', width: 1920, height: 1080, fps: 30, kbps: 3120, target: 4000, path: 18200 } }] });
    later(500, () => {
      if (door.code === '000000000') return setRemote(id, { status: 'notfound' });
      if (door.kind !== 'reach') setRemote(id, { status: 'waiting' });
      else setRemote(id, { status: 'password' });
    });
    if (door.kind !== 'reach') later(1800, () => goLive(id));
    return id;
  }
  function goLive(id) {
    const r = remote(id);
    if (!r) return;
    const host = { name: r.door.title && r.door.kind === 'reach' ? r.door.title : 'Studio Mac', os: 'darwin', terminal: r.door.terminal };
    setRemote(id, { status: 'live', title: host.name, host, perms: FULL, still: thumb(1) });
    if (r.door.kind === 'join') addRecent({ code: r.door.code, name: host.name, os: host.os, at: NOW(), thumb: thumb(3) });
  }
  const scenes = {
    home() { emit({ signal: 'connected', session: { code: '456257529', invite: 'uOxMUHDgzYn10kl0Qu_45Q' } }); },
    devices() {
      scenes.home();
      emit({ devices: [
        { id: 'a', name: 'Home Mac', os: 'darwin', last_used: dt(11, 13, 7), favorite: true },
        { id: 'b', name: 'Workstation', os: 'windows', last_used: dt(9, 21, 7), favorite: false },
      ], recents: [{ code: '464786468', name: 'Studio Mac', os: 'darwin', at: dt(11, 18, 7) }] });
    },
    thumbs() {
      scenes.devices();
      emit({ devices: s.devices.map((d, i) => ({ ...d, thumb: thumb(i) })), recents: s.recents.map((d) => ({ ...d, thumb: thumb(2) })) });
    },
    empty() { scenes.home(); emit({ devices: [], recents: [] }); },
    offline() { scenes.home(); emit({ signal: 'disconnected' }); },
    connecting() { emit({ signal: 'connecting', session: null }); },
    knock() { scenes.devices(); emit({ consent: { id: 'k1', name: 'Ana Support', browser: 'Chrome 142 on Windows', link: false, deadline: NOW() + 47500, warning: true } }); },
    knockLink() { scenes.devices(); emit({ consent: { id: 'k2', name: 'Ana Support', browser: 'Chrome 142 on Windows', link: true, deadline: NOW() + 47500, warning: true } }); },
    session() {
      scenes.devices();
      const since = new Date(2026, 10, 1, 15, 7).getTime();
      emit({ active: { peer: 'p', name: 'Ana Support', browser: 'Chrome 142 on Windows', since, permissions: FULL },
        chat: [{ fromViewer: true, text: 'Hello, I can see your screen now.', at: since }, { fromViewer: false, text: 'Great, please check the printer.', at: since + 60000 }],
        transfer: { upload: true, name: 'driver.zip', size: 100, done: 42 } });
    },
    recording() { scenes.session(); emit({ active: { ...s.active, recordingSince: s.active.since } }); },
    callAsk() { scenes.session(); emit({ callAsk: { name: 'Ana Support', video: true } }); },
    call() { scenes.session(); emit({ call: { name: 'Ana Support', hostCamera: false, hostMic: true, viewerCamera: false, viewerMic: true } }); },
    remote(terminal) {
      scenes.devices();
      const r = { id: 'r1', door: { kind: 'join', code: '464786468', terminal }, title: 'Studio Mac', status: 'live', detail: '', host: { name: 'Studio Mac', os: 'darwin', terminal }, perms: FULL, hud: false, sound: true, preset: 'balanced', retina: false,
        displays: [{ id: 1, name: 'Built-in display', width: 2560, height: 1440 }, { id: 2, name: 'Studio Display', width: 3840, height: 2160 }], current: 1,
        chat: [{ fromHost: true, text: 'Hi, tell me when you are in.', at: new Date(2026, 10, 1, 15, 7).getTime() }, { text: 'I am in.', at: new Date(2026, 10, 1, 15, 8).getTime() }],
        notes: ['no_screen_permission'], typing: false, still: thumb(0), 
        files: { path: '', entries: HOME_TREE[''], status: 'items' }, sent: { codec: 'H264', width: 1920, height: 1080, fps: 30, kbps: 3120, target: 4000, path: 18200 },
        tabs: [{ id: 1, name: 'zsh' }, { id: 2, name: 'build' }] };
      emit({ remotes: [r] });
    },
    remoteStatus(status) { scenes.remote(); setRemote('r1', { status }); },
    remoteHud() { scenes.remote(); setRemote('r1', { hud: true }); },
    unattended() {
      scenes.devices();
      emit({ config: { ...s.config, unattended: { enabled: true, has_password: true }, assistants: { enabled: true, act: false } }, pairing: { code: '812 440 276' },
        links: [{ id: 'l1', name: "Federico's iPhone", created: dt(11, 13, 7), last_seen: dt(30, 9, 41), permissions: { control: true, audio: true, clipboard: false, files: false, record: false } }],
        assistants: { clients: [{ name: 'Claude', registered: true }, { name: 'Claude Code', registered: true }, { name: 'Cursor', registered: false }], saved: { a: true }, errs: {} } });
    },
    updateCurrent() { scenes.home(); emit({ update: { state: 'current' } }); },
    updateFound() { scenes.home(); emit({ update: { state: 'available', latest: '1.9.0', url: 'https://github.com/lordbasex/go-link-desk/releases' } }); },
  };
  later(600, () => { if (!s.session) scenes.home(); });
  deliver({ t: 'state', set: s });

  const handlers = {
    copy() {},
    'code.new'() { emit({ session: { ...s.session, code: code() } }); },
    'session.end'() { emit({ active: null, chat: [], transfer: null }); },
    'session.permissions'(m) { emit({ active: { ...s.active, permissions: m.permissions } }); },
    'consent.answer'(m) {
      const c = s.consent;
      emit({ consent: null });
      if (m.accept) emit({ active: { peer: 'p', name: c.name, browser: c.browser, since: NOW(), permissions: m.permissions } });
    },
    'chat.send'(m) { emit({ chat: [...s.chat, { fromViewer: false, text: m.text, at: NOW() }] }); },
    'chat.typing'() {},
    'call.answer'(m) { const a = s.callAsk; emit({ callAsk: null }); if (m.accept) emit({ call: { name: a.name, hostCamera: m.camera, hostMic: m.mic, viewerCamera: true, viewerMic: true } }); },
    'call.set'(m) { emit({ call: { ...s.call, hostCamera: m.camera ?? s.call.hostCamera, hostMic: m.mic ?? s.call.hostMic } }); },
    'call.end'() { emit({ call: null }); },
    'settings.set'(m) { setting(m.key, m.value); },
    'link.set'(m) { emit({ links: s.links.map((l) => (l.id === m.link ? { ...l, permissions: { ...l.permissions, [m.what]: m.on } } : l)) }); },
    'link.revoke'(m) { emit({ links: s.links.filter((l) => l.id !== m.link) }); },
    'perm.allow'(m) { emit({ perms: s.perms.map((p) => (p.id === m.perm ? { ...p, granted: true } : p)) }); },
    connect(m) { openRemote({ kind: m.kind, code: m.code }); },
    demo() { openRemote({ kind: 'join', code: '700100200', title: 'go-link Desk demo' }); },
    'device.connect'(m) { const d = s.devices.find((x) => x.id === m.device); if (d) openRemote({ kind: 'reach', device: d.id, title: d.label || d.name }); },
    'device.favorite'(m) { emit({ devices: s.devices.map((d) => (d.id === m.device ? { ...d, favorite: !d.favorite } : d)) }); },
    'device.rename'(m) { emit({ devices: s.devices.map((d) => (d.id === m.device ? { ...d, label: m.label.trim().slice(0, 64) } : d)) }); },
    'device.forget'(m) { emit({ devices: s.devices.filter((d) => d.id !== m.device) }); },
    'recent.connect'(m) { openRemote({ kind: 'join', code: m.code }); },
    'recent.remove'(m) { emit({ recents: s.recents.filter((r) => r.code !== m.code) }); },
    'recent.clear'() { emit({ recents: [] }); },
    'remote.close'(m) { emit({ remotes: s.remotes.filter((r) => r.id !== m.remote) }); },
    'remote.password'(m) { setRemote(m.remote, { status: 'checking' }); later(500, () => goLive(m.remote)); },
    'remote.tool'(m) {
      const r = remote(m.remote);
      if (!r) return;
      if (m.tool === 'info') setRemote(r.id, { hud: !r.hud });
      if (m.tool === 'sound') setRemote(r.id, { sound: !r.sound });
      if (m.tool === 'preset') setRemote(r.id, { preset: m.value });
      if (m.tool === 'retina') setRemote(r.id, { retina: !r.retina });
      if (m.tool === 'display') setRemote(r.id, { current: m.value });
    },
    'remote.chat'(m) { const r = remote(m.remote); setRemote(m.remote, { chat: [...r.chat, { text: m.text, at: NOW() }] }); },
    // Files: paths are relative to the other computer's home folder, and nothing above it can be asked for.
    'files.list'(m) {
      const p = String(m.path || '');
      if (p.startsWith('/') || p.split('/').includes('..') || p.startsWith('~')) return res(m, null, 'outside_home');
      const entries = HOME_TREE[p];
      if (!entries) return res(m, null, 'not_found');
      res(m, { path: p, entries: [...entries].sort((a, b) => (a.dir !== b.dir ? (a.dir ? -1 : 1) : a.name.localeCompare(b.name))) });
    },
    'files.download'(m) { res(m, { saved: `Downloads/${m.path.split('/').pop()}` }); },
    // What is sent to the computer always lands in its ~/Downloads: the sender has no say in the folder.
    'files.upload'(m) { res(m, { saved: m.name.split('/').pop() }); },
    'update.check'() { emit({ update: { state: 'checking' } }); later(900, () => emit({ update: { state: 'current' } })); },
    'network.test'() { emit({ netTest: 'running' }); later(800, () => emit({ netTest: null })); },
    quit() {},
  };
  function setting(key, value) {
    const c = { ...s.config };
    if (key === 'password') c.unattended = { ...c.unattended, has_password: true };
    else if (key === 'password_clear') c.unattended = { enabled: false, has_password: false };
    else if (key === 'unattended') c.unattended = { ...c.unattended, enabled: value };
    else if (key === 'assistants') c.assistants = { ...c.assistants, enabled: value };
    else if (key === 'assistants_act') c.assistants = { ...c.assistants, act: value };
    else c[key] = value;
    emit({ config: c });
  }
  return {
    send(m) { const h = handlers[m.t]; if (h) h(m); else res(m, null, 'unknown'); },
    scene(name) {
      const r = Object.assign(initial(), {});
      for (const k of Object.keys(s)) if (!(k in r)) delete s[k];
      emit(r);
      const [n, arg] = String(name).split(':');
      if (scenes[n]) scenes[n](arg === 'terminal' ? true : arg);
    },
  };
}
