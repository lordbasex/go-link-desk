// Builds locales/{en,es,pt}.json from go-link's catalogs (backend-device/internal/gui/i18n*.go).
// Run from lordos/app: node tools/gen-i18n.mjs
import fs from 'node:fs';
import path from 'node:path';
const dir = '../../backend-device/internal/gui';
const out = { es: {}, pt: {} };
const pair = /^\s*("(?:[^"\\]|\\.)*")\s*:\s*("(?:[^"\\]|\\.)*"),?\s*$/;
for (const f of fs.readdirSync(dir).filter((n) => /^i18n.*\.go$/.test(n))) {
  let lang = /_es\.go$/.test(f) ? 'es' : /_pt\.go$/.test(f) ? 'pt' : null;
  for (const line of fs.readFileSync(path.join(dir, f), 'utf8').split('\n')) {
    const open = line.match(/(?:add\(|catalogs\[)"(es|pt)"/);
    if (open) lang = open[1];
    const m = line.match(pair);
    if (m && lang) out[lang][JSON.parse(m[1])] = JSON.parse(m[2]);
  }
}
const keys = new Set([...Object.keys(out.es), ...Object.keys(out.pt)]);
fs.writeFileSync('locales/en.json', JSON.stringify(Object.fromEntries([...keys].sort().map((k) => [k, k])), null, 1) + '\n');
for (const l of ['es', 'pt']) fs.writeFileSync(`locales/${l}.json`, JSON.stringify(out[l], null, 1) + '\n');
console.log('es', Object.keys(out.es).length, 'pt', Object.keys(out.pt).length);
