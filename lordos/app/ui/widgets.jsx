// The main window's and Settings' controls: pkg/fyneview/widgets.go and prefs.go.
import { View, Text, TextInput } from '@lordos/sdk';
import { createContext, useContext, useState } from 'react';
import { Icon } from './icon.jsx';
export const Pal = createContext(null);
export const usePal = () => useContext(Pal);
export const Txt = ({ s, size = 13, w = 400, color, style, nowrap = true, lines }) => (
  <Text numberOfLines={lines} style={[{ fontSize: size, lineHeight: size * 1.21, fontWeight: w, color, whiteSpace: nowrap ? 'nowrap' : 'normal' }, style]}>{s}</Text>
);
// "button": 30 high, radius 7, a symbol and a title; primary is the brand's orange
export function Btn({ title, symbol, primary, onClick, enabled = true, fill, tint, trailing, size = 13, danger, style }) {
  const p = usePal();
  const [hover, setHover] = useState(false);
  let fg = primary ? p.onAccent : p.text;
  if (!enabled) fg = p.faint;
  let bg = fill !== undefined ? fill : primary && enabled ? p.accent : p.surface2;
  if (danger) { bg = p.dangerFill; fg = p.danger; }
  const glyph = symbol ? <Icon name={symbol} size={15} color={tint || fg} /> : null;
  const label = title ? <Txt s={title} size={size} w={primary ? 600 : 400} color={fg} /> : null;
  return (
    <View onClick={enabled ? onClick : undefined} onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={[{ height: 30, minWidth: 30, paddingHorizontal: title ? 11 : 7.5, borderRadius: 7, backgroundColor: bg, flexDirection: 'row', gap: 6, alignItems: 'center', justifyContent: 'center',
        flexShrink: 0, opacity: hover && enabled ? 0.88 : 1 }, style]}>
      {trailing ? label : glyph}{trailing ? glyph : label}
    </View>
  );
}
export const IconBtn = ({ name, size = 15, color, onClick, onContext }) => (
  <View onClick={(e) => { e.stopPropagation?.(); onClick?.(e); }} style={{ width: size + 6, height: size + 6, alignItems: 'center', justifyContent: 'center' }}><Icon name={name} size={size} color={color} /></View>
);
// the small switch: 32 × 18, knob 14
export function Switch({ on, enabled = true, onChange }) {
  const p = usePal();
  const track = on ? p.accent : p.surface2;
  return (
    <View onClick={enabled ? () => onChange(!on) : undefined} style={{ width: 32, height: 18, borderRadius: 9, backgroundColor: track, opacity: enabled ? 1 : 0.47 }}>
      <View style={{ position: 'absolute', top: 2, left: on ? 16 : 2, width: 14, height: 14, borderRadius: 7, backgroundColor: '#ffffff' }} />
    </View>
  );
}
export function Field({ value, onChange, placeholder, onSubmit, width = 220, secure, size = 12 }) {
  const p = usePal();
  return (
    <View style={{ width, height: 22, backgroundColor: p.bg, borderRadius: 2, borderWidth: 1, borderColor: p.fieldBorder, justifyContent: 'center', overflow: 'hidden' }}>
      <TextInput value={value} onChange={onChange} onSubmit={onSubmit} placeholder={placeholder} secure={secure} size={size}
        style={{ backgroundColor: 'transparent', borderWidth: 0, height: 20, paddingHorizontal: 4 }} />
    </View>
  );
}
// popup menus, drawn by the window: Menu host in the root; items [{title, run, checked, disabled}, '-']
export const MenuCtx = createContext({ open() {}, close() {} });
export function MenuLayer({ menu, close }) {
  const p = usePal();
  if (!menu) return null;
  return (
    <View onClick={close} onContextMenu={close} style={{ position: 'absolute', left: 0, top: 0, right: 0, bottom: 0 }}>
      <View style={{ position: 'absolute', left: menu.x, top: menu.y, minWidth: 180, paddingVertical: 4, backgroundColor: p.surface, borderRadius: 5, boxShadow: '0 4px 16px rgba(0,0,0,0.4)', borderWidth: 1, borderColor: p.border }}>
        {menu.items.map((it, i) => (it === '-' ? <View key={i} style={{ height: 1, marginVertical: 4, backgroundColor: p.border }} /> : (
          <MenuRow key={i} it={it} close={close} />
        )))}
      </View>
    </View>
  );
}
function MenuRow({ it, close }) {
  const p = usePal();
  const [hover, setHover] = useState(false);
  return (
    <View onClick={it.disabled ? undefined : (e) => { close(); it.run?.(); }} onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{ height: 30, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 10, gap: 6, backgroundColor: hover && !it.disabled ? p.selected : 'transparent' }}>
      <View style={{ width: 16 }}>{it.checked ? <Icon name="check" size={13} color={p.text} /> : null}</View>
      <Txt s={it.title} size={13} color={it.disabled ? p.faint : p.text} />
    </View>
  );
}
