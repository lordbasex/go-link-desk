# go-link Desk demonstration host

`desk.go-link.org/demo` shows the code of a computer that is always on: anyone (and the App Store and Google Play reviewers) can try go-link Desk with it. It shows the test card (a clock, color bars, a tone) and lets every viewer in without asking, with every feature, none of them reaching the computer's own things: the mouse and keyboard are only logged, the clipboard is a file, files come and go in a folder of their own (`~/go-link-desk-demo/files`, whose `Received` is emptied every few minutes; 25 MB at most), and linking for unattended access uses a password the page shows.

The session code changes whenever the host reconnects to signalhub (the server makes a new one; a host cannot choose it), and the pairing code every few minutes and once used. The host writes the current ones, with the password, to a file (`--test-code-file`), and systemd uploads that file as `desk.go-link.org/demo.json` when it changes, which the page reads every 15 seconds.

## Pieces

| File | What |
|---|---|
| `go-link-desk-demo.service` | The host, headless, with its own settings folder (`~/.config/go-link-desk-demo`): it runs beside a normal go-link Desk |
| `go-link-desk-demo-publish.path` | Watches `demo.json` |
| `go-link-desk-demo-publish.service` | Runs `publish.py` |
| `go-link-desk-demo-publish.timer` | Runs it every minute too, in case a change slipped past the path unit |
| `publish.py` | PUTs the file to the bucket (Python's standard library, Signature V4) |
| `go-link-desk-demo-clean.timer`, `.service` | Every 5 minutes, deletes what viewers sent more than 10 minutes ago |
| `welcome.txt` | A file in the demo's folder, for viewers to download |

The AWS key is the IAM user `go-link-desk-demo`, whose only right is `s3:PutObject` on `arn:aws:s3:::desk.go-link.org/demo.json`. It lives in `~/.config/go-link-desk-demo/aws.env` (mode 600) on that computer only:

```sh
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

`scripts/deploy-web.sh` leaves `demo.json` in the bucket.

The unattended password is in `~/.config/go-link-desk-demo/demo.env` (`DEMO_PASSWORD=...`, 8 to 128 characters). It is public: the page shows it.

## Install (Linux, as the user that runs it)

```sh
mkdir -p ~/go-link-desk-demo/files ~/.config/go-link-desk-demo ~/.config/systemd/user
tar -xzf go-link-desk-<version>-linux-amd64.tar.gz --strip-components=1 -C ~/go-link-desk-demo   # the release's binary
cp publish.py ~/go-link-desk-demo/
cp welcome.txt ~/go-link-desk-demo/files/
echo 'DEMO_PASSWORD=...' > ~/.config/go-link-desk-demo/demo.env
cp go-link-desk-demo*.service go-link-desk-demo-publish.path go-link-desk-demo-publish.timer go-link-desk-demo-clean.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now go-link-desk-demo.service go-link-desk-demo-publish.path go-link-desk-demo-publish.timer go-link-desk-demo-clean.timer
loginctl enable-linger "$USER"   # keeps it running with nobody logged in
```

`journalctl --user -u go-link-desk-demo` shows the host's log (and every viewer's input); `systemctl --user stop go-link-desk-demo` takes the demo down, and the page says it is reconnecting.
