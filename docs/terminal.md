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
```

`go-link-desk-terminal` (or `serve`) runs the service; systemd starts it as the user whose shell the sessions get. The commands talk to it over a Unix socket next to `device.json` (`control.sock`, mode 0600 in a 0700 folder; on Linux the caller's user is checked with `SO_PEERCRED`). The log goes to standard error, so the journal keeps it.

## Who can come in

A computer without a screen has nobody to accept a viewer, so **only linked browsers come in, with the password**:

1. `go-link-desk-terminal password` sets the unattended password. Until then nobody can come in, and `link` refuses.
2. `go-link-desk-terminal link` shows the pairing code. A browser that types it at desk.go-link.org (Link a computer) is **asked about in that terminal**: "Ana (Chrome 141 on macOS) wants to be linked… Accept? [y/N]". Yes links it, with its first session at once; no, or closing the command, turns it down. The session code (join) is never asked: such viewers are turned down (`unavailable`).
3. From then on the browser comes back from its list of computers (reach): its token, the computer's proof, then the password, as in the app's unattended access.

A link lets the browser see the terminal and type (`view`, `control`).

## The session

- When the session lets the viewer in, the host starts the user's login shell (from `/etc/passwd`, else `$SHELL`, else `/bin/sh`, run as `-bash` the way `login(1)` does) on a new pseudo-terminal, in its home folder, in a session of its own, with a clean environment: `HOME`, `USER`, `LOGNAME`, `SHELL`, `PATH`, `TERM=xterm-256color`, `COLORTERM=truecolor` and the language (`LANG`, else `C.UTF-8`).
- The `terminal` channel (ordered, reliable, binary) carries the shell's output to the viewer and what the viewer types to the shell (only with `control`). The viewer sends its size on `control` (`terminal_size {cols, rows}`) at the start and on every change; the shell gets `SIGWINCH`.
- A flood of output (`cat` of a big file) waits while the channel's buffer is over 4 MiB, so memory stays bounded.
- The shell ending (`exit`) sends `terminal_exit {code}` and ends the session. The session ending (Disconnect, idle, the service stopping) hangs the shell up: `SIGHUP` to its whole session, `SIGKILL` two seconds later to what is left.
- One session at a time, as in the app.

## In the browser

- The web knows a terminal from its `welcome` (`source: "terminal"`) and shows the shell instead of the screen, in xterm.js (`@xterm/xterm`, `@xterm/addon-fit`, MIT), loaded only then (its own chunk, about 85 KB compressed). It keeps the shell's first output until the terminal is on the page.
- Typing goes on the `terminal` channel as UTF-8; the size follows the window (`terminal_size`); A− and A+ change the text size (kept in this browser), and full screen works as for a screen.
- A viewer without `control` watches only, and the page says so.
- **Content Security Policy:** the site allows no inline styles, and xterm.js writes its colors and sizes into `<style>` elements, which the browser refuses. The page copies their rules into a stylesheet built in script (`adoptedStyleSheets`), which the policy allows. The browser's console still notes the refused elements.
- My computers shows a terminal icon for a linked terminal (`terminal` in the saved link).

## Not root

It refuses to run as root, since every session would be a root shell; `--allow-root` is for whoever wants exactly that. Run it as the user whose shell you want, and use `sudo` inside the session.
