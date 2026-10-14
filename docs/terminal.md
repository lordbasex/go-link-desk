# go-link-desk-terminal

A remote terminal for a computer without a desktop (a Raspberry Pi, a server, a virtual machine), in the same spirit as Raspberry Pi Connect's remote shell: instead of the screen, each session gets **a shell of its own on a new pseudo-terminal** (`/dev/pts/N`), in the browser. It is not SSH: nothing listens on a port, and the computer only connects out, to signalhub and to the viewer, like the desktop app.

## What it is

- **One small binary, pure Go:** `go build -tags headless ./cmd/terminal` with `CGO_ENABLED=0`, so it needs no libvpx, libopus, X11 or GL, and runs on any Linux (glibc or musl) on x86-64, ARM64 and ARMv7. About 12 MB.
- **The same pieces as the app:** signalhub v1 with `app: "go-link-desk"`, the pairing code, linked browsers with their token and proof, the unattended password (argon2id), WebRTC with TURN as a fallback, the fixed UDP port (`udp_port`, `map_udp_port` in `device.json`) and the idle timeout (`idle_minutes`).
- **Its own identity:** `~/.config/go-link-desk-terminal/device.json`, apart from the desktop app's, so both can run on one computer.
- **Password from a script:** `go-link-desk-terminal password --stdin` reads one line, for a setup script.
- **No picture, sound, clipboard or files:** a session's connection carries the `control` channel and a `terminal` channel only (`StreamConfig.DataOnly`).

## Installing it

As the user whose shell the sessions get:

```sh
curl -fsSL https://desk.go-link.org/install.sh | sh -s -- --terminal
```

It takes the release's package for the system (`go-link-desk-terminal_<version>_<amd64|arm64|armhf>.deb`, `.rpm`, the Arch package) or, with `--user` or without a package manager, the `.tar.gz` (`go-link-desk-terminal-v<version>-linux-<amd64|arm64|armv7>.tar.gz`, whose `install.sh` puts it in `~/.local/bin`), checks it against `SHA256SUMS`, and starts the systemd user service (`/usr/lib/systemd/user/go-link-desk-terminal.service`, or `~/.config/systemd/user/` for the user install) with `loginctl enable-linger`, so it runs from boot without a login. `--uninstall` removes it. The packages depend on nothing; their post-install script only says how to enable the service, since a package cannot know whose shell it should be.

The unit leaves `NoNewPrivileges` and the like out on purpose: they would follow into the sessions' shells and break `sudo` there.

Building them: `make terminal-packages-<amd64|arm64|arm>` (pure Go, no Docker; nFPM for the packages). The release carries all twelve, listed in `SHA256SUMS`. It does not update itself: running the installer again installs the latest release.

## Using it

```sh
go-link-desk-terminal password   # once: the password linked browsers must give
go-link-desk-terminal link       # shows a 9-digit code; asks here before linking
go-link-desk-terminal status     # the service, and the browsers linked
go-link-desk-terminal unlink ID  # forget a browser
go-link-desk-terminal version    # also -v, --version
go-link-desk-terminal help       # also -h, --help: the commands and options
```

Every command, option, service command and the usual problems are in the help page at desk.go-link.org/docs/terminal (`frontend/apps/web/src/i18n/docs-*.ts`).

`go-link-desk-terminal` (or `serve`) runs the service; systemd starts it as the user whose shell the sessions get. The commands talk to it over a Unix socket next to `device.json` (`control.sock`, mode 0600 in a 0700 folder; on Linux the caller's user is checked with `SO_PEERCRED`). The log goes to standard error, so the journal keeps it. Root may run the commands too (`sudo go-link-desk-terminal link`): they find the service of the user who runs it (`--user NAME` when several do).

## Who can come in

A computer without a screen has nobody to accept a viewer, so **only linked browsers come in, with the password**:

1. `go-link-desk-terminal password` sets the unattended password. Until then nobody can come in, and `link` refuses.
2. `go-link-desk-terminal link` shows the pairing code. A browser that types it at desk.go-link.org/web, in the same box as a session code (a 9-digit code that is no session's is tried as a linking code), or opens the `/link/<code>` address it prints, is **asked about in that terminal**: "Ana (Chrome 141 on macOS) wants to be linked… Accept? [y/N]". Yes links it, with its first session at once; no, or closing the command, turns it down. The session code (join) is never asked: such viewers are turned down (`unavailable`).
3. From then on the browser comes back from its list of computers (reach): its token, the computer's proof, then the password, as in the app's unattended access.

A link lets the browser see the terminals and type, copy and paste, send and take files, and save screenshots and recordings (`view`, `control`, `clipboard`, `files`, `record`); links made before 1.5 are upgraded when the service starts.

## The session

The terminals belong to the computer, like screen's: they outlive the viewer, and the next session finds them as they were.

- When a viewer comes and there is no terminal, the host starts one: the user's login shell (from `/etc/passwd`, else `$SHELL`, else `/bin/sh`, run as `-bash` the way `login(1)` does) on a new pseudo-terminal, in its home folder, in a session of its own, with a clean environment: `HOME`, `USER`, `LOGNAME`, `SHELL`, `PATH`, `TERM=xterm-256color`, `COLORTERM=truecolor` and the language (`LANG`, else `C.UTF-8`).
- Up to 16 terminals, each with a number (id) and a name (`shell`, `shell 2`… until the viewer renames it; at most 40 characters).
- The `terminal` channel (ordered, reliable, binary) carries every terminal: each message is a terminal's 4-byte id (big-endian) and then its bytes, the shell's output to the viewer and what the viewer types to the shell (only with `control`).
- On `control`: the host sends `terminal_tabs {tabs: [{id, name, started}], max}` when the viewer comes and on every change (`resumed: true` in the first one when the terminals were there before); the viewer sends `terminal_open {name}`, `terminal_close {id}`, `terminal_rename {id, name}` (with `control`) and `terminal_size {cols, rows}`, which every terminal takes (`SIGWINCH`).
- Each terminal keeps its last 2 MiB of output. A viewer that comes (back) gets the list and then each one's history, before anything new.
- A flood of output (`cat` of a big file) waits while the channel's buffer is over 4 MiB; the shell is not held up, what does not go out stays in its history.
- A shell ending (`exit`) closes its terminal and sends `terminal_exit {id, code}`; the session goes on. `terminal_close` hangs a shell up: `SIGHUP` to its whole session, `SIGKILL` two seconds later to what is left. The session ending (Disconnect, idle, the network) leaves the terminals running; the service stopping hangs them all up.
- One session at a time, as in the app.
- The `welcome` says `features: ["terminal-tabs"]`. A browser keeps the older way (one shell, its bytes alone, `exit` ends the session) for a computer before 1.5.

## In the browser

- The web knows a terminal from its `welcome` (`source: "terminal"`) and shows it where a session shows the screen, with the same toolbar: Keys (Esc, Tab, Ctrl+C…), Screenshot and recording, a **Terminals** menu (the terminals, new, rename, close, copy what is shown, paste), the monitor button as **Screen** with the text size (A+, A−, normal size; it stays open while you adjust) and full screen. Files works as for a desktop (the `files` channel, the same Files panel; a drop on the terminal sends): what is sent lands in `~/Downloads/go-link-desk`, and the computer's side browses its user's home. Chat, sound and the video call are not offered.
- The terminal is xterm.js (`@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl`, MIT), loaded only then. One xterm.js shows the chosen terminal; the page keeps each one's output (2 MiB), so switching is at once.
- A strip at the bottom, like tmux's, lists the terminals: a click shows one, a double click renames it, ＋ opens one, and a dot marks new output in another. Alt+Shift+T opens one and Alt+Shift+← → move between them (the browser keeps Ctrl+Shift+T and Ctrl+Tab).
- Coming back shows "Resumed" when the terminals were open before. If the connection is made again, the page drops what it kept and takes the histories again.
- **Recording:** as video, like any session (the terminal is drawn by WebGL on a canvas, recorded with `captureStream`), or as text, an asciicast v2 file (`.cast`, for asciinema) of what the shown terminal prints. A screenshot is the canvas as PNG. Both ask the computer first, as for a screen.
- Typing goes on the `terminal` channel as UTF-8; the size follows the window (`terminal_size`); the text size is kept in this browser.
- A viewer without `control` watches only, and the page says so.
- **Content Security Policy:** the site allows no inline styles, and xterm.js writes its colors and sizes into `<style>` elements, which the browser refuses. The page copies their rules into a stylesheet built in script (`adoptedStyleSheets`), which the policy allows. The browser's console still notes the refused elements.
- My computers shows a terminal icon for a linked terminal (`terminal` in the saved link).

## Not root

It refuses to run as root, since every session would be a root shell; `--allow-root` is for whoever wants exactly that. Run it as the user whose shell you want, and use `sudo` inside the session.
