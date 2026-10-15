# The Mac app's terminal page

`page.html` is the page the Mac app's session window shows for go-link-desk-terminal (remote_terminal.go fills in the files below). The library files are copied unchanged from the web's npm packages, at the versions in `frontend/apps/web/package.json`:

| File | From |
|---|---|
| `xterm.js`, `xterm.css` | `@xterm/xterm` 6.0.0 (`lib/xterm.js`, `css/xterm.css`), MIT |
| `addon-fit.js` | `@xterm/addon-fit` 0.11.0 (`lib/addon-fit.js`), MIT |

After updating those packages, copy them again from `frontend/node_modules`.
