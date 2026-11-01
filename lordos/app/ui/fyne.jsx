// The small windows' controls, as Fyne 2.8.1 lays them out: text 14, padding 4, inner padding 8,
// radius 5, a 20 px icon in a button. Measured against captures of go-link's real windows.
import { View, Text, TextInput } from '@lordos/sdk';
import { useState } from 'react';
import { Icon } from './icon.jsx';
const WEIGHT = { regular: 400, bold: 600 };
export function FWindow({ f, children, style }) {
  return <View style={[{ flex: 1, backgroundColor: f.bg, padding: 8, gap: 4 }, style]}>{children}</View>;
}
const IMPORTANCE = (f) => ({ low: f.low, warning: f.warning, danger: f.error, success: f.success, medium: f.fg });
export function FLabel({ f, children, bold, importance = 'medium', italic, mono, center, nowrap, style }) {
  return (
    <View style={[{ paddingTop: 8, paddingBottom: 7, paddingHorizontal: 8, minHeight: 33 }, style]}>
      <Text style={{ fontSize: 14, lineHeight: 18, color: IMPORTANCE(f)[importance], fontWeight: bold ? WEIGHT.bold : WEIGHT.regular,
        fontFamily: mono ? 'monospace' : undefined, textAlign: center ? 'center' : 'start', whiteSpace: nowrap ? 'nowrap' : 'normal' }}>{children}</Text>
    </View>
  );
}
export const FSep = ({ f }) => <View style={{ height: 1, backgroundColor: f.separator }} />;
export function FCheck({ f, label, on, onChange, disabled }) {
  return (
    <View onClick={disabled || !onChange ? undefined : () => onChange(!on)} style={{ height: 33, flexDirection: 'row', alignItems: 'center', paddingLeft: 6, gap: 4 }}>
      <View style={{ width: 20, height: 20, alignItems: 'center', justifyContent: 'center' }}>
        <View style={{ width: 16, height: 16, borderRadius: 3, backgroundColor: on ? f.primary : f.input, alignItems: 'center', justifyContent: 'center' }}>
          {on ? <Icon name="fyne-check" size={14} color="#ffffff" /> : null}
        </View>
      </View>
      {label ? <Text style={{ fontSize: 14, lineHeight: 18, color: f.fg, whiteSpace: 'nowrap' }}>{label}</Text> : null}
    </View>
  );
}
export function FButton({ f, label, icon, importance, onClick, disabled, style }) {
  const [hover, setHover] = useState(false);
  const bg = importance === 'high' ? f.primary : importance === 'danger' ? f.error : f.button;
  const fg = disabled ? f.low : importance === 'high' ? f.onPrimary : importance === 'danger' ? f.onError : f.fg;
  return (
    <View onClick={disabled ? undefined : onClick} onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={[{ height: 36, minWidth: 36, paddingHorizontal: 8, borderRadius: 5, flexDirection: 'row', gap: 4, alignItems: 'center', justifyContent: 'center',
        backgroundColor: bg, opacity: hover && !disabled ? 0.9 : 1, flexShrink: 0 }, style]}>
      {icon ? <Icon name={icon} size={20} color={fg} /> : null}
      {label ? <Text style={{ fontSize: 14, fontWeight: 600, color: fg, whiteSpace: 'nowrap' }}>{label}</Text> : null}
    </View>
  );
}
export function FEntry({ f, value, onChange, placeholder, onSubmit, secure, style }) {
  return (
    <View style={[{ height: 36, borderRadius: 5, backgroundColor: f.input, borderWidth: 1, borderColor: f.inputBorder, justifyContent: 'center' }, style]}>
      <TextInput value={value} onChange={onChange} placeholder={placeholder} onSubmit={onSubmit} secure={secure} size={14}
        style={{ backgroundColor: 'transparent', borderWidth: 0, height: 34 }} />
    </View>
  );
}
export function FProgress({ f, value }) {
  return (
    <View style={{ height: 33, borderRadius: 5, backgroundColor: f.input, overflow: 'hidden', justifyContent: 'center', alignItems: 'center', flex: 1 }}>
      <View style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: `${Math.round(value * 100)}%`, backgroundColor: f.primary }} />
      <Text style={{ fontSize: 14, color: f.fg }}>{Math.round(value * 100)}%</Text>
    </View>
  );
}
export const Spacer = () => <View style={{ flex: 1 }} />;
