// go-link's small windows, in Fyne's look: consent.go, session.go, chat.go, call.go, update.go, about.go.
import { View, Text, ScrollView, Image } from '@lordos/sdk';
import { useEffect, useState } from 'react';
import { engine } from '../engine/index.js';
import { L, Lf } from '../ui/i18n.js';
import { Logo } from '../ui/icon.jsx';
import { hhmm } from '../ui/format.js';
import { FButton, FCheck, FEntry, FLabel, FProgress, FSep, FWindow, Spacer } from '../ui/fyne.jsx';
const FULL = { view: true, control: true, audio: true, clipboard: true, files: true, record: false };
const VIEW = { view: true, control: false, audio: false, clipboard: false, files: false, record: false };
function PermChecks({ f, value, onChange }) {
  const box = (k, label) => <FCheck f={f} label={L(label)} on={!!value[k]} onChange={(v) => onChange({ ...value, [k]: v })} />;
  return <>{box('control', 'Use your mouse and keyboard')}{box('audio', "Hear your computer's sound")}{box('clipboard', 'Share the clipboard (copy and paste)')}{box('files', 'Send and receive files')}{box('record', 'Take screenshots and record the screen')}</>;
}
export function Consent({ f, s, close }) {
  const c = s.consent;
  const [perms, setPerms] = useState(() => (!c?.link && s.config.request_ticks === '' ? FULL : VIEW));
  const [left, setLeft] = useState(() => Math.ceil(((c?.deadline ?? 0) - Date.now()) / 1000));
  useEffect(() => { const t = setInterval(() => setLeft(Math.max(0, Math.ceil(((c?.deadline ?? 0) - Date.now()) / 1000))), 1000); return () => clearInterval(t); }, [c?.id]);
  if (!c) return <FWindow f={f} />;
  const name = c.name || L('Someone');
  const answer = (accept) => { engine.send('consent.answer', { consent: c.id, accept, permissions: perms }); close?.(); };
  return (
    <FWindow f={f}>
      <FLabel f={f} bold>{c.link ? Lf('%s wants to link this browser to your computer.', name) : Lf('%s wants to see your screen.', name)}</FLabel>
      <FLabel f={f}>{Lf('Browser: %s', c.browser || L('unknown'))}</FLabel>
      <FLabel f={f} importance="low">{L('The name and the browser are what the other side says; go-link Desk cannot check them.')}</FLabel>
      {c.link ? <FLabel f={f} importance="warning">{L('If you accept, this browser will come back without asking you, whenever go-link Desk is open, with the permissions you tick now. You can revoke it in Settings.')}</FLabel> : null}
      <FSep f={f} />
      <FLabel f={f}>{`${L('If you accept, they will be able to:')}\n  • ${L('See your screen')}`}</FLabel>
      <PermChecks f={f} value={perms} onChange={setPerms} />
      {c.warning ? <><FSep f={f} /><FLabel f={f} bold importance="danger">{L('Did someone call you and ask you to do this? Reject it and hang up. Nobody legitimate calls asking for access to your computer: not your bank, not the police, not a support line.')}</FLabel></> : null}
      <FLabel f={f} importance="low">{Lf('Rejected on its own in %d s.', left)}</FLabel>
      <View style={{ flexDirection: 'row', gap: 4 }}><Spacer />
        <FButton f={f} label={L('Reject')} icon="fyne-cancel" onClick={() => answer(false)} />
        <FButton f={f} label={L('Accept')} icon="fyne-check" importance="high" onClick={() => answer(true)} />
      </View>
    </FWindow>
  );
}
export function ChatLines({ f, lines }) {
  return (
    <ScrollView style={{ flex: 1, minHeight: 112 }}>
      <View style={{ gap: 4 }}>{lines.map((l, i) => (
        <FLabel key={i} f={f} importance={l.mine ? 'low' : 'medium'} italic={l.note}>{l.note ? l.text : `${l.who} · ${hhmm(l.at)}\n${l.text}`}</FLabel>
      ))}</View>
    </ScrollView>
  );
}
export function ChatInput({ f, onSend, onTyping, typing }) {
  const [v, set] = useState('');
  const go = () => { if (v.trim() && onSend(v)) set(''); };
  return (
    <View style={{ gap: 4 }}>
      {typing ? <FLabel f={f} importance="low">{typing}</FLabel> : null}
      <View style={{ flexDirection: 'row', gap: 4 }}>
        <FEntry f={f} value={v} onChange={(t) => { set(t); onTyping?.(t !== ''); }} onSubmit={go} placeholder={L('Write a message')} style={{ flex: 1 }} />
        <FButton f={f} icon="fyne-mail-send" onClick={go} />
      </View>
    </View>
  );
}
export function recordingText(a) {
  if (a.recordingSince) return Lf('● Recording your screen since %s', hhmm(a.recordingSince));
  if (a.screenshots === 1) return Lf('Took a screenshot at %s', hhmm(a.lastScreenshot));
  if (a.screenshots > 1) return Lf('Took %d screenshots, the last at %s', a.screenshots, hhmm(a.lastScreenshot));
  return '';
}
function transferText(t) {
  const pct = t.size ? Math.floor(t.done * 100 / t.size) : 0;
  if (t.over && t.err) return Lf('%s: stopped', t.name);
  if (t.over && t.upload) return Lf('Received %s, in Downloads', t.name);
  if (t.over) return Lf('Sent %s', t.name);
  return t.upload ? Lf('Receiving %s… %d%%', t.name, pct) : Lf('Sending %s… %d%%', t.name, pct);
}
export function SessionPanel({ f, s }) {
  const a = s.active;
  if (!a) return <FWindow f={f} />;
  const name = a.name || L('Someone');
  const rec = recordingText(a);
  const lines = s.chat.map((m) => ({ who: m.fromViewer ? L('Viewer') : L('You'), mine: !m.fromViewer, at: m.at, text: m.text }));
  return (
    <FWindow f={f}>
      <FLabel f={f} bold>{a.unattended ? Lf('%s is viewing your screen (unattended access)', name) : Lf('%s is viewing your screen', name)}</FLabel>
      {rec ? <FLabel f={f} bold importance="danger">{rec}</FLabel> : null}
      <FLabel f={f}>{a.browser}</FLabel>
      <FLabel f={f}>{Lf('Since %s', hhmm(a.since))}</FLabel>
      <FLabel f={f}>{L('Can see your screen, and:')}</FLabel>
      <PermChecks f={f} value={a.permissions} onChange={(permissions) => engine.send('session.permissions', { permissions })} />
      {s.transfer ? <FLabel f={f}>{transferText(s.transfer)}</FLabel> : null}
      <FLabel f={f} bold>{L('Chat')}</FLabel>
      <ChatLines f={f} lines={lines} />
      <ChatInput f={f} typing={s.typing ? Lf('%s is typing…', a.name || L('The viewer')) : ''} onTyping={(on) => engine.send('chat.typing', { on })} onSend={(text) => { engine.send('chat.send', { text }); return true; }} />
      <FButton f={f} label={L('Disconnect')} icon="fyne-logout" importance="danger" onClick={() => engine.send('session.end')} />
    </FWindow>
  );
}
export function CallAsk({ f, s, close }) {
  const c = s.callAsk;
  const [cam, setCam] = useState(!!c?.video), [mic, setMic] = useState(true);
  if (!c) return <FWindow f={f} />;
  const answer = (accept) => { engine.send('call.answer', { accept, camera: cam, mic }); close?.(); };
  return (
    <FWindow f={f}>
      <FLabel f={f} bold>{Lf('%s wants a video call.', c.name || L('Someone'))}</FLabel>
      <FCheck f={f} label={L('Turn on my camera')} on={cam} onChange={setCam} />
      <FCheck f={f} label={L('Turn on my microphone')} on={mic} onChange={setMic} />
      <View style={{ flexDirection: 'row', gap: 4 }}><Spacer />
        <FButton f={f} label={L('Decline')} icon="fyne-cancel" onClick={() => answer(false)} />
        <FButton f={f} label={L('Accept')} icon="fyne-check" importance="high" onClick={() => answer(true)} />
      </View>
    </FWindow>
  );
}
export function CallWindow({ f, s }) {
  const c = s.call;
  if (!c) return <FWindow f={f} />;
  const name = c.name || L('Someone');
  const note = !c.viewerCamera && !c.viewerMic ? Lf('%s has the camera and microphone off.', name) : !c.viewerCamera ? Lf('%s has the camera off.', name) : !c.viewerMic ? Lf('%s has the microphone off.', name) : Lf('In a call with %s', name);
  return (
    <View style={{ flex: 1, backgroundColor: f.bg }}>
      <View style={{ flex: 1, minHeight: 360, backgroundColor: '#000000' }} />
      <FLabel f={f}>{note}</FLabel>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
        <FCheck f={f} label={L('My camera')} on={c.hostCamera} onChange={(camera) => engine.send('call.set', { camera })} />
        <FCheck f={f} label={L('My microphone')} on={c.hostMic} onChange={(mic) => engine.send('call.set', { mic })} />
        <Spacer /><FButton f={f} label={L('Hang up')} icon="fyne-cancel" importance="danger" onClick={() => engine.send('call.end')} />
      </View>
    </View>
  );
}
// The updater stays off on LordOS (APT does it): Check for Updates opens the Store from the main view, and
// this window keeps go-link's states.
export function UpdateWindow({ f, s, close, version }) {
  const u = s.update;
  let title = '', text = '', buttons = [];
  const b = (label, run, importance) => buttons.push(<FButton key={label} f={f} label={label} onClick={run} importance={importance} style={{ height: 33 }} />);
  if (u.state === 'checking') { title = L('Checking for updates…'); b(L('Close'), close); }
  else if (u.state === 'error') { title = L('Could not check for updates'); text = L('There is no connection to GitHub, where the versions are. Try again later.'); b(L('Close'), close); }
  else if (u.state === 'current') { title = L('go-link Desk is up to date'); text = Lf('You have %s, the newest version.', version); b(L('OK'), close); }
  else if (u.state === 'available') {
    title = Lf('go-link Desk %s is available', u.latest); text = `${Lf('You have %s.', version)} ${L('This copy cannot replace itself: download the new one from its page and install it as before.')}`;
    b(L("What's new"), () => {}); b(L('Later'), close); b(L('Download'), () => {}, 'high');
  }
  return (
    <FWindow f={f}>
      <FLabel f={f} bold>{title}</FLabel>
      {text ? <FLabel f={f}>{text}</FLabel> : null}
      {u.state === 'checking' ? <FProgress f={f} value={0.4} /> : null}
      <View style={{ flexDirection: 'row', gap: 4 }}><Spacer />{buttons}</View>
    </FWindow>
  );
}
export function About({ f, s }) {
  const Center = ({ children, ...o }) => <FLabel f={f} center {...o}>{children}</FLabel>;
  return (
    <FWindow f={f}>
      <View style={{ alignItems: 'center' }}><Logo size={96} /></View>
      <Center bold>go-link Desk</Center>
      <Center>{`v${s.version}`}</Center>
      <Center>{`${L('Remote desktop over a direct connection, encrypted end to end.')}\n${L('Free and open source software (MIT license).')}`}</Center>
      <View style={{ alignItems: 'center' }}><Text style={{ fontSize: 14, color: f.primary, paddingVertical: 8 }}>desk.go-link.org</Text></View>
      <View style={{ flexDirection: 'row', justifyContent: 'center', gap: 4 }}>
        <FButton f={f} label={L('Check for Updates…')} onClick={() => engine.send('update.check')} />
        <FButton f={f} label={L('Test My Connection…')} onClick={() => engine.send('network.test')} />
      </View>
      <Center importance="low">© 2026 Federico Pereira</Center>
    </FWindow>
  );
}
