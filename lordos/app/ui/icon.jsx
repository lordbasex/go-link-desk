// Lucide and Fyne icons as go-link draws them. The engine has no SVG, so a mask made by
// tools/gen-icons.mjs gets the color here, once per icon, size and color.
import { Image, View } from '@lordos/sdk';
import { ICONS, SIZES, LOGO } from './icons.gen.js';
// fyneview's "symbols": the Mac's names to these icons
const SYMBOLS = { gearshape: 'gear', display: 'monitor', desktopcomputer: 'monitor', link: 'link', 'doc.on.doc': 'copy', 'arrow.clockwise': 'rotate-cw',
  'checkmark.shield': 'shield-check', 'exclamationmark.shield': 'shield-alert', 'house.fill': 'house-fill', 'square.grid.2x2': 'layout-grid',
  'rectangle.grid.1x2': 'rows-2', 'list.bullet': 'list', star: 'star', 'star.fill': 'star-fill', ellipsis: 'ellipsis', 'arrow.right': 'arrow-right',
  'display.2': 'monitor-smartphone', 'lock.shield': 'lock-keyhole', 'hand.raised': 'hand', xmark: 'x', 'info.circle': 'info', folder: 'folder',
  'bubble.left.and.bubble.right': 'message-square', 'slider.horizontal.3': 'sliders-horizontal', 'speaker.wave.2': 'volume-2', command: 'command',
  'arrow.up.left.and.arrow.down.right': 'maximize-2', 'xmark.circle.fill': 'circle-x', 'play.circle': 'circle-play', 'record.circle': 'record-circle',
  'lock.open': 'lock-keyhole', sparkles: 'monitor' };
const rgb = (c) => { const n = parseInt(c.slice(1), 16); return [n >> 16, (n >> 8) & 255, n & 255]; };
const masks = new Map(), tinted = new Map();
function mask(name, s) {
  const k = name + s;
  if (masks.has(k)) return masks.get(k);
  const src = ICONS[name][s], a = new Uint8Array(s * s);
  let i = 0, p = 0;
  while (p < src.length) {
    const v = src.charCodeAt(p++) - 97;
    let n = 0;
    while (p < src.length) { const c = src.charCodeAt(p); if (c < 48 || c > 57) break; n = n * 10 + c - 48; p++; }
    n = n || 1;
    a.fill(v * 17, i, i + n); i += n;
  }
  masks.set(k, a);
  return a;
}
export function iconPixels(name, size, color) {
  const n = ICONS[SYMBOLS[name] || name] ? (SYMBOLS[name] || name) : 'monitor';
  const s = SIZES.find((x) => x >= size * 2) || SIZES[SIZES.length - 1];
  const key = `${n}|${s}|${color}`;
  let px = tinted.get(key);
  if (!px) {
    const a = mask(n, s), [r, g, b] = rgb(color), data = new Uint8ClampedArray(s * s * 4);
    for (let i = 0; i < a.length; i++) { const o = i * 4; data[o] = r; data[o + 1] = g; data[o + 2] = b; data[o + 3] = a[i]; }
    px = { width: s, height: s, data };
    tinted.set(key, px);
  }
  return px;
}
export function Icon({ name, size = 15, color = '#ffffff', style }) {
  return <Image pixels={iconPixels(name, size, color)} style={[{ width: size, height: size, flexShrink: 0 }, style]} />;
}
let logo = null;
export function logoPixels() {
  if (logo) return logo;
  const bin = atobBytes(LOGO.b64);
  logo = { width: LOGO.width, height: LOGO.height, data: bin };
  return logo;
}
function atobBytes(b64) {
  const T = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/', map = {};
  for (let i = 0; i < 64; i++) map[T[i]] = i;
  const n = Math.floor(b64.length / 4) * 3 - (b64.endsWith('==') ? 2 : b64.endsWith('=') ? 1 : 0), out = new Uint8ClampedArray(n);
  let o = 0;
  for (let i = 0; i < b64.length; i += 4) {
    const v = (map[b64[i]] << 18) | (map[b64[i + 1]] << 12) | ((map[b64[i + 2]] || 0) << 6) | (map[b64[i + 3]] || 0);
    if (o < n) out[o++] = (v >> 16) & 255;
    if (o < n) out[o++] = (v >> 8) & 255;
    if (o < n) out[o++] = v & 255;
  }
  return out;
}
export function Logo({ size = 96 }) { return <Image pixels={logoPixels()} style={{ width: size, height: size }} />; }
export { View };
