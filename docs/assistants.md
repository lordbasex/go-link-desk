# AI assistants (MCP)

go-link Desk lets AI assistants (MCP clients: Claude, Claude Code, Cursor, and any client that runs stdio servers) use the computers linked in **My computers**. User-facing guide: <https://desk.go-link.org/docs/assistants>.

## Shape

- **One binary.** `go-link-desk mcp` runs the MCP server over stdio (`internal/mcpserver`, on the official Go SDK). It runs beside the app, never as the host: no instance lock, nothing on standard output but MCP, its log in `~/go-link-desk/logs/mcp.log` (`%LOCALAPPDATA%\go-link-desk\logs` on Windows).
- **Settings › Assistants** (`internal/gui/assistants.go`) turns it on (`assistants.enabled` in `device.json`) and adds `{"command": <the app's executable>, "args": ["mcp"]}` as `mcpServers["go-link-desk"]` to the assistants found (`mcpserver.Sync`): Claude (`claude_desktop_config.json`), Claude Code (`~/.claude.json`, with `"type": "stdio"`) and Cursor (`~/.cursor/mcp.json`). Only installed assistants are touched; the rest of each file and its key order stay; a file is rewritten atomically and only when the entry changes. It runs again at every start, so an app that moved or updated is followed. Off removes the entry.
- **Viewer side.** The server is a viewer like the Mac app's (`internal/viewer`): it reaches a linked computer through `DoorReach` with that link's token, as a viewer named after the assistant (`Claude Code (AI assistant)`), asks only for VP8 (decodable everywhere with libvpx), and keeps the picture hidden until a screenshot needs one.

## Passwords

A linked computer always asks for its unattended password. The assistant never gets it: **Settings › Assistants › Computers they may use** keeps each one in the system's secret store (`pkg/keyring`): the macOS Keychain (Security framework), the Windows Credential Manager (`CredWriteW`), or the Secret Service through `secret-tool` on Linux (the secret goes on its standard input). Service `go-link-desk-assistants`, account the computer's device id. **Stop allowing**, or forgetting the computer in My computers, deletes it. MCP's form elicitation is not used for it: the specification forbids asking for passwords that way.

A wrong saved password is sent once; the computer's own lockout still applies.

## Tools

| Tool | Does | Gate |
|---|---|---|
| `list_computers` | Linked computers, allowed or not, connected or not. | Assistants on |
| `connect` / `disconnect` | Opens / ends the connection (other tools connect by themselves). | A saved password |
| `terminal_run` | Opens the assistant's own terminal (`terminal_open` named `assistant`, once) and types `{ <command>\n}; printf '\n%s_%s %d\n' __gldone <nonce> $?`. The mark, printed joined but typed apart, gives the end and the exit code; the echo is cut. Output is plain text (escape sequences out, `\r` rewrites and backspaces applied). Default 30 s, at most 600; a command still running keeps running. | **Act**; a terminal computer; the link's Control |
| `terminal_send` | Types raw text or keys (`\u0003`). | Same |
| `terminal_read` | A terminal's last output (256 KiB kept per terminal). | A terminal computer |
| `screenshot` | `screenshot` request to the computer (shown there), then the next VP8 keyframe, decoded, scaled to at most 1568 px wide, JPEG. | The link's Record permission |
| `click`, `scroll` | Pointer at a point of the last screenshot (its pixels, scaled to 0..1 of the screen), then the button (left, right, middle; double) or the wheel in lines, through `viewer.Input` like the Mac app's. | **Act**; a desktop; the link's Control |
| `type_text`, `key` | Text packets (any layout), or a key or shortcut (`ctrl+c`, `cmd+space`, `f5`, or a browser key code) pressed in order and let go in reverse. | Same |
| `clipboard_get`, `clipboard_set` | The last text the computer copied (since connecting); or text sent to its clipboard, not pasted. | The link's Clipboard; setting it, **Act** |
| `file_list`, `file_download` | The files channel: a folder of the computer's home, and a file copied into this computer's `~/Downloads` (never over a file; a short UTF-8 text is also returned). The first request waits a second after the link is in: the computer drops what comes before. | The link's Files |
| `file_upload` | A local file (absolute path) to the computer's Downloads, never over a file there. | The link's Files; **Act** |

Settings are read again on every call, so turning assistants or **Act** off counts at once (for the next call).

## Activity log

`assistant-activity.log` next to `device.json`: one JSON line per call (`time`, `assistant`, `tool`, `computer`, `detail` — the command or text, at most 2000 characters — and `result`). Passwords never go there. It starts again past 5 MiB (the old one kept as `.1`).
