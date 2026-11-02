// Desk go-link: go-link Desk's window as a native LordOS app. Everything it knows comes from
// engine/index.js. It's one window component that shows the view its window was opened for, since
// go-link's request, session panel, chat, files, call, update, About and Settings are windows of their own.
import { View, Image, useAppWindow, useTheme, wm, clipboard, useSettings, statusItem } from '@lordos/sdk';
import { useEffect, useMemo, useRef, useState } from 'react';
import { engine, useEngine } from './engine/index.js';
import { language, L, setLanguage } from './ui/i18n.js';
import { Logo } from './ui/icon.jsx';
import { fyne, palette } from './ui/tokens.js';
import { MenuCtx, MenuLayer, Pal } from './ui/widgets.jsx';
import { Home, Tab, TopBar } from './views/home.jsx';
import { Settings } from './views/settings.jsx';
import { About, CallAsk, CallWindow, Consent, SessionPanel, UpdateWindow } from './views/small.jsx';
import { RemoteChat, RemoteFiles, RemotePage } from './views/remote.jsx';
import { trayItems } from './views/tray.jsx';

const ID = 'go-link-desk';
export const WINDOWS = {
  consent: { w: 440, h: 660, title: 'Connection request' }, session: { w: 320, h: 650, title: 'go-link Desk session' }, callask: { w: 380, h: 199, title: 'Video call' }, call: { w: 480, h: 480, title: 'Video call' },
  settings: { w: 640, h: 640, title: 'Settings' }, about: { w: 340, h: 370, title: 'About go-link Desk' }, update: { w: 440, h: 139, title: 'Software update' }, chat: { w: 340, h: 420, title: 'Chat' }, files: { w: 520, h: 480, title: 'Files' },
};
const openView = (view, extra = {}) => { const d = WINDOWS[view]; wm.open(ID, { instance: extra.remote ? `${view}-${extra.remote}` : view, args: { view, ...extra }, frame: { w: d.w, h: d.h } }); };
const closeView = (view) => wm.close(`${ID}#${view}`, true);

export function Shell({ children }) {
  const [menu, setMenu] = useState(null);
  const root = useRef(null);
  const api = useMemo(() => ({
    open(e, items) {
      let ox = 0, oy = 0;
      try { const b = lord.getLayout(root.current.id); ox = b[0]; oy = b[1]; } catch (x) { /* */ }
      setMenu({ x: (e?.x ?? 40) - ox, y: (e?.y ?? 40) - oy + 8, items });
    },
    close() { setMenu(null); },
  }), []);
  return <MenuCtx.Provider value={api}><View ref={root} style={{ flex: 1 }}>{children}<MenuLayer menu={menu} close={api.close} /></View></MenuCtx.Provider>;
}
export function MainWindow({ s, startTab = 'home', filter, mode, renameFirst }) {
  const p = palette(s.config.theme);
  const [tab, setTab] = useState(startTab);
  const act = useRef({});
  const selected = s.remotes.find((r) => r.id === tab);
  const known = useRef(new Set());
  useEffect(() => {          // a new session gets the front, as a new tab does
    for (const r of s.remotes) if (!known.current.has(r.id)) { known.current.add(r.id); setTab(r.id); }
    if (tab !== 'home' && !selected) setTab('home');
  }, [s.remotes]);
  return (
    <Pal.Provider value={p}>
      <Shell>
        <View style={{ flex: 1, backgroundColor: p.bg }}>
          <View style={{ backgroundColor: p.surface, paddingTop: 8, borderBottomWidth: 1, borderColor: p.border }}>
            <View style={{ paddingHorizontal: 12, marginBottom: 8 }}><TopBar s={s} onSettings={() => openView('settings')} /></View>
            <View style={{ paddingHorizontal: 10, flexDirection: 'row', gap: 2 }}>
              <Tab title="go-link Desk" symbol="house.fill" selected={tab === 'home'} onClick={() => setTab('home')} />
              {s.remotes.map((r) => <Tab key={r.id} title={r.title} symbol="display" selected={tab === r.id} onClick={() => setTab(r.id)} onClose={() => engine.send('remote.close', { remote: r.id })} />)}
            </View>
          </View>
          {selected ? <RemotePage r={selected} onChat={() => openView('chat', { remote: selected.id })} onFiles={() => openView('files', { remote: selected.id })} />
            : <Home s={s} onPerm={() => openView('settings')} startFilter={filter} startMode={mode} renameFirst={renameFirst} />}
        </View>
      </Shell>
    </Pal.Provider>
  );
}
export function viewOf(view, s, args = {}, close) {
  const f = fyne(s.config.theme), p = palette(s.config.theme);
  const r = args.remote ? s.remotes.find((x) => x.id === args.remote) : null;
  switch (view) {
    case 'consent': return <Consent f={f} s={s} close={close} />;
    case 'session': return <SessionPanel f={f} s={s} />;
    case 'callask': return <CallAsk f={f} s={s} close={close} />;
    case 'call': return <CallWindow f={f} s={s} />;
    case 'update': return <UpdateWindow f={f} s={s} close={close} version={s.version} />;
    case 'about': return <About f={f} s={s} />;
    case 'chat': return r ? <RemoteChat f={f} r={r} /> : null;
    case 'files': return r ? <RemoteFiles f={f} r={r} /> : null;
    case 'settings': return <Pal.Provider value={p}><Shell><Settings s={s} page={args.page} /></Shell></Pal.Provider>;
    default: return null;
  }
}
export function Icon({ size }) { return <Logo size={size} />; }

export default function App() {
  const win = useAppWindow();
  const s = useEngine();
  const view = win?.args?.view;
  if (s.config) setLanguage(s.config.language);
  useEffect(() => { if (win?.id && view && WINDOWS[view]) wm.setTitle(win.id, L(WINDOWS[view].title)); }, [view, language()]);
  useEffect(() => {
    if (view || !win?.id) return;
    const act = (a) => { if (a === 'settings') openView('settings'); else if (a === 'about') openView('about'); else if (a === 'updates') openView('update'); else if (a === 'quit') wm.close(win.id); else wm.focus(win.id); };
    wm.setMenus(win.id, [{ label: 'go-link Desk', items: trayItems(s, act).map((it) => (it === '-' ? '-' : { label: it.title, onClick: it.run, disabled: it.disabled })) }]);
  }, [s.session, s.active, view]);
  // the item by the clock: go-link's tray menu, red while a session lasts
  const tray = useRef({});
  useEffect(() => {
    if (view || !win?.id) return undefined;
    const act = (a) => { if (a === 'settings') openView('settings'); else if (a === 'about') openView('about'); else if (a === 'updates') openView('update'); else if (a === 'quit') wm.close(win.id); else wm.focus(win.id); };
    const menu = trayItems(s, act).map((it, i) => {
      if (it === '-') return { line: true };
      const id = `m${i}`;
      tray.current[id] = it.run;
      return { id, label: it.title, off: !!it.disabled };
    });
    statusItem.set({ icon: 'icon.png', title: 'go-link Desk', alert: !!s.active, menu });
    return statusItem.onSelect((id) => tray.current[id]?.());
  }, [s.session, s.active, view, language()]);
  // the windows the engine's state asks for, from the main window only
  useEffect(() => {
    if (view) return;
    for (const [key, v] of [['consent', 'consent'], ['active', 'session'], ['callAsk', 'callask'], ['call', 'call']]) { if (s[key]) openView(v); else closeView(v); }
  }, [s.consent, s.active, s.callAsk, s.call]);
  if (!s.config) return null;
  if (view) return viewOf(view, s, win.args, () => wm.close(win.id, true));
  return <MainWindow s={s} />;
}
