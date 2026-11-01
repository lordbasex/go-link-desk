const z = (n) => String(n).padStart(2, '0');
export const hhmm = (t) => { const d = new Date(t); return `${z(d.getHours())}:${z(d.getMinutes())}`; };
export const ddmm = (t) => { const d = new Date(t); return `${z(d.getDate())}/${z(d.getMonth() + 1)} ${hhmm(t)}`; };                 // "02/01 15:04"
export const ddmmyyyy = (t) => { const d = new Date(t); return `${z(d.getDate())}/${z(d.getMonth() + 1)}/${d.getFullYear()} ${hhmm(t)}`; };
export const ymd = (t) => { const d = new Date(t); return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}`; };
export const formatCode = (s) => { const d = String(s).replace(/\D/g, '').slice(0, 9); return d.replace(/(\d{3})(?=\d)/g, '$1 '); };
export const digits = (s) => String(s).replace(/\D/g, '');
export const osName = (os) => ({ darwin: 'macOS', windows: 'Windows', linux: 'Linux' }[os] || os || '');
export const middleTruncate = (s, max) => { if (s.length <= max) return s; const h = (max - 1) >> 1; return s.slice(0, h) + '…' + s.slice(s.length - h); };
export function humanSize(n) { if (n < 1000) return `${n} B`; let div = 1000, e = 0; for (let m = n / 1000; m >= 1000; m /= 1000) { div *= 1000; e++; } return `${(n / div).toFixed(1)} ${'kMGTPE'[e]}B`; }
