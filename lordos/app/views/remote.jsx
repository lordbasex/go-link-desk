// A session to another computer: internal/gui/remote.go and remote_panels.go, and the toolbar, overlay
// and details as pkg/remoteview/remoteview_darwin.m draws them (go-link's Windows build opens the
// browser here, so the Mac's is the reference).
import { View, Text, ScrollView, Image, TextInput } from '@lordos/sdk';
import { useContext, useEffect, useState } from 'react';
import { engine } from '../engine/index.js';
import { L, Lf } from '../ui/i18n.js';
import { Icon } from '../ui/icon.jsx';
import { humanSize } from '../ui/format.js';
import { Btn, MenuCtx, Txt, usePal } from '../ui/widgets.jsx';
import { FButton, FLabel, FProgress, FWindow } from '../ui/fyne.jsx';
import { ChatInput, ChatLines } from './small.jsx';

export function statusText(r) {
  const T = {
    connecting: 'Connecting…', waiting: 'Waiting for the person at the computer to accept…', password: 'This computer asks for its unattended access password.',
    checking: 'Checking the password…', reconnecting: 'The connection dropped. Reconnecting…', timeout: 'Nobody answered at the computer.', busy: 'The computer is in another session.',
    unavailable: 'Nobody can accept at the computer (it has no window).', off: 'Unattended access is off at the computer.', declined: 'The person at the computer declined.',
    ended_host: 'The person at the computer ended the session.', ended_viewer: 'You ended the session.', ended_network: 'The connection was lost.', ended_signal: 'The computer went offline.',
    ended_idle: 'The session ended after a while without use.', ended_revoked: 'The computer removed this link.', ended: 'The session ended.',
    notfound: 'There is no computer with that code. Check it and try again.', ratelimited: 'Too many tries. Wait a minute and try again.', unreachable: 'Cannot reach the go-link server. Check the connection.',
    unlinked: 'That computer no longer has this one linked. Link it again with its pairing code.', offline: 'That computer is not on, or go-link Desk is closed there.',
    untrusted: 'Whoever answered could not prove it is your linked computer. Nothing was sent to it.', self: 'That is this computer.',
  };
  if (r.status === 'wrong') return Lf('Wrong password. %d tries left.', r.left ?? 2);
  return T[r.status] ? L(T[r.status]) : '';
}
const SPIN = new Set(['connecting', 'waiting', 'checking', 'reconnecting']);
const OVER = new Set(['timeout', 'busy', 'unavailable', 'off', 'declined', 'ended', 'ended_host', 'ended_viewer', 'ended_network', 'ended_signal', 'ended_idle', 'ended_revoked', 'notfound', 'ratelimited', 'unreachable', 'unlinked', 'offline', 'untrusted', 'self']);
function Spinner() {
  const p = usePal();
  const [i, set] = useState(0);
  useEffect(() => { const t = setInterval(() => set((v) => (v + 1) % 8), 100); return () => clearInterval(t); }, []);
  return (
    <View style={{ width: 32, height: 32 }}>
      {[...Array(8).keys()].map((k) => {
        const a = (k * Math.PI) / 4;
        return <View key={k} style={{ position: 'absolute', left: 14 + 11 * Math.sin(a), top: 14 - 11 * Math.cos(a), width: 4, height: 4, borderRadius: 2, backgroundColor: p.muted, opacity: ((k - i + 8) % 8) / 8 + 0.12 }} />;
      })}
    </View>
  );
}
function Overlay({ r }) {
  const p = usePal();
  const [pw, setPw] = useState('');
  const text = statusText(r);
  const asks = r.status === 'password' || r.status === 'wrong';
  const go = () => { engine.send('remote.password', { remote: r.id, password: pw }); setPw(''); };
  return (
    <View style={{ position: 'absolute', left: 0, top: 0, right: 0, bottom: 0, alignItems: 'center', justifyContent: 'center' }}>
      <View style={{ minWidth: 300, maxWidth: 440, backgroundColor: 'rgba(22,26,35,0.96)', borderRadius: 14, borderWidth: 1, borderColor: p.border, paddingVertical: 26, paddingHorizontal: 30, gap: 14, alignItems: 'center' }}>
        {SPIN.has(r.status) ? <Spinner /> : null}
        <Text style={{ fontSize: 15, color: p.text, textAlign: 'center', maxWidth: 360 }}>{text}</Text>
        {asks ? <View style={{ width: 260, height: 26, backgroundColor: p.bg, borderWidth: 1, borderColor: p.fieldBorder, borderRadius: 4, justifyContent: 'center' }}><TextInput secure value={pw} onChange={setPw} onSubmit={go} autoFocus size={14} style={{ backgroundColor: 'transparent', borderWidth: 0, height: 24 }} /></View> : null}
        {asks ? <Btn title={L('Unlock')} symbol="lock.open" primary onClick={go} /> : null}
      </View>
    </View>
  );
}
const hudText = (s) => ` ${s.codec} ${s.width}x${s.height} · ${s.fps} fps · ${s.kbps} kbps (${L('aim')} ${s.target} · ${L('path')} ${s.path}) `;
function toolMenu(id, r) {
  const tool = (t, value) => () => engine.send('remote.tool', { remote: r.id, tool: t, value });
  if (id === 'display') return r.displays.map((d, i) => ({ title: `${d.name || Lf('Display %d', i + 1)} (${d.width}×${d.height})`, checked: d.id === r.current, run: tool('display', d.id) }));
  if (id === 'quality') return [['speed', 'Speed'], ['balanced', 'Balanced'], ['quality', 'Quality']].map(([k, t]) => ({ title: L(t), checked: r.preset === k, run: tool('preset', k) })).concat(['-', { title: L("Retina (the display's own pixels)"), checked: r.retina, run: tool('retina') }]);
  if (id === 'keys') {
    const mac = r.host?.os === 'darwin' || !r.host?.os;
    const k = (title, keys) => ({ title, run: () => engine.send('remote.keys', { remote: r.id, keys }) });
    return [{ title: L('Type the clipboard'), run: () => engine.send('remote.typeClipboard', { remote: r.id }) }, '-', ...(mac ? [
      k(L('Switch apps (Cmd+Tab)'), 'MetaLeft+Tab'), k(L('Spotlight (Cmd+Space)'), 'MetaLeft+Space'), k(L('Force quit (Cmd+Option+Esc)'), 'MetaLeft+AltLeft+Escape'), k(L('Lock screen (Ctrl+Cmd+Q)'), 'ControlLeft+MetaLeft+KeyQ'), k(L('Screenshot (Cmd+Shift+3)'), 'MetaLeft+ShiftLeft+Digit3'),
    ] : [k('Ctrl+Alt+Del', 'ControlLeft+AltLeft+Delete'), k(L('Switch apps (Alt+Tab)'), 'AltLeft+Tab'), k(L('Start menu (Win)'), 'MetaLeft'), k(L('Lock screen (Win+L)'), 'MetaLeft+KeyL')])];
  }
  if (id === 'record') return [{ title: L('Take a screenshot') }, { title: L('Record the screen') }, '-', { title: L('Show downloads') }, { title: L('The person at the computer sees when you take a screenshot or record.'), disabled: true }];
  return [];
}
export function RemoteToolbar({ r, onChat, onFiles }) {
  const p = usePal();
  const menu = useContext(MenuCtx);
  const live = r.status === 'live', pm = r.perms || {}, term = r.host?.terminal;
  const tools = [
    ['display', 'Displays', 'display', live], ['quality', 'Picture', 'slider.horizontal.3', live && !term], ['keys', 'Keys', 'command', live && pm.control],
    ['chat', 'Chat', 'bubble.left.and.bubble.right', !term], ['files', 'Files', 'folder', live && pm.files], ['record', 'Screenshot and recording', 'record.circle', live && pm.record && !term],
    ['sound', 'Sound', 'speaker.wave.2', live && pm.audio && !term], ['info', 'Details', 'info.circle', true], ['fullscreen', 'Full screen', 'arrow.up.left.and.arrow.down.right', true],
  ];
  const act = (id) => (e) => {
    if (id === 'chat') return onChat();
    if (id === 'files') return onFiles();
    if (id === 'info' || id === 'sound') return engine.send('remote.tool', { remote: r.id, tool: id });
    if (id === 'fullscreen') return engine.send('remote.tool', { remote: r.id, tool: id });
    menu.open(e, toolMenu(id, r));
  };
  return (
    <View style={{ height: 44, backgroundColor: p.surface, flexDirection: 'row', gap: 6, alignItems: 'center', paddingLeft: 14, paddingRight: 10, borderBottomWidth: 1, borderColor: p.border }}>
      <Txt s={r.title} size={13} w={600} color={p.text} style={{ flexShrink: 1 }} />
      <View style={{ flex: 1 }} />
      {tools.map(([id, label, sym, on]) => <Btn key={id} symbol={sym} enabled={!!on} tint={(id === 'info' && r.hud) || (id === 'sound' && r.sound && on) ? p.accent : undefined} onClick={act(id)} />)}
      <Btn title={L('Disconnect')} symbol="xmark.circle.fill" fill={p.dangerFill} onClick={() => engine.send('remote.close', { remote: r.id })} />
    </View>
  );
}
function Terminal({ r }) {
  // terminal/page.html's colors. The real screen is the engine's output in the SDK's terminal surface: stage 3.
  const on = 1;
  return (
    <View style={{ flex: 1, backgroundColor: '#0d1117' }}>
      <View style={{ flex: 1, paddingTop: 8, paddingLeft: 10, paddingRight: 6, paddingBottom: 6 }}>
        <Text style={{ fontFamily: 'monospace', fontSize: 14, lineHeight: 18, color: '#e6edf3' }}>{'federico@studio ~ % ls\nDesktop  Documents  Downloads  notes.txt\nfederico@studio ~ % '}</Text>
      </View>
      <View style={{ flexDirection: 'row', gap: 2, paddingHorizontal: 6, backgroundColor: '#161b22', borderTopWidth: 1, borderColor: '#30363d' }}>
        {(r.tabs || []).map((t) => (
          <View key={t.id} style={{ flexDirection: 'row', gap: 6, alignItems: 'center', paddingVertical: 7, paddingLeft: 12, paddingRight: 10, backgroundColor: t.id === on ? '#0d1117' : 'transparent', borderTopWidth: 1, borderColor: t.id === on ? '#58a6ff' : 'transparent' }}>
            <View style={{ width: 6, height: 6, borderRadius: 3, backgroundColor: '#3fb950' }} />
            <Text style={{ fontFamily: 'monospace', fontSize: 12, color: t.id === on ? '#e6edf3' : '#8b949e' }}>{t.name}</Text>
          </View>
        ))}
        <Text style={{ fontFamily: 'monospace', fontSize: 15, color: '#8b949e', paddingVertical: 5, paddingHorizontal: 12 }}>+</Text>
      </View>
    </View>
  );
}
export function RemotePage({ r, picture, onChat, onFiles }) {
  const p = usePal();
  const showOverlay = r.status !== 'live';
  return (
    <View style={{ flex: 1 }}>
      <RemoteToolbar r={r} onChat={onChat} onFiles={onFiles} />
      <View style={{ flex: 1, backgroundColor: p.video, overflow: 'hidden' }}>
        {r.host?.terminal ? <Terminal r={r} /> : picture || null}
        {r.hud ? <View style={{ position: 'absolute', left: 10, top: 10, backgroundColor: 'rgba(0,0,0,0.65)' }}><Text style={{ fontFamily: 'monospace', fontSize: 11, color: p.text }}>{hudText(r.sent)}</Text></View> : null}
        {showOverlay ? <Overlay r={r} /> : null}
      </View>
    </View>
  );
}
export const RemoteOver = OVER;
export function RemoteChat({ f, r }) {
  const NOTES = { no_screen_permission: 'The computer may not record its screen: you see a test card.', no_accessibility: 'The computer has no Accessibility permission: your mouse and keyboard do not reach it.', capture_failed: 'The computer could not capture its screen.', wayland_session: "The computer's Wayland desktop cannot share its screen: you see a test card.", share_denied: 'The person at the computer did not allow sharing the screen: you see a test card.', no_remote_input: 'The person at the computer shared the screen but not the mouse and keyboard: you can only watch.' };
  const host = r.host?.name || L('The other computer');
  const lines = r.chat.map((c) => ({ who: c.fromHost ? host : L('You'), mine: !c.fromHost, at: c.at, text: c.text })).concat((r.notes || []).map((n) => ({ note: true, text: L(NOTES[n] || n) })));
  return (
    <FWindow f={f}>
      <ChatLines f={f} lines={lines} />
      <ChatInput f={f} typing={r.typing ? Lf('%s is typing…', host) : ''} onTyping={(on) => engine.send('remote.typing', { remote: r.id, on })} onSend={(text) => { engine.send('remote.chat', { remote: r.id, text }); return true; }} />
    </FWindow>
  );
}
// The other computer's files. Paths are relative to its home folder: the top of this view is "~",
// back stops there, and nothing above it can be asked for. What is sent lands in its ~/Downloads.
export const parentOf = (p) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '');
export function RemoteFiles({ f, r }) {
  const [path, setPath] = useState(r.files?.path || '');
  const [entries, setEntries] = useState(r.files?.entries || null);
  const [status, setStatus] = useState(L('Click a file to download it to your Downloads folder, or drop files on the screen to send them.'));
  const [bar, setBar] = useState(null);
  const open = (p) => {
    if (!r.perms?.files) return setStatus(L('The computer does not let you use its files in this session.'));
    setStatus(L('Loading…'));
    engine.ask('files.list', { remote: r.id, path: p }).then((d) => { setPath(d.path); setEntries(d.entries); setStatus(Lf('%d items', d.entries.length)); }, (e) => setStatus(Lf('Cannot open the folder: %s', e.message)));
  };
  useEffect(() => { if (!entries) open(''); else setStatus(Lf('%d items', entries.length)); }, []);
  const click = (e) => {
    const p = path ? `${path}/${e.name}` : e.name;
    if (e.dir) return open(p);
    setStatus(Lf('Downloading %s…', e.name)); setBar(0);
    engine.ask('files.download', { remote: r.id, path: p }).then((d) => { setBar(null); setStatus(Lf('Downloaded to %s', d.saved)); }, (x) => { setBar(null); setStatus(Lf('%s was not downloaded: %s', e.name, x.message)); });
  };
  const send = () => engine.ask('files.upload', { remote: r.id, name: 'notes.txt' }).then((d) => setStatus(Lf('Sent: it is in Downloads/%s on the other computer', d.saved)));
  return (
    <FWindow f={f}>
      <View style={{ flexDirection: 'row', gap: 4, alignItems: 'center' }}>
        <FButton f={f} icon="fyne-arrow-back" onClick={() => { if (path) open(parentOf(path)); }} />
        <FButton f={f} icon="fyne-home" onClick={() => open('')} />
        <View style={{ flex: 1 }}><FLabel f={f} mono nowrap>{`~/${path}`}</FLabel></View>
        <FButton f={f} label={L('Send files…')} icon="fyne-upload" onClick={send} />
        <FButton f={f} label={L('Show downloads')} icon="fyne-folder-open" onClick={() => engine.send('files.reveal')} />
      </View>
      <ScrollView style={{ flex: 1 }}>
        <View>{(entries || []).map((e) => (
          <View key={e.name} onClick={() => click(e)} style={{ height: 37, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 8, gap: 4 }}>
            <Icon name={e.dir ? 'fyne-folder' : 'fyne-file'} size={20} color={f.fg} />
            <View style={{ flex: 1 }}><FLabel f={f} nowrap>{e.name}</FLabel></View>
            {e.dir ? null : <FLabel f={f} nowrap>{humanSize(e.size)}</FLabel>}
          </View>
        ))}</View>
      </ScrollView>
      <FLabel f={f}>{status}</FLabel>
      {bar != null ? <View style={{ flexDirection: 'row', gap: 4 }}><FProgress f={f} value={bar} /><FButton f={f} label={L('Stop')} icon="fyne-cancel" onClick={() => setBar(null)} /></View> : null}
    </FWindow>
  );
}
