# Changelog

All notable changes to go-link Desk. Newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **go-link-desk-terminal, a remote terminal for a Raspberry Pi or a server without a desktop** instead of the screen, each session gets a shell of its own on a new pseudo-terminal, as the service's user (never root unless `--allow-root`). One pure-Go binary for Linux x86-64, ARM64 and ARMv7, with signalhub, the codes, the relay and unattended access of the app, and its own `device.json`. Only linked browsers come in, with the password: `go-link-desk-terminal password`, then `go-link-desk-terminal link`, which shows the pairing code and asks in that terminal before linking. `status` and `unlink` too. See [docs/terminal.md](docs/terminal.md). In the browser, the session shows the shell in xterm.js instead of the screen, loaded only for a terminal: typing, the size following the window, A−/A+, full screen, watch-only without control, and a terminal icon in My computers. Install it with `curl -fsSL https://desk.go-link.org/install.sh | sh -s -- --terminal` (the `.deb`, `.rpm`, Arch package or `.tar.gz` for x86-64, ARM64 and ARMv7, checked against `SHA256SUMS`), which starts its systemd user service and keeps it running from boot; a help page at desk.go-link.org/docs/terminal. New dependencies: `github.com/creack/pty` and `@xterm/xterm`, `@xterm/addon-fit` (MIT).
- **Screenshots and recordings of the session, in the web viewer** (also Windows' and Linux' Connect, which open it): a new menu in the session's toolbar takes a screenshot (PNG, with the host's pointer) or records the screen and, when the viewer may hear it, the computer's sound (WebM in Chrome, Edge and Firefox, MP4 in Safari), into a file chosen at the start (Chrome, Edge) or downloaded at the end; a red pill with the time stops it. Everything happens in the browser, from the picture it already gets. A recording stops by itself after 2 hours, or at 1 GB when the browser keeps it in memory.
- **Screenshots and recordings in the Mac app's sessions too:** a **●** button in the session's toolbar takes a screenshot (PNG, as the window shows it) or records the session into an MPEG-4 file (H.264 and AAC, encoded by the Mac, with the sound when you may hear it and the computer's pointer), both into Downloads › go-link Desk, with the same permission and the computer told first; no new permission on the Mac. A recording stops after 2 hours, when the computer stops it or the session ends.
- **Screenshots and recordings in the iPhone, iPad and Android apps** (in their next versions): a **●** button in the session's toolbar, with the same permission and the computer told first. The pictures and the sound the app already gets are encoded by the device (H.264 and AAC, MPEG-4), with the computer's pointer; iPhone and iPad save to Photos (asking once to add, `NSPhotoLibraryAddUsageDescription`), Android to Pictures and Movies › go-link Desk (MediaStore, no permission). A recording stops after 2 hours, when the computer stops it or the session ends.
- **A new permission, Take screenshots and record the screen**, in the connection request, the session panel and each linked device, never ticked by default (the request still ticks the rest). The viewer asks the computer before each screenshot and recording, and saves or starts only after it answered yes, which it does once the person there is shown it: the session panel says **● Recording your screen since 15:04** in red (or the screenshots taken), and so do the main window and the tray. Unticking it stops a recording. Older computers never give it, so the menu does not show. See [docs/security.md](docs/security.md#screenshots-and-recordings).

### Changed

- **The host's code builds without cgo** (`CGO_ENABLED=0`, Linux x86-64, ARM64 and ARMv7): libvpx and libopus answer "not in this build" there, so a program that sends no picture or sound (the coming go-link-desk-terminal, for a Raspberry Pi or a server without a desktop) is pure Go and runs on any Linux. CI vets, tests and builds it that way.

## [1.3.4] - 2026-10-14

A direct connection through a router you set up: with a fixed UDP port, the computer app opens it in the router by itself (UPnP or NAT-PMP, off by default) or tells you the exact rule to add, and a port forwarded by hand finally works.

### Added

- **The computer app opens its fixed UDP port in the router by itself** (macOS, Windows, Linux): a new **Open it in the router by itself** box in Settings › Direct connection (advanced), off by default. With a UDP port chosen, it asks the router with NAT-PMP, then UPnP, for this computer only, keeps the lease (an hour, asked again at half), and removes it on quitting; Settings says what the router answered. See [docs/network.md](docs/network.md#one-port-for-every-connection).
- **Settings › Direct connection (advanced) on Windows and Linux**, as on the Mac: the UDP port, opening it in the router, and the announced addresses (before, only in `device.json`).
- **The connection test gives the exact rules for the fixed port**: the app sends it (and whether the router opened it) with the rest, and the guide shows `UDP 40000 to: 172.17.0.5` for each router instead of the generic steps.

### Fixed

- **A port forwarded to the fixed UDP port was never tried:** pion gathers the public address (srflx) on other sockets, with random ports, so viewers got the router's public address with the wrong port. The app now also announces the public address STUN saw, and the router's while it forwards the port, on the fixed port.
- **The connection test's guide** promised a fixed port "coming soon" (it was already there) and said UPnP alone lets go-link Desk open its port: now it says how.

## [1.3.3] - 2026-10-13

Does your network go direct or through the relay? A connection test at desk.go-link.org/network, like a speed test, finds out and explains double NAT, CGNAT and blocked UDP, with the steps to fix each; Test My Connection… in the computer app sees the routers a browser cannot.

### Added

- **A connection test at desk.go-link.org/network** (also `/red` and `/rede`), in English, Spanish and Portuguese: does this network go direct or through the relay? Like a speed test, it draws the path from the device through its routers to go-link Desk's servers, with packets moving at each step, a gauge that lands on **Direct**, **Depends** or **Relay**, and each step live: the signaling server's round trip, UDP, the public address, whether the router keeps the port toward two STUN servers (symmetric NAT when it does not), and the relay's round trip and transport (two connections in the page, both forced through the TURN of `hello`; a few bytes, no bandwidth test). It explains that the relay is not a fault, with guides for two routers in cascade (bridge mode, DMZ, UPnP), one router, the provider's CGNAT (a public address, IPv6) and networks that block UDP, and a map of the Americas (Natural Earth) with the relays: Virginia, and São Paulo on the way, with an invitation to offer one. The apps will open it with the local address and the router's after `#d=` (never sent to a server) to find routers in cascade; the session's (i) panel links to it when a session goes through the relay. Everything runs in the browser and nothing is stored (`docs/network.md`).
- **Test My Connection… in the computer app** (macOS, Windows, Linux): in the Mac's app menu, the tray's menu on every system and the About window off the Mac. It finds what a browser cannot (`pkg/netprobe`): this computer's local address, its router (the default gateway), the router's own outside address over NAT-PMP or UPnP IGD (both only read; nothing is opened on the router) and the first three hops toward the signaling server, from the system's own traceroute (`traceroute`, `tracepath` or `tracert`, no administrator rights), since many routers answer neither UPnP nor NAT-PMP. Then it opens desk.go-link.org/network with them after `#d=`, never sent to a server, where the test shows the routers in cascade and the provider's modem. The Mac asks once to let it reach the local network.
- **Test my connection in the iPhone, iPad and Android apps**, in Settings: it opens desk.go-link.org/network with this device's address on its network and whether it is on Wi-Fi, mobile data (almost always the provider's CGNAT) or Ethernet, and on Android its router too, after `#d=` (never sent to a server).

## [1.3.2] - 2026-10-13

Try go-link Desk without a second computer: a Try the demo button in every app (Mac, Windows, Linux, iPhone, iPad, Android and the web) connects to a locked-down demonstration computer, whose codes are always at desk.go-link.org/demo.

### Added

- **Try the demo**, everywhere: a button next to Connect (the Mac app, Windows and Linux), under it (iPhone, iPad, Android) and on the web app's Home connects to go-link Desk's demonstration computer with its current code (desk.go-link.org/demo.json), for anyone without another computer at hand and for app reviewers; when it is not there the apps say so (the computer apps open its page). desk.go-link.org/demo has two columns on a wide screen and one on a phone, and says what the demo is now: a small desktop with Minesweeper.
- **A demonstration computer at desk.go-link.org/demo**, for anyone to try go-link Desk and for the App Store and Google Play reviewers: a locked-down computer that is always on, with a small desktop and Minesweeper, and lets every viewer in without asking, with every feature and none of the computer's own things: the clipboard a file, files in a folder of their own (emptied every few minutes, 25 MB at most), and linking for unattended access with a password the page shows. Its codes change (the session code whenever it reconnects to signalhub, the pairing code every few minutes), so the page always shows the current ones (read every 15 seconds), with a button to connect in the browser.
  - **The host:** `--test-code-file` keeps the session and pairing codes, the link and the password in a JSON file as they change; it needs `--test-input-log`, `--test-clipboard-file`, `--test-files-folder` (the only folder viewers browse, download from and send to, with files of 25 MB at most) and either `--test-demo-screen` or `--source testpattern` with `--test-input-log`.
  - **A computer of its own, locked down** (`deploy/demo/vm`, a Proxmox VM): it shares its real screen (`--test-demo-screen`), a small desktop with Minesweeper, a calculator and a text editor; its user has no password, sudo, network, browser or terminal, its files live in memory and the computer restarts every night; the Proxmox firewall lets it reach the signaling server and the relay only (the host carries every session through the relay, `--test-relay-only`), not the local network or the rest of the internet; it keeps no credential (the Proxmox host reads its codes with a key that may only read them, and publishes them).
  - **Running it:** `deploy/demo` has the systemd units and `publish.py`, which uploads the file as `demo.json` with an AWS key that may write that one object only; `scripts/deploy-web.sh` leaves it in the bucket.

## [1.3.1] - 2026-10-13

Windows encodes H.264 on the graphics card (AMD, Intel, NVIDIA) and its sound no longer crackles; on every system the picture no longer drops to 640×360 for good after a quiet scene.

### Added

- **Windows encodes H.264 on the graphics card** (AMD, Intel or NVIDIA), found by itself: it used Windows' own encoder on the processor only, as the encoders of the graphics cards' drivers are asynchronous transforms, which it did not look for.
  - **Picking it:** the host asks Media Foundation for the hardware H.264 encoders first (the drivers register them), and the first that takes the configuration is tried once with a frame, which must come back as a keyframe with its SPS and PPS. Otherwise (no graphics card encoder, or one that fails the frame) Windows' own as before, and VP8 where there is neither.
  - **How it runs:** it takes NV12 (converted from the capture's I420) and says by events when it wants a frame and has one encoded; a thread of its own reads them, and each frame's output is waited for so it leaves at once. Low latency, constant bitrate, no B-frames, keyframes when asked, the bitrate changed as it runs, as before.
  - **If it stops** during a session (a driver reset, the card taken away), Windows' own takes over from a keyframe and the session goes on; the card's is not used again until go-link Desk restarts.
  - **The log** (`video encoder`) names the encoder (for example `AMDh264Encoder`) and says whether it is hardware.

### Fixed

- **Windows: the computer's sound crackled and played slow** while something played. The reader kept the sound's pace with GetTickCount and Sleep, which move by 15.6 ms steps, and filled a packet that seemed late (they come every 10 ms) with silence: short gaps in the sound, and more of it than there was, so the viewer played it slower and later. Now times come from the performance counter, naps from a high-resolution timer, and silence is added only when the stream has given nothing for three device periods (at least 30 ms), which is when nothing plays.
- **The picture no longer drops to 640×360 for good on a fast network.** Seen on a LAN between a Windows PC and a Mac: 1080p at 6 Mbps, then 640×360 at 300 kbps within 11 seconds, without a packet lost, and it never came back. Google Congestion Control lowers its estimate to 85% of what it received when the delay grows, and raises it to at most 1.5 times that: a burst of delay on Wi-Fi during a quiet scene (the encoder sending far less than the network carries) brought it down to what that scene needed, the picture shrank, the encoder sent even less, and the estimate could not climb. Every host (macOS, Windows, Linux) now counts a drop only when the encoder was filling the estimate and the figure comes from that traffic, or the viewer reports loss; and an estimate held under 4 Mbps without loss is tried half again higher every 30 seconds, as browsers probe, so a network that cannot carry it says so.

## [1.3.0] - 2026-10-13

go-link Desk updates itself: it says when there is a new version and installs it, on macOS, Windows and Linux. Also the Mac app shows computers that send VP8, and Linux encodes H.264 on Intel and AMD GPUs.

### Added

- **The app updates itself** (macOS, Windows and Linux):
  - **It says so:** it looks for a new version when it opens and every 6 hours, and a small window says "go-link Desk 1.3.1 is available" with **What's new**, **Later** and **Update**. Once per version, and never in the middle of a session: then when it ends. The tray's menu reads "Update to 1.3.1…" meanwhile.
  - **Check for Updates…** in the Mac's app menu, the tray's menu on every system and the About window on Windows and Linux; it also says when there is nothing new.
  - **Update** downloads the file for this computer with a progress bar, checks it, installs it and opens the new version: the Mac's app in place (same developer's signature required), Windows' installer silently (`/S /UPDATE`, no administrator), and on Linux the user's own copy (`install.sh`) or the package through the desktop's password prompt (`pkexec`). A copy that cannot replace itself (the Windows .zip, a development build) opens the release's page.
  - **Signed:** each release's `SHA256SUMS` carries an Ed25519 signature by an update key that only the release computer holds (`make release-sums`); the app installs nothing else. A file that does not match is thrown away.
  - Pre-releases are never offered.
  - The update reaches copies from 1.3.0 on: earlier ones are updated by hand once more.
- **The Mac app shows computers that send VP8:** a Linux computer without an H.264 encoder on its GPU (and every virtual machine), or a Windows one without Media Foundation. It said "This computer sends its picture in a format this app cannot show yet" and pointed to the website. The app decodes VP8 with libvpx, which it already carried for its own screen, and draws it in the same layer as H.264; a frame that built on a lost one is not drawn, and a whole picture is asked for. Its knock now lists VP8 among the codecs it decodes, after H.264: the computer still sends H.264 when it has an encoder for it, and VP8 otherwise.
- **H.264 on the GPU on Linux** (VA-API: Intel's and AMD's drivers), as macOS has with VideoToolbox.
  - **How it encodes:** P frames only, one reference, picture order type 2 (a browser shows each frame as it decodes it), constant bitrate.
  - **Bitrate:** changed as the encoder runs. The rate control is told the rate frames actually come at, since a desktop sends one only when it changes.
  - **Loading:** libva (MIT) is loaded when a session uses it, so the program also runs where it is not installed. The packages recommend it (`libva-drm2`; `libva-drm.so.2` on RPM).
  - **Picking the GPU:** the first render node (`/dev/dri/renderD128`…) whose driver encodes the profile, on the full encoder or the low-power one.
  - **When it opens:** the driver writes the SPS and PPS itself. So H.264 is offered only after a test frame shows it does; otherwise (no GPU, a virtual machine, NVIDIA) sessions use VP8 as before.
  - **Tests:** a test checks the IDR with its SPS and PPS, the P frames, an asked-for keyframe and a bitrate change at 720p, 1366×768 and 1080p, where a GPU is present. Elsewhere a test checks that it says why it did not open. Not tried on an Intel or AMD GPU yet: where it does not open, sessions keep using VP8.

## [1.2.0] - 2026-10-13

go-link Desk for Windows: a Windows 10 or 11 PC shares its screen as a Mac and Linux do, with an installer that needs no administrator. Also the Mac app's window on Windows and Linux, sharper VP8, the measured system requirements, and fixes.

### Added

- **System requirements in the docs** (`/docs/requirements`, in English, Spanish and Portuguese), measured rather than guessed:
  - **What it covers:** the systems; how many frames a second each processor encodes a busy screen at 1280×800, 1080p, 1440p and 4K; the minimum and recommended hardware; the network each preset uses and how the picture adapts; the viewer; virtual machines.
  - **On the home page:** two new questions, "What computer do I need?" and "How much internet does it use?".
- **`build/vbench`** measures that on any computer: a terminal's text scrolling without end, encoded with the host's VP8 and the system's H.264 at each size. Pin it to fewer cores (`taskset -c 0`) to see a smaller computer.
- **The Windows build** (phase 7): `make device-windows-amd64` (also `-arm64`, and `-amd64-headless` for a console program) cross-compiles the host for Windows 10 and 11 in Docker with llvm-mingw, libvpx and libopus built from source and linked in with the C runtime: one `go-link-desk.exe` that needs no DLL beyond Windows' own. It carries the app's icon, its version (Explorer's Properties) and a manifest: an ordinary user's program (no administrator rights) that sees every monitor in real pixels (per-monitor DPI awareness). `scripts/check-deps.sh windows` reads the .exe's import table on any system, and CI builds it on every push (`deps-windows`) and runs the host's tests on Windows (`device-windows`, with MSYS2's compiler and libraries). Two tests take Windows' ways: the telemetry database's Unix mode is not checked there (the profile folder's access list keeps it private), and the file browser's symbolic link cases run only where Windows lets the user make links. The host runs on Windows with its window, code and connection; sessions show the test card until screen capture arrives.
- **Windows shares its screen** (phase 7): the Desktop Duplication API reads the monitor on its GPU when the desktop changes, and GDI where that cannot run (a remote desktop session, a rotated monitor, a driver without it). Every monitor, named as Windows names it, in real pixels (the process is per-monitor DPI aware); the pointer drawn in when the viewer wants it; the last picture stays while Windows shows the lock screen or a UAC prompt, which no ordinary program may read. Sessions use VP8. Tested on Windows 11 25H2 (VMware): the desktop at 1554×1002 in Chrome on a Mac.
- **The viewer's mouse and keyboard on Windows** (SendInput): the pointer on any monitor (absolute coordinates on the virtual screen, rounded to land on the pixel meant), the five buttons, the wheel both ways in fractions of a notch, keys by position (the scan codes of `protocol/keycodes.json`, so the computer's layout makes the characters; Pause and Print Screen by their virtual keys), typed text as Unicode characters whatever the layout, Ctrl+V to paste, and Windows+L when a session ends with "lock the screen". Windows lets no ordinary program drive a window running as administrator, nor the secure desktop.
- **Windows: the rest of a session** — the computer's sound (a WASAPI loopback stream of the default output, converted by Windows to 48 kHz stereo, with silence while nothing plays), the pointer for the viewer to draw (its position and image, color, monochrome and the text cursor that inverts what is under it, drawn black), the clipboard both ways (Unicode text), keeping the display and the computer awake during a session (a power request, listed by `powercfg /requests`), and starting with Windows (a value under the user's Run key, which Settings › Apps › Startup shows).
- **H.264 on Windows** through Media Foundation: Windows' own encoder (software on every Windows 10 and 11), low latency, constant bitrate, no B-frames, keyframes with their SPS and PPS; the capture's I420 frames go in with their times. mfplat.dll is loaded when the first encoder opens, so the program also starts on the N editions without the Media Feature Pack, where sessions use VP8 (the encoder is tried once before H.264 is offered).
- **The Windows release** (`make windows-package-amd64`, also `-arm64`): `go-link-desk-<version>-windows-<arch>-setup.exe`, an installer (NSIS, zlib license, built in Docker) that needs no administrator: it installs for the user in `%LOCALAPPDATA%\Programs\go-link Desk`, adds a Start menu entry and an entry in Settings › Apps with its uninstaller (which also removes "start with Windows" and keeps the settings), in English, Spanish and Portuguese; and the same program portable in `go-link-desk-<version>-windows-<arch>.zip`. Neither is signed yet: Windows' SmartScreen warns before the first run.
- **Opening go-link Desk again brings its window forward** on Windows and Linux (the Start menu or the desktop's launcher, a double click), instead of a second copy quitting silently while the first one sits in the tray: the second one wakes the running one (`instancelock.Wake`), through a named event of the user's session on Windows and a Unix socket next to the lock (`desk.lock.wake`, in the user's own settings folder) on Linux.
- **A dead link to signalhub is noticed in seconds**, not at the server's next ping: the client pings every 15 s and gives the link up after 35 s without an answer (it waited 75 s). Found on a network that dropped connections without a word (an IP address conflict): the host said "Connected" and showed a code the server no longer had, until the next reconnection.
- The captured pictures and their scaler are shared by Linux and Windows (`pkg/capture/frame.c`); it now reads RGBx pictures (some Wayland desktops' PipeWire streams) as RGB, not BGR.
- On Windows the log goes to `%LOCALAPPDATA%\go-link-desk\logs\desk.log` (when not started from a terminal).

- The website asks before counting visits: a cookie notice (English, Spanish and Portuguese) on the home page, the docs and the privacy page. Only if the visitor accepts does Google Analytics 4 load there; "Cookie settings" in the footer asks again, and rejecting deletes its cookies. The app's pages (connecting, sessions, linked computers) never load it, and leaving the public pages after it loaded is a full page load. The privacy page, the security docs and the "no tracking" texts say so.
- The website measures with its own Google Analytics property (`G-JF0SPS21MX`), apart from go-link.org's, and its own cookies (`desk__ga…`, on desk.go-link.org only): a choice on one site never changes the other.

### Changed

- **The website and the README say Windows is available** (English, Spanish and Portuguese): the home page lists Windows among the computers you can share and has a Windows download (the installer or the portable .zip), and the docs' Installation has Windows' steps: the installer for the user, SmartScreen's warning while the installer is not signed, the firewall's question and what Windows does not let any program share (a window running as administrator, UAC prompts, the lock screen).
- **The connection request on GNOME (Wayland) says what comes next:** the desktop then asks too, with "Allow Remote Interaction" off by default, and without it the viewer can only watch. Shown until the desktop remembers a choice that includes the mouse and keyboard.
- **Settings' words:** on Windows the Theme's third choice reads "Use system setting", as Windows' own settings do; the announced address's example reads "For example: 192.168.1.20", so it no longer looks like a value already set.
- **`docs/architecture.md` covers phase 7:** a section on the Windows host (capture, H.264, control, one copy at a time, the window, the release), libyuv and the VP8 tuning in the picture, Fyne's window on Windows and Linux, and the measured hardware.
- **Windows and Linux get the Mac app's window** (`pkg/fyneview`, drawn with Fyne):
  - **The main window:** the address field with Session/Link, Connect and Settings; the "This computer" panel with the code, the link and the permissions; the filters; the linked computers and recent sessions as cards, a list or a detailed list, with their menus (favorite, rename, forget, copy code).
  - **Settings**, with the same pages and rows.
  - **Same look:** the Mac's colors (dark and light), Inter (OFL) with tabular figures, and Lucide icons (ISC).
  - **Not yet:** sessions do not open in the app's own tabs. Connect opens the viewer in the browser.
  - The small code window is gone on both systems.
  - **On Windows, the frame takes the app's colors:** the title bar in the top bar's color, its title in the text's, and the border in the lines'. Windows draws them (DWM), with its own minimize, maximize and close buttons. Windows 11 takes the exact colors; Windows 10 takes only a dark or light frame.
  - **On Linux too, where the desktop lets a window ask:** KDE Plasma draws the title bar in the app's exact colors (a color scheme hint, `_KDE_NET_WM_COLOR_SCHEME`; on Wayland the windows go through XWayland for it), and GNOME up to 43 dark or light (`_GTK_THEME_VARIANT`). GNOME 44 and later draw every title bar in the system's own style (Settings › Appearance).
  - **No margin around the content:** the windows no longer have Fyne's own margin, which showed as a white frame.
  - **Inter is the app's font** on both systems, its other windows included. It is set before anything measures text: Fyne keeps the faces it measured with, so fields had been measured in its own narrower font and their last characters were cut on Windows.
  - **Settings words the icon rows the way each system does:** taskbar and notification area on Windows; dock or taskbar and the system tray on Linux.
  - **The main window opens at 1120×680**, which fits a 1366×768 laptop or a 1280×800 screen with its taskbar.
  - **The other windows look as on the Mac:** the connection request, the session, the call, the chat and the files. They keep Fyne's own look, in the variant of the app's Theme setting (dark by default) rather than the system's: a light Windows drew them white next to the dark main window. On Windows their title bar takes their background's color.
- **Faint text that reads in Fyne's windows** (also on the Mac):
  - **Which text:** labels of low importance, such as the request's note that "the name and the browser are what the other side says", its countdown and the chat's times.
  - **The problem:** Fyne drew them in its disabled color, nearly the background's (#39393a on #171717 in the dark variant).
  - **The fix:** a grey that reads (4.5:1 or more) and still looks secondary.
- **The connection request opens centered on the screen**, as on the Mac. On Windows it opened in the top-left corner.
- **Sharper, steadier VP8** (Linux, and Windows without H.264):
  - **Frames carry their times:** libvpx was told every frame came a sixtieth of a second after the last. A desktop changes far less often, so each frame got a fraction of the bits it had. Now each frame gets the bits of the time since the previous one; a test checks that frames 100 ms apart get about six times more.
  - **No more restarts on bitrate changes:** a change of a tenth or more now applies while the encoder runs, at most once a second. Before, a change of a fifth or more started the encoder again with a keyframe, which made bursts the viewer saw as stalls.
  - **WebRTC's short rate control buffer** (1 s, not libvpx's 6 s), so a busy screen does not send long bursts.
  - **libvpx's screen content mode**, for sharper text and still areas left alone.
  - **Threads no more than the computer's cores**, and six for pictures of 2560×1440 and larger.
  - **A fixed speed (6, WebRTC's)** instead of libvpx's own choice.
    - **The problem:** on a busy screen, libvpx's own choice ran to its fastest settings, which there are slower and blow far past the bitrate.
    - **The measurement** (`build/vbench`, 2 cores of a 1.8 GHz Xeon, text scrolling at 1080p): 22 fps at 45 Mbps (8 asked) before, 45 fps at 9 Mbps now. At 1280×800: 43 fps at 22 Mbps before, 80 fps at 7.7 Mbps now.
    - **Why it mattered:** the overshoot was what the congestion control took for a full network, dropping a LAN session to 640×400.
- **The screen's pictures are scaled and converted to I420 with libyuv** (BSD-3-Clause; Linux and Windows):
  - Its SIMD code does a 1080p frame in about 2 ms. The plain C before took 28–37 ms, which alone held the capture to about 30 frames a second and took a core.
  - Smaller pictures average every pixel under each of their own (a box filter), not up to sixteen samples.
  - Linking: on Linux, Debian stable's static `libyuv.a`; on Windows, built from source in Docker without its JPEG decoder. Both are linked in, so no new runtime libraries are needed (checked by `check-deps.sh`).
  - On Debian and Ubuntu, development builds need `libyuv-dev`.

### Fixed

- **The host could crash when a video call ended** (seen on Linux, where the call's camera uses VP8): the camera's encoder was closed while a frame was still encoding in it, and libvpx's memory freed under it. Frames and the closing now take turns. The host also says the call ended (to the viewer, its window and its log) before stopping its camera and sound, so nobody waits for that. CI keeps the hosts' logs of a failed end-to-end run.
- **Linux: the main window drawn whole when GNOME maximizes it as it opens** (it does so with a window that nearly fills the screen, such as 1120×680 on 1280×800). Fyne took the new size but went on drawing at the old one, cut at the top and with black bars, until the window was resized again; it is now resized by a pixel and back once it shows.

## [1.1.0] - 2026-10-12

go-link Desk for Linux: a Linux desktop shares its screen as a Mac does, on X11 and Wayland. Also the new website, Android ready for Google Play, and fixes.

### Added

- **The Linux release** (`make linux-package-amd64`, also `-arm64`): `go-link-desk-<version>-linux-<arch>.tar.gz` with the host, its menu entry and icons, and `install.sh`, which installs them for the user (no root; `--uninstall` removes them). Needs glibc 2.34 (Ubuntu 22.04, Debian 12, Fedora 35, RHEL 9 or newer). The windows carry the app's ID (`RESOURCE_NAME` on X11, the app ID on Wayland), so the desktop shows the app's icon and name.
- **Linux packages and a one-line installer:** `make linux-packages-<arch>` builds `.deb`, `.rpm` and Arch packages with nFPM (MIT, run with `go run`, not a dependency of the code): the host in `/usr/bin`, its menu entry and icons, and the libraries it needs (PipeWire, PulseAudio and the desktop portal recommended). `curl -fsSL https://desk.go-link.org/install.sh | sh` picks the package for the system (apt, dnf, yum, zypper, pacman), checks it against the release's `SHA256SUMS` and installs it, or installs the .tar.gz for the user without root (`--user`, also when there is no sudo or known package manager); `--uninstall` removes it. Tested in Ubuntu 24.04, Fedora 41, Arch and Debian 12 (user install).
- **Documentation at desk.go-link.org/docs**, in English, Spanish and Portuguese: getting started (introduction, installation for each system, sharing your screen, connecting), the session (in the browser, iPhone, iPad and Android, the Mac app, clipboard and files, chat, sound and video call), unattended access and the settings, Linux on X11 and Wayland, security, troubleshooting and the command line. Search, tabs by system (opening on the visitor's), a table of contents, previous and next; the site's header and footer link to it. Built on go-link's documentation page.
- **The website says Linux is here:** the computer you share is macOS or Linux today (Windows coming), with a Linux download card and footer link, in the three languages.
- **The Linux host** (phase 7): a Linux desktop shares its screen as a Mac does, in an X11 (Xorg) or a Wayland session, with the same window, consent, session panel and settings. Tested on Ubuntu 24.04 with GNOME (Xorg and Wayland) and KDE Plasma 5.27 (X11 and Wayland): the picture, the pointer, clicks and right clicks, keys by position, typed text (ñ, €, accents, ¿), the clipboard both ways, the sound, keeping the display awake and locking the screen when a session ends.
  - **X11:** the screen (XShm, every monitor with XRandR), the pointer (XFixes), the mouse and keyboard (XTest, with characters the layout lacks), the clipboard (CLIPBOARD).
  - **Wayland, through the desktop's remote desktop portal:** the screen as PipeWire streams, the pointer as their metadata, the mouse and keyboard through the portal (one call at a time: the portal handles them on many threads, which shuffled typed text), typed text by the keys of the desktop's layout with their Shift or AltGr, accented letters by their dead key and anything else through IBus's Unicode entry. The desktop asks the person once (GNOME's dialog); its restore token lets later sessions start without asking, unless the mouse and keyboard were not allowed. The clipboard through the data-control protocol on KDE Plasma, Sway and Hyprland, and through XWayland on GNOME. libpipewire is loaded at run time: a system without it still runs the host.
  - **The sound** (what the speakers play, through PulseAudio or PipeWire's pulse service, loaded at run time) and **keeping the display awake** (GNOME's session manager, the screen saver and power services, the Inhibit portal).
  - The viewer is told when the person refuses to share (`share_denied`), shares the screen without the mouse and keyboard (`no_remote_input`), or the Wayland desktop has no portal (`wayland_session`); the website and the Mac app say so in the three languages.
- Keys go to each system as its own key (`keymap.Key.Native`: macOS's virtual key, X11's keysym); the test logs keep macOS's numbers everywhere. Paste on Linux is Ctrl+V.
- **A new desk.go-link.org.** The landing (`/`) says what go-link Desk is, with real screenshots: the four ways to connect (the web, iPhone and iPad, Android, the Mac app) and their state, the shared computer (macOS today; Windows and Linux coming), how it works, what it does, why it is safe, downloads, questions, and a footer with the site map, the scam warning, a live check of the connection server and the language. A code typed in its hero opens the session at once.
- **The web app at `/web`, as the iPhone and iPad app:** Home with the code, My computers (rename, favorite, forget in each row's menu) and the recent sessions, and Settings in a sheet (language, appearance, message sound). `/c` and `/devices` lead there; session and linking links keep working.
- **The web session's toolbar, as the iPhone app's:** a floating capsule over the picture (disconnect, the computer's name, Keys, chat, files, video call, sound, Screen, full screen, hide) whose Keys and Screen buttons open menus (special keys, Type text, Ctrl = Cmd, Game mode; displays, picture, view, Retina, the remote cursor). In full screen nothing covers the picture: the toolbar and the details button come back when the pointer reaches the top, or with a tap on the handle.
- **The website speaks the browser's language:** Spanish or Portuguese when the browser prefers them, English otherwise; the switcher still wins.
- **go-link Desk for Android phones and tablets** (`mobile/android`, Android 11 or newer): the iPhone and iPad app on Android, with the same screens, look and texts (English, Spanish and Portuguese, generated from the iOS app's). A session code, linking with the pairing code, and linked computers opened with the fingerprint, face or screen lock (the token sealed in the keystore) and their unattended password kept with the link; the fingers as a trackpad or by touch, a mouse, a hardware keyboard by position and the on-screen keyboard with its bar of keys; special keys, chat with "is typing" and the message sound, files (downloads in Downloads › go-link Desk), displays, picture, sound, details, the lock and the guide. The portable logic (`core`, Kotlin) is the iOS app's `DeskCore`, tested on the JVM against the same vectors; CI runs it and builds the app.
- `gen-keycodes.mjs` also writes the Android app's keyboard table.
- **Google Play, ready to publish** (`mobile/android/play`): the store texts in English, Spanish and Portuguese, the icon and feature graphics, the data safety and content rating answers, and `scripts/screenshots.py`, which takes the phone and 7" and 10" tablet screenshots on emulators. Checked on tablets in both orientations.
- `desk --test-picture FILE` (with `--source testpattern`): a test host shows a still picture instead of the test card, silently, for screenshots of a desktop.
- The website says Android is coming to Google Play, and the privacy page covers the Android app.

### Fixed

- **The code was nearly invisible in the host's window with a light theme** (Linux, Fyne): it took the dark theme's white at start; it now follows the theme.
- **The connection request could stay behind the other windows** (macOS) when it came while the main thread was busy, as right after a session ended: it was brought to the front only within 200 ms of being asked. Now it is looked for until it is on screen (up to 5 s), and put over the app in front even when that app keeps the focus.

## [1.0.0] - 2026-10-11

The first release: go-link Desk for macOS (13 or newer, Intel and Apple silicon, signed and notarized), for iPhone and iPad (iOS and iPadOS 26 or newer, on the App Store), and the website (desk.go-link.org). Tested on an iPhone 17 Pro Max and an iPad Pro (M4), connecting to a Mac.

### Added (the iPhone and iPad app, phase 6c)

- **go-link Desk for iPhone and iPad** (`mobile/ios`): connect to another computer with its 9-digit code, and see and use it once the person there accepts. Home has the code and the recent computers; Settings has your name, the fingers and your own signaling server. English, Spanish and Portuguese.
- **Fingers as a trackpad** (slide, tap, long press to drag; two fingers to right click and zoom; three fingers to scroll) or by touch; an iPad's trackpad, mouse and keyboard; the on-screen keyboard with a bar of special keys; the computer's shortcuts (Ctrl+Alt+Del, Spotlight, switch apps…); the displays, the picture preset, the sound and the details.
- **Files, as on the Mac app:** browse the computer's home folder, tap a file to download it (into the Files app, On My iPhone or iPad › go-link Desk, with Share), and send a file from the Files app or a photo or video to the computer's Downloads/go-link Desk, with progress and Cancel.
- **Unattended access with Face ID** (Touch ID, Optic ID, or the device's passcode): link your own computers once with their pairing code (the person there accepts once), and open them from My computers without anyone there. Each link's token is in the keychain, only on this device and given back only after Face ID; the computer proves itself before the token goes out, as in the website and the Mac app, and its unattended password is asked when it has one.
- **Unattended access always has a password** (all platforms): it cannot be turned on without one, removing it turns it off, and a configuration without one lets no linked device in. Settings asks for it twice, says when it is set or what is wrong, and shows the change at once (it waited for the field to lose the focus). The iPhone and iPad app keeps it with the link, behind Face ID, and sends it by itself; the website asks for it every time. "Linked browsers" are now "linked devices", and the linking code says how to link from the iPhone and iPad app.
- **"Is typing"** in the chat, both ways and everywhere (the computer's session window, the website, the Mac app and the iPhone and iPad app), with three dots that rise in turn: a new `typing {on}` control message.
- **The message sound:** a short sound when the person at the computer writes, on the website (two soft notes, with a 🔔 to turn it off in the chat) and the iPhone and iPad app (the system's chime; Settings › Message sound).
- **Chat** with the person at the computer, with a count of the messages not read and a short notice when one comes in with the chat closed.
- **How to use it:** a guide to the gestures (trackpad and touch), the keyboard and its keys, the toolbar, an iPad's trackpad and mouse, and the lock; it opens the first time a session starts, and from the toolbar's ? button.
- **Lock, to watch** (a film, a presentation): the screen, an iPad's trackpad and keyboard stop sending and the view stays still; the toolbar becomes a small lock, held for a second to unlock.
- **The same protocol as the website and the Mac app**, with libwebrtc (hardware H.264) and the picture shown as soon as it is decoded. `DeskCore`, the app's portable logic, is tested on the Mac (`make test-core`) and in CI, against the same input vectors as the host and the website.
- **Signed and in TestFlight:** `make testflight` archives, signs with the App Store Connect API key and uploads.
- **The Mac app signed and notarized:** `make device-notarized` signs with the Developer ID, notarizes the app and the disk image, and staples their tickets (`make device-dmg` for the disk image alone).
- **A privacy page** on the website (desk.go-link.org/privacy), linked from the footer.

### Added

- **About go-link Desk:** the version, what the app is, its license and website. On macOS it is the system's About panel, first in the app menu, which is now go-link Desk's own (About, Settings… with ⌘,, Hide, Quit) in the app's language; elsewhere, a small window. Also in the menu bar icon's menu.

### Added (the app as a viewer, phase 6b)

- **go-link Desk connects to other computers too,** in one window: Home has the address bar (a 9-digit session code, or a pairing code to link the computer for unattended access), and each session opens in a tab, or in a window of its own (Settings).
- **The same protocol as the website,** so the other computer needs nothing new: the knock, the consent, the permissions, the link, with the computer's proof before the token, and the unattended password.
- **The picture is H.264 drawn by the system** (decoded on the hardware, no copy through Go), with the computer's pointer, and the mouse, keyboard and wheel while it may be controlled: keys by position, shortcuts to the other computer, Cmd+V pasting this computer's clipboard there.
- **In the session's toolbar:** displays, picture preset (Speed, Balanced, Quality, Retina), special keys (switch apps, Spotlight, force quit, lock screen, screenshot; Ctrl+Alt+Del and the Windows key for other systems), type the clipboard, chat, files (browse and download, send with a button or by dropping files on the picture), sound, details and full screen.
- **Home:** this computer's code in a panel of the brand's color (clicking the code or its link copies it), My computers (the linked ones, with favorites), the recent sessions, as cards with a picture of the screen, a list or a detailed list.
- **Settings, native on macOS, in pages:** General (state, appearance: dark, light or the system's theme, the app icon and the menu bar icon in color or dark; the signaling server and website, for a person's own signalhub; language; open at login), Sessions, Unattended access and Permissions.
- **Direct connection (advanced), in Settings:** the UDP port every connection uses and the addresses announced for it, for a computer behind a second router (a double NAT, where browsers on the first network cannot reach it and go through a relay); it shows this computer's address and which port to forward to it, and applies on opening again.
- **The connection request comes with every box ticked** (mouse and keyboard, sound, clipboard, files; not a request to link a browser, which is for good): whoever asks for support accepts with one click, and unticks what they do not want, there or in the session panel. The session panel says "Can see your screen, and:" above its boxes (it said only "See your screen" whatever was allowed). Settings › Sessions › "A request starts with" (Full control, or Only the screen) chooses it ahead of time.
- **A new icon:** a screen with a pointer, with depth in the Dock (amber, or graphite with amber), flat in the menu bar.
- **`--connect`** opens sessions at start; **`--test-viewer`** runs only the viewer, without a window (end-to-end tests only).
- **End-to-end tests:** the app's viewer joins with a code and gets the picture; links with the pairing code, is refused with a wrong password and comes back with the right one.

### Fixed (phase 6b)

- **The bitrate follows what gets through, not only the estimate:** while the picture is small or still, the congestion control saw no congestion and climbed to 30 Mbps, and the next busy screen was sent at 20 Mbps through a relay that carries 2 (40–50% lost, frames dropped, the size falling to 640 and climbing back every half minute). Each viewer's bitrate is now also capped by what it was actually sent: the cap starts at 4 Mbps, grows 15% a second while the encoder fills it and nothing is lost, and drops at once to what got through when the viewer reports 10% loss or more over enough data (by half at most). Without loss it always comes back to 4 Mbps, also on a still screen; beyond that only while the encoder fills it. The session details show that figure as the estimated bandwidth.
- **The picture's size no longer flaps on a shaky network:** the bandwidth ladder still goes down at once, but climbs back one rung at a time and only 10 seconds after its last change. A browser on an iPad saw 21 size changes in two and a half minutes (each a new stream for its decoder) and closed twice.

### Added (unattended access and my computers, phase 6)

- **Unattended access** (Settings, off by default): a browser linked once with the computer's linking code comes back from My computers without a code and without anyone accepting, also after the computer restarts. Linking asks the person at the computer, and the permissions they tick are the link's.
- **The computer proves itself first, then the browser its token,** over the encrypted connection; until then the browser is connected but gets nothing, not even the computer's name. The host keeps only the token's hash.
- **An optional password** for unattended access, after the link: argon2id, 5 tries per browser, and 20 wrong ones in 10 minutes lock it for 10 minutes.
- **Linked browsers in Settings:** their name, when linked and last used, their permissions (changed there) and Revoke; a revoked browser is turned away, also mid-session.
- **My computers** on the website (`/devices`): the linked computers with a thumbnail of their screen, favorites first, renamed and forgotten here; the recent sessions; `/link` to link a computer.
- **Around a session:** disconnect after 5 minutes to an hour without activity, lock the screen when a session ends, keep the display awake while one is on, and open go-link Desk at login (a LaunchAgent on macOS, an autostart entry on Linux).
- **The host's language** can be picked in Settings.
- **`--test-unattended`** and **`--test-unattended-password`** for the end-to-end tests (only with `--test-auto-accept`, and never saved).
- **End-to-end tests:** a browser linked with the code comes back without it after the host restarts, types the password when one is set, and is turned away once its link is revoked; without unattended access the linking code links nothing.
- **Dependency:** `golang.org/x/crypto` (BSD-3-Clause, already in the build through Pion) is now direct, for argon2id.

### Added (files, phase 5)

- **Send files to the computer** by dropping them anywhere on the remote screen or choosing them: they land in Downloads › go-link Desk, never over a file already there, with progress on both sides and a Stop button.
- **Browse the computer's home folder and download** its files: Chromium writes straight to a file the person picks; other browsers save it when complete.
- **The permission** "Send and receive files", unticked by default, in the request and the session panel; the panel shows the file moving.
- **An audit trail:** every file sent or received is in the host's log.
- **Measured:** 1 GB sent in 63 s (16 MB/s) while the picture kept flowing (`E2E_BIG_FILE_MB=1024`).
- **End-to-end tests:** a big file's content checked by SHA-256 while the picture's frames are timed, a second file with the same name saved beside it, a download from the host's Documents with hidden files left out, and no files without the permission.
- `make check` also type-checks the end-to-end tests.

### Added (video call, phase 4b)

- **A video call between the viewer and the person at the host,** both ways: the viewer asks from the session's toolbar, the person at the host takes it (choosing camera and microphone) or not, and each side can mute, turn the camera off or hang up.
- **On the host:** the camera with AVFoundation, the microphone with the system's echo cancellation (AVAudioEngine voice processing), the viewer's camera decoded by VideoToolbox in a call window, and its voice played. No new dependencies.
- **On the website:** a call panel with the host's camera large and the viewer's own small, using the browser's camera and microphone with echo cancellation.
- **No new negotiation:** a call's media is in every offer and starts with `replaceTrack`.
- **macOS:** the camera and audio-input entitlements and their usage texts in Info.plist.
- **End-to-end test:** a fake camera and microphone in Chromium; the host sees and hears the viewer, the viewer sees the host's test card and hears its tone, and a click on the call's video never reaches the host.

### Added (sound, clipboard and chat, phase 4)

- **The computer's sound** in the browser: ScreenCaptureKit captures it (without the host's own) and Opus carries it at 96 kbps stereo, only to a viewer allowed to hear it. The page asks for a click if the browser blocks autoplay, and has a mute button. The test card plays its 1 kHz tone.
- **The clipboard, both ways, as text:** what is copied on the host lands on the viewer's clipboard (in Safari and Firefox at the next click or key, or with a Copy button), and the viewer's Cmd+V or Ctrl+V pastes what is copied on the viewer's computer into the host. Up to 1 MiB, in 16 KiB pieces.
- **Chat** between the viewer and the person at the computer, in the session panel and in a panel on the website, with an unread count.
- **Permissions** "Hear your computer's sound" and "Share the clipboard (copy and paste)" in the connection request and the session panel, unticked by default.
- **Tablets and phones:** touchpad mode (drag to move the pointer, tap to click, two fingers to right-click and scroll) with the host's pointer drawn, and a Keyboard button for the touchscreen keyboard, whose typing reaches the host.
- **Session details** float over the picture, as in go-link's player, also in full screen, with new rows (quality, jitter buffer, freezes, sound) and "Show on screen": the key figures in one line over the picture.
- **desk.go-link.org** is published (S3 and CloudFront): `make web-deploy`.
- **`--test-clipboard-file`** keeps the host's clipboard in a file for the end-to-end tests.
- **End-to-end tests:** the tone is heard by a viewer allowed to, silence for one that is not; the clipboard both ways; the chat.

### Added (control, phase 3)

- **Control of the host's mouse and keyboard** from the browser, when the person at the computer allows it: a "Use your mouse and keyboard" checkbox in the connection request, off by default, and a switch in the session panel that gives or takes it back during the session.
- **Mouse:** moves at up to one per frame on an unreliable channel, clicks that carry their own position, drags, double and triple clicks, back and forward buttons, and smooth (pixel) or line scrolling.
- **Keyboard by position:** the host's own layout makes the characters, so shortcuts, dead keys and letters like ñ work as on the host. Key repeat, IME input (Chinese, Japanese) and "Type text…" for pasting text as typing.
- **Nothing stays stuck:** everything held is let go when the page loses the focus, is hidden or leaves full screen, when the viewer loses control, hangs up or its connection stops answering. On macOS browsers, keys pressed with Cmd are let go when Cmd is.
- **On the website:** a Keys menu with the host's special shortcuts (Cmd+Tab, Force Quit, Spotlight and screenshots on a Mac; Ctrl+Alt+Del, Alt+Tab and the Windows key elsewhere), "Ctrl is Cmd" for viewers on Windows or Linux, game mode (pointer lock), the keyboard locked in full screen so the system's shortcuts reach the host, and the host's pointer shape as the browser's cursor. The viewer is told when the host has no Accessibility permission.
- **One list of keys,** `protocol/keycodes.json`, with the Go and TypeScript tables generated from it, and shared test vectors for the binary packets.
- **`--test-input-log`** writes viewers' input to the log instead of injecting it, for the end-to-end tests; it needs `--test-auto-accept`.
- **End-to-end tests** of control: a click, keys, Ctrl+C, the wheel, typed text and the release when the page loses the focus, and a watch-only viewer that gets no keyboard.

### Added (screen, phase 2)

- **The real screen on macOS:** ScreenCaptureKit hands each changed frame to the system's H.264 encoder (VideoToolbox, on the hardware) without a copy, at up to 60 frames per second. Nothing is captured while nobody watches, and a still screen sends nothing but a few sharpening frames.
- **Presets** Speed, Balanced and Quality, all up to 60 frames per second, and **Original resolution (Retina)**. The picture follows the viewer's window; Quality sends the display's own pixels.
- **Several displays:** the viewer picks one and the picture switches without reconnecting.
- **The pointer** travels as its shape and position and the browser draws it, sharp at any size; or, as an option, it is drawn into the picture.
- **Bandwidth estimation** (Google Congestion Control, TWCC): the encoder follows what the path carries, and a short bandwidth lowers the frame rate and the size. Lost packets are resent.
- **Latency measured from end to end** with a beacon: on an Intel Mac on the LAN, 30 to 47 ms from the host's screen to the viewer's at 1080p and 58 frames per second.
- **Permissions:** the host's window shows Screen Recording and Accessibility, with a button that asks for them. Without Screen Recording, the viewer sees the test card and is told why.
- **On the website:** display, quality, view (Fit, Original, Stretch), Retina and remote-cursor options; session details with the target bitrate, the estimated bandwidth, resent packets and the screen latency, with a Measure button.
- **H.264 tuning:** Constrained High with the low-latency rate control (5 dB sharper text than Baseline on an M1 for a tenth more bits), falling back without it where the encoder refuses it.
- **Headless hosts on macOS** serve the main thread's event loop, which ScreenCaptureKit needs.
- **Measurement builds** (`GO_TAGS=desk_test_screen`) for testing the real screen without a person to accept; never a release.

### Added (sessions)

- **A session from end to end, with the test card:** the viewer types the code, says its name and knocks; the person at the computer gets a connection request (the declared name and browser, what the viewer will be able to do, 60 seconds to answer, and the first time a warning about phone scams) and accepts or rejects. Only an accepted viewer gets a WebRTC offer: the video (VP8) and a `control` channel with the host's name, round trips, what the host sends and what the browser receives.
- **Always visible on the host:** while a session lasts, the tray icon turns red and gets a Disconnect item, and a small panel floats above every window with who is connected, since when, what they can do and Disconnect.
- **Every way a session ends:** Disconnect on either side, the viewer leaving (signalhub's `peer_left`), the host leaving, or the network. The viewer is told why.
- **One session at a time:** a second viewer gets "busy". A host without a window turns every viewer down (nobody can accept).
- **ICE restart:** when the connection drops, the host offers again with new credentials, so a session survives a network change.
- **Each viewer gets its own track and encoder,** so a keyframe one browser needs costs nobody else anything.
- **On the website,** `/c/<code>` asks for your name (remembered), waits for the answer, and shows the remote screen filling the window (fitted, never cropped, drawn smooth), with session details (latency, path, frames sent, received and decoded, size, bitrate, loss, codec), full screen and Disconnect. In English, Spanish and Portuguese.
- **`--test-auto-accept`** accepts every viewer without asking, for the end-to-end tests only: the host refuses it unless `--source testpattern`.
- **End-to-end tests** with a real Chromium: a viewer gets in and decodes frames, a second one is turned away as busy, leaving ends the session and frees the host, Disconnect ends it, and a host with nobody to ask turns the viewer down.

### Added (host)

- **The host registers with signalhub and shows a session code:** `cmd/desk` registers as `app: "go-link-desk"`, opens an invite-only room and asks for its invitation; the 9-digit code and the `/c/<invite>` link show in a minimal window (Fyne, with a tray icon, in English, Spanish and Portuguese) or, with `--headless`, in the log. "New code" replaces the invitation and the old code stops working. The room and its code are opened again after every reconnect.
- **The generic pieces of go-link's device,** adapted where they named games: the signalhub client with reconnection, pairing and linked browsers (the remembered-browser proof now signs `go-link-desk link proof`, so a proof never passes between the two apps), the PIN gate, status, CPU and RAM metrics, update checks against this repository's releases, the VP8 and Opus encoders and the VideoToolbox H.264 encoder (called directly; no ffmpeg), the VP8 payloader and playout-delay helpers, telemetry storage, and the test card without its gamepad (it shows the frame rate and the remote pointer instead).
- **Flags:** `--config`, `--server-signaling`, `--web-url`, `--headless`, `--debug`, `--version` and `--source screen|testpattern` (the source is used from the next phase on).

### Added (web)

- **desk.go-link.org's first pages:** the landing, `/c` with the code form (the code groups as `915 355 636` while typed; a pasted `/c/<invite>` link works too) and `/c/<code or invite>`, which for now only confirms the code. Strict CSP, self-hosted fonts, light and dark themes, English, Spanish and Portuguese.
- **The shared package** (`@go-link-desk/shared`): go-link's signalhub client, signaling URL helpers, code editing, pure TypeScript SHA-256 and HMAC, linked-browser storage and the design tokens, under go-link-desk names (`APP = "go-link-desk"`, `/c/` links, `go-link-desk.*` storage keys).

### Added (build and CI)

- **`make check`** runs what CI runs: gofmt, `go vet` (with and without the window), `go test -race`, govulncheck, typecheck, Vitest, the web build and gitleaks.
- **A portable macOS app** (`make device-darwin-arm64`, `-amd64`, `-universal`): macOS 13 or newer, libvpx and libopus built from source and linked in, hardened runtime with no entitlement exceptions.
- **`scripts/check-deps.sh`:** fails when a binary needs a library that does not come with the operating system (`otool -L` on macOS, `ldd` on Linux). CI checks the Linux build (Docker, Debian stable) and the macOS build, including the encoders.
- **End-to-end tests** (`make e2e`): a signalhub and a headless host of their own; the code opens the host's invite-only room, a wrong code opens nothing, and the website takes the code.
- **Cross-language test vectors** in `protocol/vectors/`, read by both the Go and the TypeScript tests (the linked-browser proof first).

### Docs

- Architecture, protocol (including the proposed input channels), security and threat model, provenance of every file taken from go-link, and the rules for signalhub proposals.
