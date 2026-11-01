// A test theme for an isolated headless session: one scene at a time, chosen by the file SCENE_FILE.
import { View, ThemeProvider, DEFAULT_TOKENS, fs } from '@lordos/sdk';
import { render } from '@lordos/runtime';
import { useEffect, useState } from 'react';
import { engine, useEngine } from '../../engine/index.js';
import { setLanguage } from '../../ui/i18n.js';
import { MainWindow, viewOf, WINDOWS, Shell } from '../../index.jsx';
import { fyne } from '../../ui/tokens.js';
import { MenuCtx, Pal } from '../../ui/widgets.jsx';
import { palette } from '../../ui/tokens.js';
const FILE = '/home/lord/LordOS-appui/home/scene.txt';
function Scene({ spec }) {
  const s = useEngine();
  if (!s.config) return null;
  const q = Object.fromEntries(spec.split(';').map((x) => x.split('=')));
  const w = Number(q.w || 1120), h = Number(q.h || 680);
  const v = q.view || 'main';
  let body;
  if (v === 'main') body = <MainWindow key={spec} s={s} filter={q.filter} mode={q.mode ? Number(q.mode) : 0} startTab={q.tab} renameFirst={q.rename === '1'} />;
  else body = viewOf(v, s, { page: q.page, remote: q.remote });
  return <View style={{ position: 'absolute', left: 0, top: 0, width: w, height: h, overflow: 'hidden' }}>{body}</View>;
}
function Root() {
  const [spec, setSpec] = useState('scene=home');
  useEffect(() => {
    let last = '';
    const t = setInterval(() => {
      try {
        const v = String(fs.read(FILE)).trim();
        if (v && v !== last) {
          last = v;
          const q = Object.fromEntries(v.split(';').map((x) => x.split('=')));
          engine.scene(q.scene);
          if (q.lang) engine.send('settings.set', { key: 'language', value: q.lang });
          if (q.theme) engine.send('settings.set', { key: 'theme', value: q.theme });
          setSpec(v);
        }
      } catch (e) { /* not there yet */ }
    }, 250);
    return () => clearInterval(t);
  }, []);
  return <View style={{ position: 'absolute', left: 0, top: 0, width: '100%', height: '100%', backgroundColor: '#000000' }}><Scene spec={spec} /></View>;
}
render(<ThemeProvider tokens={DEFAULT_TOKENS}><Root /></ThemeProvider>);
