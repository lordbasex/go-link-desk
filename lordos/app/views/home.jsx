// The main window: pkg/fyneview/main.go (top bar, tabs, this computer's panel, the filters) and
// tiles.go (cards, list, detailed list). Same data as remoteview.Home.
import { View, Text, Image, ScrollView, TextInput, clipboard } from '@lordos/sdk';
import { useContext, useEffect, useRef, useState } from 'react';
import { engine } from '../engine/index.js';
import { L, Lf } from '../ui/i18n.js';
import { Icon } from '../ui/icon.jsx';
import { Btn, IconBtn, MenuCtx, Txt, usePal } from '../ui/widgets.jsx';
import { TILE } from '../ui/tokens.js';
import { ddmm, ddmmyyyy, digits, formatCode, middleTruncate, osName } from '../ui/format.js';

function Thumb({ t, style }) {
  if (!t.thumb) return null;
  return t.thumb.data ? <Image pixels={t.thumb} style={style} /> : <Image src={t.thumb} style={[{ objectFit: 'fill' }, style]} />;
}
export function TopBar({ s, onSettings, initial = '' }) {
  const p = usePal();
  const menu = useContext(MenuCtx);
  const [addr, setAddr] = useState(initial);
  const [door, setDoor] = useState(0);
  const connect = () => {
    const code = digits(addr);
    if (code.length !== 9) return;
    setAddr('');
    engine.send('connect', { kind: door === 1 ? 'claim' : 'join', code });
  };
  const doorMenu = (e) => menu.open(e, [L('Session'), L('Link (unattended)')].map((title, i) => ({ title, checked: i === door, run: () => setDoor(i) })));
  return (
    <View style={{ flexDirection: 'row', height: 30, gap: 8, alignItems: 'center' }}>
      <View style={{ flex: 1, maxWidth: 520, height: 30, borderRadius: 7, backgroundColor: p.bg, borderWidth: 1, borderColor: p.fieldBorder, flexDirection: 'row', alignItems: 'center', paddingLeft: 10, paddingRight: 8 }}>
        <Icon name="display" size={15} color={p.faint} />
        <View style={{ flex: 1, justifyContent: 'center', marginLeft: 4 }}>
          <TextInput value={addr} onChange={(v) => setAddr(formatCode(v))} onSubmit={connect} size={14} placeholder={L('Its 9-digit code')}
            style={{ backgroundColor: 'transparent', borderWidth: 0, height: 28, paddingHorizontal: 4 }} />
        </View>
        <View onClick={doorMenu} style={{ flexDirection: 'row', gap: 3, alignItems: 'center', marginLeft: 6 }}>
          <Txt s={door ? L('Link (unattended)') : L('Session')} size={11} w={500} color={p.text} />
          <Icon name="chevrons-up-down" size={10} color={p.muted} />
        </View>
      </View>
      <Btn title={L('Connect')} symbol="arrow.right" primary trailing onClick={connect} />
      <Btn title={L('Try the demo')} symbol="play.circle" onClick={() => engine.send('demo')} />
      <View style={{ flex: 1 }} />
      <Btn symbol="gearshape" fill="transparent" onClick={onSettings} />
    </View>
  );
}
export function Tab({ title, symbol, selected, onClick, onClose }) {
  const p = usePal();
  return (
    <View onClick={onClick} style={{ height: 30, paddingLeft: 12, paddingRight: onClose ? 8 : 14, borderRadius: 8, backgroundColor: selected ? p.selected : 'transparent', flexDirection: 'row', gap: 7, alignItems: 'center' }}>
      <Icon name={symbol} size={14} color={p.accent} />
      <Txt s={title} size={12.5} w={500} color={selected ? p.text : p.muted} />
      {onClose ? <IconBtn name="xmark" size={11} color={p.faint} onClick={onClose} /> : null}
    </View>
  );
}
function Panel({ s, flash, copied, onPerm }) {
  const p = usePal();
  const code = s.session ? formatCode(s.session.code) : '';
  const link = s.session?.invite ? `${s.webURL.replace(/\/+$/, '')}/c/${s.session.invite}` : '';
  const copy = () => { if (code) { clipboard.set(code); flash(); } };
  const copyLink = () => { if (link) { clipboard.set(link); flash(); } };
  const a = s.active;
  let status, ok = false;
  if (a) status = !a.recordingSince ? Lf('In session with %s', a.name || L('Someone')) : Lf('In session with %s, who is recording your screen', a.name || L('Someone'));
  else if (s.signal === 'connected') { status = L('Connected'); ok = true; }
  else if (s.signal === 'disconnected') status = L('Offline, reconnecting…');
  else status = L('Connecting…');
  const allPerms = s.perms.every((x) => x.granted);
  return (
    <View style={{ backgroundColor: p.tintBG, borderRadius: 14, borderWidth: 1, borderColor: p.tintBorder, paddingVertical: 18, paddingHorizontal: 24, flexDirection: 'row', justifyContent: 'space-between' }}>
      <View style={{ flexShrink: 1 }}>
        <Txt s={L('This computer').toUpperCase()} size={11} w={600} color={p.tintText} style={{ marginBottom: 2 }} />
        <View style={{ flexDirection: 'row', gap: 12, alignItems: 'center', marginBottom: 6 }}>
          <View onClick={copy}><Txt s={code || '— — —'} size={40} w={700} color={p.accent} /></View>
          {copied ? <Txt s={L('Copied')} size={12} w={600} color={p.ok} /> : null}
        </View>
        {link ? (
          <View style={{ flexDirection: 'row', gap: 6, alignItems: 'center', marginBottom: 8 }}>
            <Icon name="link" size={13} color={p.faint} />
            <View onClick={copyLink}><Txt s={middleTruncate(link, 64)} size={12} color={p.muted} /></View>
          </View>
        ) : null}
        <Txt s={L('Share this code only with someone you know. Nobody legitimate will call you asking for access to your computer.')} size={11.5} color={p.faint} nowrap={false} style={{ maxWidth: 560 }} />
      </View>
      <View style={{ alignItems: 'flex-end', gap: 12 }}>
        <View style={{ flexDirection: 'row', gap: 8 }}>
          <Btn symbol="doc.on.doc" enabled={!!code} onClick={copy} />
          <Btn symbol="arrow.clockwise" enabled={!!s.session} onClick={() => engine.send('code.new')} />
          {s.perms.length ? <Btn symbol={allPerms ? 'checkmark.shield' : 'exclamationmark.shield'} tint={allPerms ? p.ok : p.accent} fill={allPerms ? undefined : p.tintBG} onClick={onPerm} /> : null}
        </View>
        <View style={{ flexDirection: 'row', gap: 7, alignItems: 'center' }}>
          <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: a ? p.accent : ok ? p.ok : p.danger }} />
          <Txt s={status} size={12} color={p.muted} />
        </View>
      </View>
    </View>
  );
}
function FilterTab({ title, on, onClick }) {
  const p = usePal();
  return (
    <View onClick={onClick} style={{ height: 19 }}>
      <Txt s={title} size={14} w={on ? 600 : 400} color={on ? p.text : p.faint} style={{ lineHeight: 18 }} />
      {on ? <View style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 1.5, backgroundColor: p.text }} /> : null}
    </View>
  );
}
function tilesOf(s, filter) {
  if (filter === 'recents') {
    return s.recents.filter((r) => r.code).map((r) => {
      const code = formatCode(r.code), name = r.name || L('Unnamed computer');
      return { id: code, title: code, subtitle: `${name} · ${ddmm(r.at)}`, name, code, system: osName(r.os), when: ddmmyyyy(r.at), thumb: r.thumb };
    });
  }
  return s.devices.filter((d) => filter !== 'favorites' || d.favorite).map((d) => {
    const title = d.label || d.name || L('Linked computer');
    return { id: d.id, title, subtitle: [osName(d.os), d.last_used ? ddmm(d.last_used) : ''].filter(Boolean).join(' · '), name: title, code: L('Linked'), system: osName(d.os),
      when: d.last_used ? ddmmyyyy(d.last_used) : '', thumb: d.thumb, favorite: d.favorite };
  });
}
function Tile({ t, linked, mode, onRename }) {
  const p = usePal();
  const menu = useContext(MenuCtx);
  const [hover, setHover] = useState(false);
  const sym = 'display';
  const iconTint = mode === 0 ? '#ffffff' : p.faint;
  const open = () => (linked ? engine.send('device.connect', { device: t.id }) : engine.send('recent.connect', { code: digits(t.id) }));
  const star = <IconBtn name={t.favorite ? 'star.fill' : 'star'} color={t.favorite ? p.accent : iconTint} onClick={() => engine.send('device.favorite', { device: t.id })} />;
  const items = () => (linked ? [
    { title: L('Connect'), run: open },
    { title: t.favorite ? L('Remove from favorites') : L('Add to favorites'), run: () => engine.send('device.favorite', { device: t.id }) },
    { title: L('Rename…'), run: () => onRename(t) }, '-',
    { title: L('Forget this computer'), run: () => engine.send('device.forget', { device: t.id }) },
  ] : [
    { title: L('Connect'), run: open },
    { title: L('Copy the code'), run: () => clipboard.set(t.code) }, '-',
    { title: L('Remove from the list'), run: () => engine.send('recent.remove', { code: digits(t.id) }) },
  ]);
  const more = <IconBtn name="ellipsis" color={iconTint} onClick={(e) => menu.open(e, items())} />;
  const events = { onClick: open, onContextMenu: (e) => menu.open(e, items()), onMouseEnter: () => setHover(true), onMouseLeave: () => setHover(false) };
  if (mode === 0) {
    return (
      <View {...events} style={{ width: TILE.w, height: TILE.h, borderRadius: 10, backgroundColor: p.surface2, overflow: 'hidden' }}>
        {t.thumb ? <Thumb t={t} style={{ position: 'absolute', left: 0, top: 0, width: TILE.w, height: TILE.h }} /> : (
          <View style={{ position: 'absolute', left: 0, top: 0, width: TILE.w, height: TILE.h - 34, alignItems: 'center', justifyContent: 'center' }}><Icon name={sym} size={34} color={p.faint} /></View>
        )}
        <View style={{ position: 'absolute', left: 0, bottom: 0, width: TILE.w, height: 36, backgroundColor: 'rgba(0,0,0,0.72)' }} />
        <Icon name={sym} size={15} color={p.text} style={{ position: 'absolute', left: 9, top: TILE.h - 27 }} />
        <Txt s={t.title} size={12.5} w={600} color="#ffffff" style={{ position: 'absolute', left: 30, top: TILE.h - 26, width: TILE.w - 60, lineHeight: 16, overflow: 'hidden' }} />
        <View style={{ position: 'absolute', left: TILE.w - 30, top: TILE.h - 29 }}>{more}</View>
        {linked ? <View style={{ position: 'absolute', left: TILE.w - 28, top: 6 }}>{star}</View> : null}
        {hover ? <View style={{ position: 'absolute', left: 0, top: 0, width: TILE.w, height: TILE.h, borderRadius: 10, borderWidth: 2, borderColor: p.accent }} /> : null}
      </View>
    );
  }
  if (mode === 1) {
    return (
      <View {...events} style={{ height: TILE.row, borderRadius: 8, backgroundColor: hover ? p.surface2 : p.surface, marginBottom: 8 }}>
        <View style={{ position: 'absolute', left: 8, top: 7, width: 70, height: 42, borderRadius: 5, backgroundColor: p.video, overflow: 'hidden' }}>{t.thumb ? <Thumb t={t} style={{ width: 70, height: 42 }} /> : null}</View>
        <Txt s={t.title} size={13} w={600} color={p.text} style={{ position: 'absolute', left: 90, top: 9, right: 70, lineHeight: 18 }} />
        <Txt s={t.subtitle} size={11.5} color={p.faint} style={{ position: 'absolute', left: 90, top: 30, right: 70, lineHeight: 16 }} />
        <View style={{ position: 'absolute', right: 8, top: 17, flexDirection: 'row', gap: 4 }}>{linked ? star : null}{more}</View>
      </View>
    );
  }
  const cell = (v, at, w, col) => <Txt s={v} size={12.5} w={w} color={col} style={{ position: 'absolute', left: at, top: 11, lineHeight: 16 }} />;
  return (
    <View {...events} style={{ height: TILE.detail, borderRadius: 8, backgroundColor: hover ? p.surface2 : 'transparent', borderBottomWidth: 1, borderColor: p.border }}>
      <Icon name={sym} size={15} color={iconTint} style={{ position: 'absolute', left: 10, top: 11 }} />
      {cell(t.name, 34, 500, p.text)}{cell(t.code, '36%', 400, p.muted)}{cell(t.system, '58%', 400, p.muted)}{cell(t.when, '74%', 400, p.muted)}
      {linked ? <View style={{ position: 'absolute', right: 41, top: 8 }}>{star}</View> : null}
      <View style={{ position: 'absolute', right: 13, top: 8 }}>{more}</View>
    </View>
  );
}
function DetailHeader() {
  const p = usePal();
  const cols = [['Name', 34], ['Code', '36%'], ['System', '58%'], ['Last connection', '74%']];
  return (
    <View style={{ height: 28, borderBottomWidth: 1, borderColor: p.border }}>
      {cols.map(([k, at]) => <Txt key={k} s={L(k).toUpperCase()} size={10.5} w={600} color={p.faint} style={{ position: 'absolute', left: at, top: 7, lineHeight: 14 }} />)}
    </View>
  );
}
function Rename({ t, close }) {
  const p = usePal();
  const [v, set] = useState(t.name);
  const save = () => { close(); engine.send('device.rename', { device: t.id, label: v }); };
  return (
    <View style={{ position: 'absolute', left: 0, top: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.47)', alignItems: 'center', justifyContent: 'center' }}>
      <View style={{ backgroundColor: p.surface, borderRadius: 12, borderWidth: 1, borderColor: p.border, padding: 16, gap: 12, width: 322 }}>
        <Txt s={L('Name of this computer here')} size={13} w={600} color={p.text} />
        <View style={{ width: 288, height: 34, backgroundColor: p.bg, borderRadius: 7, borderWidth: 1, borderColor: p.fieldBorder, justifyContent: 'center' }}>
          <TextInput value={v} onChange={set} onSubmit={save} autoFocus size={13} style={{ backgroundColor: 'transparent', borderWidth: 0, height: 32 }} />
        </View>
        <View style={{ flexDirection: 'row', gap: 8 }}><Btn title={L('Cancel')} onClick={close} /><Btn title={L('Save')} primary onClick={save} /></View>
      </View>
    </View>
  );
}
export function Home({ s, onPerm, startFilter, startMode = 0, renameFirst }) {
  const p = usePal();
  const [filter, setFilter] = useState(startFilter);
  const [mode, setMode] = useState(startMode);
  const [copied, setCopied] = useState(false);
  const [renaming, setRenaming] = useState(null);
  const timer = useRef(0);
  const flash = () => { setCopied(true); clearTimeout(timer.current); timer.current = setTimeout(() => setCopied(false), 1500); };
  const f = filter || (s.devices.length === 0 && s.recents.length > 0 ? 'recents' : 'all');
  const linked = f !== 'recents';
  const items = tilesOf(s, f);
  useEffect(() => { if (renameFirst && items[0]) setRenaming(items[0]); }, [renameFirst]);
  const note = f === 'recents' ? L('The computers you connect to with a code show up here.') : f === 'favorites' ? L('Mark a linked computer with the star to see it here.')
    : L('Computers you link with their pairing code show up here, to connect to them without a code.');
  return (
    <View style={{ flex: 1 }}>
      <View style={{ paddingTop: 24, paddingHorizontal: 28 }}><Panel s={s} flash={flash} copied={copied} onPerm={onPerm} /></View>
      <View style={{ paddingTop: 26, paddingBottom: 13, paddingHorizontal: 28, flexDirection: 'row', gap: 18, alignItems: 'center' }}>
        <FilterTab title={L('My computers')} on={f === 'all'} onClick={() => setFilter('all')} />
        <FilterTab title={L('Favorites')} on={f === 'favorites'} onClick={() => setFilter('favorites')} />
        <FilterTab title={L('Recent sessions')} on={f === 'recents'} onClick={() => setFilter('recents')} />
        <View style={{ flex: 1 }} />
        {f === 'recents' && s.recents.length ? <View onClick={() => engine.send('recent.clear')}><Txt s={L('Clear')} size={12} color={p.faint} /></View> : null}
        <View style={{ backgroundColor: p.surface, borderRadius: 7, paddingVertical: 3, paddingHorizontal: 6, flexDirection: 'row', gap: 2 }}>
          {['square.grid.2x2', 'rectangle.grid.1x2', 'list.bullet'].map((sym, i) => <IconBtn key={sym} name={sym} color={i === mode ? p.accent : p.faint} onClick={() => setMode(i)} />)}
        </View>
      </View>
      <ScrollView style={{ flex: 1 }}>
        <View style={{ paddingTop: 1, paddingBottom: 12, paddingHorizontal: 28 }}>
          {items.length === 0 ? (
            <View style={{ borderRadius: 10, borderWidth: 1, borderColor: p.dashed, padding: 18 }}><Txt s={note} size={13} color={p.muted} nowrap={false} style={{ maxWidth: 560 }} /></View>
          ) : mode === 0 ? (
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: TILE.gap }}>{items.map((t) => <Tile key={t.id} t={t} linked={linked} mode={0} onRename={setRenaming} />)}</View>
          ) : (
            <View>{mode === 2 ? <DetailHeader /> : null}{items.map((t) => <Tile key={t.id} t={t} linked={linked} mode={mode} onRename={setRenaming} />)}</View>
          )}
        </View>
      </ScrollView>
      <View style={{ paddingTop: 6, paddingRight: 14, paddingBottom: 8, alignItems: 'flex-end' }}><Txt s={`v${s.version}`} size={11} color={p.faint} /></View>
      {renaming ? <Rename t={renaming} close={() => setRenaming(null)} /> : null}
    </View>
  );
}
