// node tools/harness/build.mjs <LordOS tree> <out.js>: bundles the test theme with LordOS's SDK.
import path from 'node:path'; import { createRequire } from 'node:module';
const [lordos, out] = process.argv.slice(2);
const { build } = createRequire(path.join(lordos, 'package.json'))('esbuild');
await build({ entryPoints: ['tools/harness/theme.jsx'], outfile: out, bundle: true, format: 'iife', target: 'es2020', minify: true, jsx: 'automatic',
  alias: { '@lordos/runtime': path.join(lordos, 'runtime/index.js'), '@lordos/sdk': path.join(lordos, 'sdk/index.js'), react: path.join(lordos, 'node_modules/react'), 'react/jsx-runtime': path.join(lordos, 'node_modules/react/jsx-runtime.js'), 'react-reconciler': path.join(lordos, 'node_modules/react-reconciler') },
  define: { 'process.env.NODE_ENV': '"production"' }, logLevel: 'warning', nodePaths: [path.join(lordos, 'node_modules')] });
console.log('built', out);
