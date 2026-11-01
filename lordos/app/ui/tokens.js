// Every number and color here is go-link's own: pkg/fyneview/theme.go (the main window and Settings,
// which go-link draws the same on macOS and Windows) and Fyne 2.8.1's dark and light themes (the small
// windows: request, session panel, chat, files, call, update, About).
const hex = (v) => '#' + v.toString(16).padStart(6, '0');
export const DARK = {
  bg: hex(0x0e1016), video: hex(0x05060a), surface: hex(0x161a23), surface2: hex(0x1b2030), selected: hex(0x1e2330), border: hex(0x262c3a),
  text: hex(0xe9ecf2), muted: hex(0xa3abbd), faint: hex(0x8c95a8), accent: hex(0xf2a33a), onAccent: hex(0x1a1206), ok: hex(0x7ee2a8),
  danger: hex(0xf4a9b6), tintBG: hex(0x2a1d0c), tintBorder: hex(0x6b4a1c), tintText: hex(0xd9a35a), fieldBorder: hex(0x3a4256),
  holder: hex(0x7f889c), dashed: hex(0x4a5368), dangerFill: hex(0x5a2a33),
};
export const LIGHT = {
  bg: hex(0xf3f4f7), video: hex(0x05060a), surface: hex(0xffffff), surface2: hex(0xeceef3), selected: hex(0xe2e6ee), border: hex(0xd8dce5),
  text: hex(0x151821), muted: hex(0x4b5366), faint: hex(0x6b7386), accent: hex(0xd97e12), onAccent: hex(0x1a1206), ok: hex(0x1d8f55),
  danger: hex(0xc23a4c), tintBG: hex(0xfdf2e1), tintBorder: hex(0xf1c88a), tintText: hex(0xb06a12), fieldBorder: hex(0xc9ced9),
  holder: hex(0x8a91a1), dashed: hex(0xc0c6d2), dangerFill: hex(0xf7d7dc),
};
// Fyne's theme (theme/color.go), with go-link's readable "disabled" text (fyneview/theme.go, readableColor).
export const FYNE_DARK = {
  bg: '#171718', button: '#28292e', fg: '#f3f3f3', low: '#8e8e93', input: '#202023', inputBorder: '#39393a', placeholder: '#b2b2b2',
  separator: '#000000', primary: '#296ff6', onPrimary: '#ffffff', error: '#f44336', onError: '#171718', warning: '#ff9800', success: '#43f436', hover: 'rgba(255,255,255,0.06)',
};
export const FYNE_LIGHT = {
  bg: '#ffffff', button: '#f5f5f5', fg: '#565656', low: '#767676', input: '#f3f3f3', inputBorder: '#e3e3e3', placeholder: '#888888',
  separator: '#e3e3e3', primary: '#296ff6', onPrimary: '#ffffff', error: '#f44336', onError: '#ffffff', warning: '#ff9800', success: '#43f436', hover: 'rgba(0,0,0,0.06)',
};
export const palette = (theme, systemDark = true) => (theme === 'light' || (theme === 'system' && !systemDark) ? LIGHT : DARK);
export const fyne = (theme, systemDark = true) => (theme === 'light' || (theme === 'system' && !systemDark) ? FYNE_LIGHT : FYNE_DARK);
// fyneview/tiles.go
export const TILE = { w: 200, h: 150, row: 56, detail: 38, gap: 14 };
