# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org).

This is the policy of go-link Desk's Windows files. The same text, in English, Spanish and Portuguese, is at [desk.go-link.org/code-signing](https://desk.go-link.org/code-signing).

## What is signed

- The Windows program, `go-link-desk.exe`, for x86-64 and ARM64. The portable `.zip` carries the same signed program.
- Its installer, `go-link-desk-<version>-windows-<arch>-setup.exe` (NSIS).

Only go-link Desk's own files are signed. The libraries inside the program (libvpx, libopus and libyuv) are built from their source code with it (`backend-device/build/windows.Dockerfile`), and nothing signed by someone else is signed again.

Other platforms are signed in their own ways: the Mac app with Apple's Developer ID (notarized), and every release's `SHA256SUMS` with the project's update key, which the app checks before it installs an update.

## How a signed file is made

1. A release's tag (`v*`) starts [`.github/workflows/windows.yml`](../.github/workflows/windows.yml). Every job runs on GitHub's own runners and builds from that tag's source, with the build scripts in this repository (`make device-windows-<arch>`, `make windows-package-<arch>`).
2. Both programs go to SignPath in one signing request, artifact configuration [`programs.xml`](../backend-device/build/windows/signpath/programs.xml): only a PE file whose product name is `go-link Desk` and whose product version is the tag is signed.
3. An approver checks the release and approves the request by hand in SignPath. SignPath checks that the file comes from that workflow run of this repository (origin verification).
4. The installers are made with the signed programs inside and go to SignPath in a second request ([`installers.xml`](../backend-device/build/windows/signpath/installers.xml)), approved the same way.
5. The workflow's artifact `go-link-desk-<tag>-windows` (installers, `.zip` files and their `.sha256`) goes into the release (`make windows-from-ci VERSION=<tag>`, then `make release-sums`).

Pull requests that change the Windows build run the same workflow without signing.

## Team roles

| Role | Members |
|---|---|
| Committers and reviewers | [Federico Pereira (@lordbasex)](https://github.com/lordbasex) |
| Approvers | [Federico Pereira (@lordbasex)](https://github.com/lordbasex) |

Changes from anyone else come as pull requests and are reviewed before they are merged. Everyone in these roles uses multi-factor authentication on GitHub and SignPath.

## Privacy

go-link Desk sends nothing to other computers unless you use it to: it reaches the signaling server (and, when a direct path is not possible, its relay) to start a session, and every 6 hours it asks GitHub whether a newer version exists, which **Settings › General › Check for updates automatically** turns off. The rest is in the [privacy policy](https://desk.go-link.org/privacy).

## Check a signature

In Windows: right-click the file, **Properties › Digital Signatures**; the signer is SignPath Foundation. In PowerShell:

```powershell
Get-AuthenticodeSignature .\go-link-desk-setup.exe | Format-List
```

## Report a problem

If a signed file does something it should not, write to lord.basex@gmail.com or [open an issue](https://github.com/lordbasex/go-link-desk/issues), with the file's SHA-256 and where you downloaded it.
