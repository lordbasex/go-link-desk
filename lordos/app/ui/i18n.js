// L and Lf as go-link's internal/gui/i18n.go: keyed by the English text, from its own catalogs.
import { locale } from '@lordos/sdk';
import es from '../locales/es.json';
import pt from '../locales/pt.json';
const catalogs = { es, pt };
let current = 'en';
export const LANGUAGES = [['', 'Automatic'], ['en', 'English'], ['es', 'Español'], ['pt', 'Português']];
export function setLanguage(id) {
  let l = id;
  if (!l) { try { l = String(locale.get().lang || 'en').split(/[-_]/)[0]; } catch (e) { l = 'en'; } }
  current = catalogs[l] ? l : 'en';
}
export const language = () => current;
export const L = (s) => catalogs[current]?.[s] ?? s;
export function Lf(format, ...a) {
  let i = 0;
  return L(format).replace(/%%|%[sd]/g, (m) => (m === '%%' ? '%' : String(a[i++])));
}
