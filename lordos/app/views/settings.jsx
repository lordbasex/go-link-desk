// Settings: internal/gui/prefs.go builds the pages, pkg/fyneview/prefs.go draws them.
import { View, Text, ScrollView } from '@lordos/sdk';
import { useContext, useState } from 'react';
import { engine } from '../engine/index.js';
import { L, Lf, LANGUAGES } from '../ui/i18n.js';
import { Icon } from '../ui/icon.jsx';
import { Field, MenuCtx, Switch, Txt, usePal } from '../ui/widgets.jsx';
import { ymd, hhmm } from '../ui/format.js';
const DEFAULT_SIGNAL = 'wss://signal.go-link.org/ws', DEFAULT_WEB = 'https://desk.go-link.org';
const OFF = false;
const IDLE = [0, 5, 15, 30, 60];
const idleLabel = (m) => (m === 0 ? L('Never') : m % 60 === 0 ? Lf('After %d h', m / 60) : Lf('After %d min', m));
const set = (key) => (value) => engine.send('settings.set', { key, value });

export function prefsSpec(s, local = {}) {
  const cfg = s.config;
  const server = s.signal === 'connected' ? [L('Connected'), 'ok'] : s.signal === 'disconnected' ? [L('Offline, reconnecting…'), 'warn'] : [L('Connecting…'), 'warn'];
  const langs = LANGUAGES.map(([id, title]) => ({ id, title: id ? title : L('Automatic') }));
  const color = [{ id: '', title: L('Color') }, { id: 'dark', title: L('Dark') }];
  const pages = [];
  const serverRows = [
    { id: 'signal_url', kind: 'text', label: L('Signaling server'), value: cfg.signal_url, placeholder: DEFAULT_SIGNAL, detail: L("Your own signalhub (github.com/lordbasex/signalhub). Empty: go-link's.") },
    { id: 'web_url', kind: 'text', label: L('Website for viewers'), value: cfg.web_url, placeholder: DEFAULT_WEB, detail: L('The one shown with your code. It must use the same server.') },
  ];
  if (cfg.signal_url || cfg.web_url) serverRows.push({ id: 'server_default', kind: 'button', label: L("Go back to go-link's server"), button: L("Use go-link's") });
  const direct = [
    { kind: 'info', label: L("This computer's address"), value: s.localIP },
    { id: 'udp_port', kind: 'text', label: L('UDP port'), value: cfg.udp_port ? String(cfg.udp_port) : '', placeholder: L('Random'), detail: L('Every connection on one port, to open it in a router.') },
    { id: 'map_udp_port', kind: 'toggle', label: L('Open it in the router by itself'), on: cfg.map_udp_port, detail: L('With UPnP or NAT-PMP, while go-link Desk is open. Off: forward it in the router by hand.') },
    { id: 'announce_ips', kind: 'text', label: L('Announced address'), value: cfg.announce_ips.join(', '), placeholder: Lf('For example: %s', '192.168.1.20'), detail: L('Where viewers reach that port through the router: its address on their network. Several, with commas.') },
  ];
  pages.push({ id: 'general', title: L('General'), symbol: 'gearshape', sections: [
    { title: L('State'), rows: [
      { kind: 'info', label: L('Version'), value: `v${s.version}` }, { kind: 'info', label: L('This computer'), value: s.hostname },
      { kind: 'info', label: L('go-link server'), value: server[0], tint: server[1] }] },
    { title: L('Appearance'), rows: [
      { id: 'theme', kind: 'select', label: L('Theme'), value: cfg.theme, options: [{ id: '', title: L('Dark') }, { id: 'light', title: L('Light') }, { id: 'system', title: L('Like the system') }] },
      { id: 'app_icon', kind: 'select', label: L('App icon'), detail: L('The one in the dock or taskbar while go-link Desk is open.'), value: cfg.app_icon, options: color },
      { id: 'tray_icon', kind: 'select', label: L('System tray icon'), detail: L('In the panel, where the desktop has a system tray. It turns red while someone is connected.'), value: cfg.tray_icon, options: color }] },
    { title: L('Server'), footer: L('This computer, the computers it connects to and their browsers must all use the same server.'), rows: serverRows },
    { title: L('Direct connection (advanced)'), footer: L('For a computer behind a second router: without it, viewers on the other network go through a relay, which adds delay.'), rows: direct },
    { title: L('Interface'), footer: L('The new language shows the next time go-link Desk opens.'), rows: [{ id: 'language', kind: 'select', label: L('Language'), value: cfg.language, options: langs }] },
    { title: L('Start'), rows: [{ id: 'autostart', kind: 'toggle', label: L('Open go-link Desk when I log in'), on: cfg.autostart }] },
  ] });
  pages.push({ id: 'sessions', title: L('Sessions'), symbol: 'display.2', sections: [
    { title: L('When someone connects to this computer'), rows: [
      { id: 'idle_minutes', kind: 'select', label: L('Disconnect when idle'), value: String(cfg.idle_minutes), options: IDLE.map((m) => ({ id: String(m), title: idleLabel(m) })), num: true },
      { id: 'lock_on_end', kind: 'toggle', label: L('Lock the screen when a session ends'), on: cfg.lock_on_end },
      { id: 'request_ticks', kind: 'select', label: L('A request starts with'), detail: L('Only the screen: whoever asks has to be given each permission by hand.'), value: cfg.request_ticks, options: [{ id: '', title: L('Full control') }, { id: 'view', title: L('Only the screen') }] }] },
    { title: L('Connecting to other computers'), rows: [
      { id: 'viewer_name', kind: 'text', label: L('Your name'), detail: L("Shown to the computer you connect to. Empty: your user's name."), value: cfg.viewer_name, placeholder: 'Federico' },
      { id: 'session_windows', kind: 'toggle', label: L('Open sessions in tabs'), detail: L('Off: each session in a window of its own.'), on: !cfg.session_windows, invert: true }] },
  ] });
  const has = cfg.unattended.has_password, un = cfg.unattended.enabled && has;
  const toggle = { id: 'unattended', kind: 'toggle', label: L('Allow unattended access'), on: un };
  if (!has) { toggle.enabled = OFF; toggle.detail = L('Set a password below first.'); }
  const pass = [has ? { kind: 'info', label: L('Password'), value: L('Set'), tint: 'ok', detail: L('Linked devices type it after their link; the iPhone and iPad app keeps it behind Face ID.') }
    : { kind: 'info', label: L('Password'), value: L('None'), tint: 'warn', detail: L('Unattended access needs one.') },
  { id: 'password', kind: 'password', label: L('New password'), placeholder: Lf('%d characters or more', 8), repeat: L('The same again'), button: L('Set password') }];
  if (has) pass.push({ id: 'password_clear', kind: 'button', label: L('Remove the password'), button: L('Remove password'), tint: 'danger', confirm: { title: L('Remove the password?'), text: L('Unattended access turns off: it always needs a password.'), ok: L('Remove password'), cancel: L('Cancel') } });
  const secs = [{ rows: [toggle, { kind: 'note', label: L('Linked devices come in without you accepting, with the permissions you gave them and the password. Link only your own devices.'), tint: 'warn' }] },
    { title: L('Password'), rows: pass }];
  if (un) {
    secs.push({ title: L('Linking code'), rows: [{ kind: 'code', label: L('Linking code'), value: s.pairing.code || '— — —', detail: Lf('To link a device: open %s in its browser, or in the iPhone and iPad app go to My computers › Link, and type this code. You will be asked to accept it.', `${s.webURL}/link`) }] });
    if (!s.links.length) secs.push({ title: L('Linked devices'), rows: [{ kind: 'note', label: L('No device is linked.') }] });
    s.links.forEach((l, i) => {
      const perm = (what, label) => ({ id: `link:${l.id}:${what}`, kind: 'toggle', label, on: !!l.permissions[what], link: l.id, what });
      const name = l.name || L('A device');
      secs.push({ title: i === 0 ? L('Linked devices') : '', rows: [
        { kind: 'info', label: name, detail: Lf('Linked %s · last used %s', ymd(l.created), `${ymd(l.last_seen)} ${hhmm(l.last_seen)}`) },
        perm('control', L('Use your mouse and keyboard')), perm('audio', L("Hear your computer's sound")), perm('clipboard', L('Share the clipboard (copy and paste)')), perm('files', L('Send and receive files')), perm('record', L('Take screenshots and record the screen')),
        { id: `revoke:${l.id}`, kind: 'button', label: L('Revoke this device'), button: L('Revoke'), tint: 'danger', revoke: l.id, confirm: { title: L('Revoke this device?'), text: Lf('%s will not be able to come in again until it is linked with a new code.', name), ok: L('Revoke'), cancel: L('Cancel') } }] });
    });
  }
  pages.push({ id: 'unattended', title: L('Unattended access'), symbol: 'lock.shield', sections: secs });
  const a = cfg.assistants;
  const rows = [{ id: 'assistants', kind: 'toggle', label: L('Let AI assistants use your computers'), on: a.enabled, detail: L('Adds go-link Desk to Claude, Claude Code and Cursor on this computer (as an MCP server). Open them again after turning it on.') }];
  if (a.enabled) for (const c of s.assistants.clients) rows.push({ kind: 'info', label: c.name, value: c.err ? L('Could not add it') : c.registered ? L('Added') : L('Not installed'), tint: c.err ? 'danger' : c.registered ? 'ok' : '', detail: c.err });
  const page = { id: 'assistants', title: L('Assistants'), symbol: 'sparkles', sections: [{ rows }] };
  if (a.enabled) {
    page.sections.push({ title: L('What they may do'), rows: [{ id: 'assistants_act', kind: 'toggle', label: L('Let them type and run commands'), on: a.act, detail: L('Off: they only look: your computers, screenshots, terminals, the clipboard, and files they copy here.') }] });
    const devs = s.devices.map((d) => (s.assistants.saved[d.id]
      ? { id: `assistant_forget:${d.id}`, kind: 'button', label: d.label || d.name, detail: L("Allowed: its password is kept in this computer's keychain."), button: L('Stop allowing'), tint: 'danger' }
      : { id: `assistant_pass:${d.id}`, kind: 'password', label: d.label || d.name, placeholder: L('Its password'), button: L('Allow') }));
    if (!devs.length) devs.push({ kind: 'note', label: L('No computer is linked yet: link one from My computers first.') });
    page.sections.push({ title: L('Computers they may use'), rows: devs, footer: L("Each computer's unattended password is kept in this computer's keychain: assistants never see it. They come in by name, with that link's permissions, and the computer shows it.") });
    page.sections.push({ rows: [{ id: 'assistants_log', kind: 'button', label: L('Everything they do is written to an activity log.'), button: L('Show the log') }] });
  }
  pages.push(page);
  const permRows = [['screen', L('Screen Recording: to show your screen')], ['accessibility', L('Accessibility: to let the viewer use the mouse and keyboard')]].map(([id, label]) => (s.perms.find((x) => x.id === id)?.granted
    ? { kind: 'info', label, value: L('Allowed'), tint: 'ok' } : { id: `perm:${id}`, kind: 'button', label, button: L('Allow…') }));
  pages.push({ id: 'permissions', title: L('Permissions'), symbol: 'hand.raised', sections: [{ title: L("This computer's permissions"), footer: L('After allowing it in System Settings, quit go-link Desk and open it again.'), rows: permRows }] });
  return pages;
}
function send(r, value) {
  if (r.link) return engine.send('link.set', { link: r.link, what: r.what, on: value });
  if (r.revoke) return engine.send('link.revoke', { link: r.revoke });
  if (r.id?.startsWith('perm:')) return engine.send('perm.allow', { perm: r.id.slice(5) });
  if (r.num) return set(r.id)(Number(value));
  if (r.invert) return set(r.id)(!value);
  return set(r.id)(value);
}
const SmallBtn = ({ title, danger, onClick }) => {
  const p = usePal();
  return <View onClick={onClick} style={{ height: 30, paddingHorizontal: 11, borderRadius: 7, justifyContent: 'center', backgroundColor: danger ? p.dangerFill : p.surface2 }}><Txt s={title} size={12} color={danger ? p.danger : p.text} /></View>;
};
function tintOf(p, t) { return t === 'warn' ? p.accent : t === 'ok' ? p.ok : t === 'danger' ? p.danger : p.muted; }
function Select({ r, enabled }) {
  const p = usePal();
  const menu = useContext(MenuCtx);
  const cur = r.options.find((o) => o.id === r.value) || r.options[0];
  const w = Math.max(...r.options.map((o) => o.title.length)) * 7 + 40;
  return (
    <View onClick={enabled ? (e) => menu.open(e, r.options.map((o) => ({ title: o.title, checked: o.id === r.value, run: () => send(r, o.id) }))) : undefined}
      style={{ minWidth: w, height: 22, borderRadius: 5, backgroundColor: p.surface2, flexDirection: 'row', alignItems: 'center', paddingLeft: 9, paddingRight: 6, gap: 6 }}>
      <Txt s={cur?.title || ''} size={12} color={enabled ? p.text : p.faint} style={{ flex: 1 }} />
      <Icon name="chevrons-up-down" size={11} color={p.text} />
    </View>
  );
}
function TextRow({ r }) {
  const [v, setV] = useState(r.value || '');
  return <Field value={v} onChange={setV} placeholder={r.placeholder} onSubmit={() => send(r, v)} />;
}
function PassRow({ r }) {
  const [a, setA] = useState(''), [b, setB] = useState('');
  const go = () => { send(r, r.repeat ? `${a}\n${b}` : a); setA(''); setB(''); };
  return (
    <View style={{ flexDirection: 'row', gap: 6, alignItems: 'center' }}>
      <View style={{ gap: 4 }}><Field width={170} secure value={a} onChange={setA} placeholder={r.placeholder} onSubmit={go} />{r.repeat ? <Field width={170} secure value={b} onChange={setB} placeholder={r.repeat} onSubmit={go} /> : null}</View>
      <SmallBtn title={r.button} onClick={go} />
    </View>
  );
}
function Row({ r, onConfirm }) {
  const p = usePal();
  const enabled = r.enabled !== false;
  if (r.kind === 'note') return <View style={{ paddingVertical: 10, paddingHorizontal: 14 }}><Txt s={r.label} size={12} color={tintOf(p, r.tint)} nowrap={false} /></View>;
  let right = null;
  if (r.kind === 'toggle') right = <Switch on={r.on} enabled={enabled} onChange={(v) => send(r, v)} />;
  else if (r.kind === 'select') right = <Select r={r} enabled={enabled} />;
  else if (r.kind === 'text') right = <TextRow key={r.value} r={r} />;
  else if (r.kind === 'password') right = <PassRow r={r} />;
  else if (r.kind === 'button') right = <SmallBtn title={r.button} danger={r.tint === 'danger'} onClick={() => (r.confirm ? onConfirm(r) : send(r, ''))} />;
  else if (r.kind === 'code') right = <Txt s={r.value} size={17} w={600} color={p.accent} />;
  else {
    const v = <Txt s={r.value} size={13} color={tintOf(p, r.tint)} />;
    right = r.tint === 'warn' || r.tint === 'danger' ? <View style={{ flexDirection: 'row', gap: 5, alignItems: 'center' }}><Icon name="shield-alert" size={13} color={tintOf(p, r.tint)} />{v}</View> : v;
  }
  return (
    <View style={{ minHeight: 40, paddingHorizontal: 14, paddingVertical: 8, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
      <View style={{ flexShrink: 1, gap: 2 }}>
        <Txt s={r.label} size={13} color={enabled ? p.text : p.faint} nowrap={false} style={{ maxWidth: 300 }} />
        {r.detail ? <Txt s={r.detail} size={11} color={p.faint} nowrap={false} style={{ maxWidth: 330 }} /> : null}
      </View>
      {right}
    </View>
  );
}
function Confirm({ r, close }) {
  const p = usePal();
  const c = r.confirm;
  return (
    <View style={{ position: 'absolute', left: 0, top: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.47)', alignItems: 'center', justifyContent: 'center' }}>
      <View style={{ width: 336, backgroundColor: p.surface, borderRadius: 12, borderWidth: 1, borderColor: p.border, padding: 18, gap: 10 }}>
        <Txt s={c.title} size={13} w={700} color={p.text} />
        <Txt s={c.text} size={12} color={p.muted} nowrap={false} />
        <View style={{ flexDirection: 'row', justifyContent: 'flex-end', gap: 4 }}><SmallBtn title={c.cancel} onClick={close} /><SmallBtn title={c.ok} danger onClick={() => { close(); send(r, ''); }} /></View>
      </View>
    </View>
  );
}
export function Settings({ s, page: start = 'general', confirmFirst }) {
  const p = usePal();
  const pages = prefsSpec(s);
  const [id, setId] = useState(start);
  const [confirm, setConfirm] = useState(null);
  const page = pages.find((x) => x.id === id) || pages[0];
  return (
    <View style={{ flex: 1, backgroundColor: p.bg }}>
      <View style={{ backgroundColor: p.surface, paddingVertical: 8, alignItems: 'center', borderBottomWidth: 1, borderColor: p.border }}>
        <View style={{ flexDirection: 'row', gap: 4 }}>
          {pages.map((pg) => {
            const on = pg.id === page.id, col = on ? p.text : p.muted;
            return (
              <View key={pg.id} onClick={() => setId(pg.id)} style={{ paddingTop: 6, paddingBottom: 5, paddingHorizontal: 10, borderRadius: 7, backgroundColor: on ? p.selected : 'transparent', alignItems: 'center', gap: 3 }}>
                <Icon name={pg.symbol} size={22} color={col} /><Txt s={pg.title} size={11} color={col} />
              </View>
            );
          })}
        </View>
      </View>
      <ScrollView style={{ flex: 1 }}>
        <View style={{ paddingTop: 20, paddingHorizontal: 20, paddingBottom: 24 }}>
          {page.sections.map((sec, i) => (
            <View key={i} style={{ marginTop: i && sec.title ? 22 : i ? 8 : 0, gap: 8 }}>
              {sec.title ? <Txt s={sec.title} size={13} w={700} color={p.text} /> : null}
              <View style={{ backgroundColor: p.surface, borderRadius: 8, borderWidth: 1, borderColor: p.border }}>
                {sec.rows.map((r, j) => (
                  <View key={r.id || r.label + j}>
                    <Row r={r} onConfirm={setConfirm} />
                    {j + 1 < sec.rows.length ? <View style={{ height: 1, marginHorizontal: 14, backgroundColor: p.border }} /> : null}
                  </View>
                ))}
              </View>
              {sec.footer ? <Txt s={sec.footer} size={11} color={p.faint} nowrap={false} style={{ maxWidth: 560 }} /> : null}
            </View>
          ))}
        </View>
      </ScrollView>
      {confirm ? <Confirm r={confirm} close={() => setConfirm(null)} /> : null}
    </View>
  );
}
