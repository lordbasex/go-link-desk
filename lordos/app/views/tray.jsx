// The menu of go-link's tray icon (internal/gui/gui.go, setupTray). LordOS has no third-party top-bar
// icon yet (GO-LINK.md section 7, to design): this is what the icon will open; for now it opens from
// the app menu.
import { View } from '@lordos/sdk';
import { engine } from '../engine/index.js';
import { L, Lf } from '../ui/i18n.js';
import { formatCode } from '../ui/format.js';
export function trayItems(s, act) {
  const a = s.active, rec = a?.recordingSince;
  let end = L('Disconnect');
  if (a?.name) end = Lf('Disconnect %s', a.name);
  if (a && rec) end = a.name ? Lf('Disconnect %s (recording your screen)', a.name) : L('Disconnect (recording your screen)');
  return [
    { title: s.session ? Lf('Code: %s', formatCode(s.session.code)) : L('No code yet'), disabled: true },
    { title: end, disabled: !a, run: () => engine.send('session.end') }, '-',
    { title: L('Open go-link Desk'), run: () => act('open') },
    { title: L('Settings…'), run: () => act('settings') },
    { title: L('Check for Updates…'), run: () => act('updates') },
    { title: L('Test My Connection…'), run: () => engine.send('network.test') },
    { title: L('About go-link Desk'), run: () => act('about') }, '-',
    { title: L('Quit go-link Desk'), run: () => act('quit') },
  ];
}
