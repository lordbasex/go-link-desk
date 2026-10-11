// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/clipboard"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/voice"
)

// thumbEvery is how often a session's picture is kept for My computers.
const thumbEvery = 15 * time.Second

// remote is one session to another computer: the viewer, its window (a
// tab by default), and its chat and files windows.
type remote struct {
	u     *ui
	s     *viewer.Session
	view  *remoteview.View
	input *viewer.Input
	door  viewer.Door
	board clipboard.Board
	stop  context.CancelFunc

	mu       sync.Mutex
	host     viewer.HostInfo
	perms    models.Permissions
	displays []viewer.Display
	current  int
	shape    *viewer.CursorShape
	preset   viewer.Preset
	retina   bool
	sent     viewer.Sent
	hud      bool
	audio    *voice.Engine
	muted    bool
	wasLive  bool
	thumb    []byte
	linked   *viewer.NewLink
	codecOK  bool
	resize   *time.Timer
	files    *viewer.Files
	chat     *remoteChat
	browse   *remoteFiles
	closed   bool
}

// viewerName is the name the other side sees.
func (u *ui) viewerName() string {
	if u.opts.Settings != nil {
		if n := strings.TrimSpace(u.opts.Settings.Get().ViewerName); n != "" {
			return n
		}
	}
	if me, err := user.Current(); err == nil {
		if me.Name != "" {
			return me.Name
		}
		return me.Username
	}
	return ""
}

// agent is what the other side sees in place of a browser.
func (u *ui) agent() string {
	os := map[string]string{"darwin": "macOS", "windows": "Windows", "linux": "Linux"}[runtime.GOOS]
	return strings.TrimSpace(fmt.Sprintf("go-link Desk %s on %s", strings.TrimPrefix(u.opts.Version, "v"), os))
}

// connect opens a session to another computer, in a new tab.
func (u *ui) connect(door viewer.Door) {
	if !remoteview.Supported {
		return
	}
	cfg := models.Config{}
	if u.opts.Settings != nil {
		cfg = u.opts.Settings.Get()
	}
	r := &remote{u: u, door: door, board: clipboard.System(), preset: viewer.PresetBalanced}
	title := L("Connecting…")
	switch {
	case door.Kind == viewer.DoorReach && door.Device != nil:
		title = door.Device.Title()
	case door.Code != "":
		title = services.FormatCode(door.Code)
	}
	tools := []remoteview.Tool{
		{ID: "display", Label: L("Displays"), Symbol: "display"},
		{ID: "quality", Label: L("Picture"), Symbol: "slider.horizontal.3"},
		{ID: "keys", Label: L("Keys"), Symbol: "command"},
		{ID: "chat", Label: L("Chat"), Symbol: "bubble.left.and.bubble.right"},
		{ID: "files", Label: L("Files"), Symbol: "folder"},
		{ID: "sound", Label: L("Sound"), Symbol: "speaker.wave.2"},
		{ID: "info", Label: L("Details"), Symbol: "info.circle"},
		{ID: "fullscreen", Label: L("Full screen"), Symbol: "arrow.up.left.and.arrow.down.right"},
		{ID: "disconnect", Label: L("Disconnect"), Symbol: "xmark.circle.fill"},
	}
	view, err := remoteview.Open(remoteview.Config{Title: title, Tabs: !cfg.SessionWindows, Tools: tools}, r)
	if err != nil {
		u.opts.Logger.Error("remote view", "err", err)
		return
	}
	r.view = view
	signal := u.opts.SignalURL
	if signal == "" {
		signal = cfg.EffectiveSignalURL()
	}
	if door.Kind == viewer.DoorReach && door.Device != nil && door.Device.SignalURL != "" {
		signal = door.Device.SignalURL
	}
	r.s = viewer.New(viewer.Options{
		Door: door, Name: u.viewerName(), Agent: u.agent(), Codecs: []string{"h264-high", "h264"},
		SignalURL: signal, OwnDeviceID: cfg.DeviceID, Logger: u.opts.Logger, Events: r.events(),
	})
	r.input = viewer.NewInput(r.s)
	r.input.Clipboard = r.board.Text
	view.Overlay(L("Connecting…"), false, "", true)
	for _, t := range []string{"display", "quality", "keys", "files", "sound"} {
		view.ToolState(t, false, false)
	}
	u.remotesMu.Lock()
	u.remotes = append(u.remotes, r)
	u.remotesMu.Unlock()
	ctx, cancel := context.WithCancel(u.opts.Ctx)
	r.stop = cancel
	r.s.Dial(ctx)
	go r.thumbs(ctx)
}

func (r *remote) events() viewer.Events {
	return viewer.Events{
		Status:      r.onStatus,
		Host:        r.onHost,
		Permissions: r.onPermissions,
		Displays:    r.onDisplays,
		Sent:        r.onSent,
		Notice:      r.onNotice,
		Video:       r.onVideo,
		Audio:       r.onAudio,
		Cursor:      r.onCursor,
		CursorPos:   func(x, y float64, visible bool) { r.view.CursorPos(x, y, visible) },
		Clipboard:   func(text string) { r.board.SetText(text) },
		Chat:        r.onChat,
		Typing: func(on bool) {
			fyne.Do(func() {
				if r.chat != nil {
					r.chat.showTyping(on)
				}
			})
		},
		Linked:   r.onLinked,
		Unlinked: r.onUnlinked,
		Files:    r.onFiles,
	}
}

// statusText is what the window says about a status ("" when live).
func statusText(st viewer.Status) string {
	switch st.Kind {
	case viewer.StatusJoining, viewer.StatusConnecting:
		return L("Connecting…")
	case viewer.StatusWaiting:
		return L("Waiting for the person at the computer to accept…")
	case viewer.StatusPassword:
		switch {
		case st.Checking:
			return L("Checking the password…")
		case st.PasswordError == "wrong":
			return Lf("Wrong password. %d tries left.", st.Left)
		}
		return L("This computer asks for its unattended access password.")
	case viewer.StatusReconnecting:
		return L("The connection dropped. Reconnecting…")
	case viewer.StatusLive:
		return ""
	case viewer.StatusRejected:
		switch st.Reason {
		case models.RejectTimeout:
			return L("Nobody answered at the computer.")
		case models.RejectBusy:
			return L("The computer is in another session.")
		case models.RejectUnavailable:
			return L("Nobody can accept at the computer (it has no window).")
		case models.RejectOff:
			return L("Unattended access is off at the computer.")
		}
		return L("The person at the computer declined.")
	case viewer.StatusEnded:
		switch st.Reason {
		case models.EndHost:
			return L("The person at the computer ended the session.")
		case models.EndViewer:
			return L("You ended the session.")
		case models.EndNetwork:
			return L("The connection was lost.")
		case models.EndSignal:
			return L("The computer went offline.")
		case models.EndIdle:
			return L("The session ended after a while without use.")
		case models.EndRevoked:
			return L("The computer removed this link.")
		}
		return L("The session ended.")
	case viewer.StatusNotFound:
		return L("There is no computer with that code. Check it and try again.")
	case viewer.StatusRateLimited:
		return L("Too many tries. Wait a minute and try again.")
	case viewer.StatusUnreachable:
		return L("Cannot reach the go-link server. Check the connection.")
	case viewer.StatusUnlinked:
		return L("That computer no longer has this one linked. Link it again with its pairing code.")
	case viewer.StatusOffline:
		return L("That computer is not on, or go-link Desk is closed there.")
	case viewer.StatusPasswordRefused:
		if st.PasswordError == "locked" {
			return Lf("Too many wrong passwords. Try again in %d minutes.", (st.RetryAfter+59)/60)
		}
		return L("Too many wrong passwords.")
	case viewer.StatusUntrusted:
		return L("Whoever answered could not prove it is your linked computer. Nothing was sent to it.")
	case viewer.StatusSelf:
		return L("That is this computer.")
	}
	return ""
}

func (r *remote) onStatus(st viewer.Status) {
	text := statusText(st)
	switch st.Kind {
	case viewer.StatusPassword:
		r.view.Overlay(text, !st.Checking, L("Unlock"), st.Checking)
	case viewer.StatusLive:
		r.view.Overlay("", false, "", false)
		r.mu.Lock()
		first := !r.wasLive
		r.wasLive = true
		r.mu.Unlock()
		if first {
			r.view.Focus()
			r.remember()
		}
	default:
		r.view.Overlay(text, false, "", !st.Over())
	}
	if st.Over() {
		r.ended()
	}
}

func (r *remote) onHost(h viewer.HostInfo) {
	r.mu.Lock()
	r.host = h
	link := r.linked
	r.mu.Unlock()
	if h.Name != "" {
		r.view.SetTitle(h.Name)
	}
	if link != nil {
		r.keepLink(*link, h)
	}
	if r.door.Kind == viewer.DoorReach && r.door.Device != nil && r.u.opts.Remotes != nil {
		r.u.opts.Remotes.Update(r.door.Device.DeviceID, func(d *models.RemoteDevice) {
			if h.Name != "" {
				d.Name = h.Name
			}
			if h.OS != "" {
				d.OS = h.OS
			}
		})
	}
}

func (r *remote) onPermissions(p models.Permissions) {
	r.mu.Lock()
	r.perms = p
	muted := r.muted
	r.mu.Unlock()
	r.input.Allow(p.Control, p.Clipboard)
	r.view.SetControl(p.Control)
	r.view.ToolState("display", false, true)
	r.view.ToolState("quality", false, true)
	r.view.ToolState("keys", false, p.Control)
	r.view.ToolState("files", false, p.Files)
	r.view.ToolState("sound", !muted && p.Audio, p.Audio)
	r.setAudio(p.Audio && !muted)
}

func (r *remote) onDisplays(d []viewer.Display, current int) {
	r.mu.Lock()
	r.displays, r.current = d, current
	shape := r.shape
	r.mu.Unlock()
	r.input.SetDisplay(r.displayIndex())
	if shape != nil {
		r.onCursor(*shape)
	}
}

// displayIndex is the shown display's index in the host's list.
func (r *remote) displayIndex() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, d := range r.displays {
		if d.ID == r.current {
			return i
		}
	}
	return 0
}

func (r *remote) onSent(s viewer.Sent) {
	r.mu.Lock()
	r.sent = s
	hud := r.hud
	r.mu.Unlock()
	if hud {
		r.view.HUD(r.hudText())
	}
}

func (r *remote) hudText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sent
	return fmt.Sprintf(" %s %dx%d · %.0f fps · %d kbps (%s %d · %s %d) ", s.Codec, s.Width, s.Height, s.FPS, s.Kbps,
		L("aim"), s.TargetKbps, L("path"), s.EstimateKbps)
}

func (r *remote) onNotice(code string) {
	switch code {
	case "no_screen_permission":
		r.notify(L("The computer may not record its screen: you see a test card."))
	case "no_accessibility":
		r.notify(L("The computer has no Accessibility permission: your mouse and keyboard do not reach it."))
	case "capture_failed":
		r.notify(L("The computer could not capture its screen."))
	}
}

// notify shows a message in the session's chat window.
func (r *remote) notify(text string) {
	fyne.Do(func() { r.chatWindow().note(text) })
}

func (r *remote) onVideo(f viewer.Frame) {
	if f.Codec != "h264" {
		r.mu.Lock()
		said := r.codecOK
		r.codecOK = true
		r.mu.Unlock()
		if !said {
			r.view.Overlay(L("This computer sends its picture in a format this app cannot show yet. Use the website."), false, "", false)
		}
		return
	}
	r.view.Video(f.Data)
}

func (r *remote) onAudio(samples []int16) {
	r.mu.Lock()
	e := r.audio
	r.mu.Unlock()
	if e != nil {
		e.Play(samples)
	}
}

// setAudio opens or closes the speaker.
func (r *remote) setAudio(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if on && r.audio == nil && !r.closed {
		e, err := voice.Start(false, nil)
		if err != nil {
			r.u.opts.Logger.Warn("remote sound", "err", err)
			return
		}
		r.audio = e
	}
	if !on && r.audio != nil {
		r.audio.Stop()
		r.audio = nil
	}
}

func (r *remote) onCursor(shape viewer.CursorShape) {
	r.mu.Lock()
	r.shape = &shape
	var d *viewer.Display
	for i := range r.displays {
		if r.displays[i].ID == r.current {
			d = &r.displays[i]
		}
	}
	r.mu.Unlock()
	if d == nil || d.PointsWidth <= 0 || d.PointsHeight <= 0 {
		return
	}
	r.view.CursorShape(shape.PNG, shape.HotX, shape.HotY, shape.Width/float64(d.PointsWidth), shape.Height/float64(d.PointsHeight))
}

func (r *remote) onChat(line viewer.ChatLine) {
	fyne.Do(func() {
		c := r.chatWindow()
		c.add(line)
		c.show()
	})
}

// onLinked keeps the link the computer made (claim).
func (r *remote) onLinked(l viewer.NewLink) {
	r.mu.Lock()
	r.linked = &l
	h := r.host
	r.mu.Unlock()
	r.keepLink(l, h)
}

func (r *remote) keepLink(l viewer.NewLink, h viewer.HostInfo) {
	if r.u.opts.Remotes == nil {
		return
	}
	signal := r.u.opts.SignalURL
	if r.door.Kind == viewer.DoorReach && r.door.Device != nil && r.door.Device.SignalURL != "" {
		signal = r.door.Device.SignalURL
	}
	r.u.opts.Remotes.Link(models.RemoteDevice{DeviceID: l.DeviceID, LinkID: l.LinkID, Token: l.Token, SignalURL: signal,
		Name: h.Name, OS: h.OS, LastUsed: time.Now()})
}

func (r *remote) onUnlinked() {
	if r.door.Kind == viewer.DoorReach && r.door.Device != nil && r.u.opts.Remotes != nil {
		r.u.opts.Remotes.Forget(r.door.Device.DeviceID)
	}
}

func (r *remote) onFiles(f *viewer.Files) {
	r.mu.Lock()
	r.files = f
	r.mu.Unlock()
}

// remember adds the session to the recent ones, and marks a linked
// computer used.
func (r *remote) remember() {
	if r.u.opts.Remotes == nil {
		return
	}
	r.mu.Lock()
	h, thumb := r.host, r.thumb
	r.mu.Unlock()
	switch r.door.Kind {
	case viewer.DoorJoin:
		if r.door.Code != "" {
			r.u.opts.Remotes.AddRecent(models.RecentSession{Code: r.door.Code, Name: h.Name, OS: h.OS, Thumb: thumb})
		}
	case viewer.DoorReach:
		if r.door.Device != nil {
			r.u.opts.Remotes.Update(r.door.Device.DeviceID, func(d *models.RemoteDevice) {
				d.LastUsed = time.Now()
				if thumb != nil {
					d.Thumb = thumb
				}
			})
		}
	}
}

// thumbs keeps a small picture of the session for My computers.
func (r *remote) thumbs(ctx context.Context) {
	t := time.NewTicker(thumbEvery)
	defer t.Stop()
	first := time.After(3 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-first:
		case <-t.C:
		}
		if r.s.Status().Kind == viewer.StatusLive {
			r.view.Thumbnail()
		}
	}
}

// ended: the session is over; the tab stays with the reason until it is
// closed.
func (r *remote) ended() {
	r.input.Allow(false, false)
	r.view.SetControl(false)
	r.setAudio(false)
	r.remember()
	for _, t := range []string{"display", "quality", "keys", "files", "sound"} {
		r.view.ToolState(t, false, false)
	}
	if r.stop != nil {
		r.stop()
	}
}

// close ends the session and closes its windows.
func (r *remote) close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.resize != nil {
		r.resize.Stop()
	}
	r.mu.Unlock()
	r.s.HangUp()
	r.setAudio(false)
	r.view.Close()
	fyne.Do(func() {
		if r.chat != nil {
			r.chat.win.Close()
		}
		if r.browse != nil {
			r.browse.win.Close()
		}
	})
	r.u.remotesMu.Lock()
	for i, o := range r.u.remotes {
		if o == r {
			r.u.remotes = append(r.u.remotes[:i], r.u.remotes[i+1:]...)
			break
		}
	}
	r.u.remotesMu.Unlock()
}

// closeRemotes ends every session to another computer (the app quits).
func (u *ui) closeRemotes() {
	u.remotesMu.Lock()
	list := append([]*remote(nil), u.remotes...)
	u.remotesMu.Unlock()
	for _, r := range list {
		r.close()
	}
}

// remoteview.Handler: the window's events (on the main thread).

func (r *remote) Move(x, y float64, inside bool) { r.input.Move(x, y, inside) }

func (r *remote) Button(b int, down bool, x, y float64, inside bool) {
	r.input.Button(b, down, x, y, inside)
}

func (r *remote) Wheel(dx, dy float64, lines bool) { r.input.Wheel(dx, dy, lines) }

func (r *remote) Key(code int, down, repeat bool, mods int) {
	r.input.Key(code, down, repeat, mods)
}

func (r *remote) Resize(w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resize != nil {
		r.resize.Stop()
	}
	r.resize = time.AfterFunc(300*time.Millisecond, func() { r.s.SetVideoWant(w, h) })
}

func (r *remote) Thumb(jpeg []byte) {
	if len(jpeg) > models.MaxThumb {
		return
	}
	r.mu.Lock()
	r.thumb = jpeg
	r.mu.Unlock()
	go r.remember()
}

func (r *remote) Action(name, arg string) {
	switch name {
	case "disconnect":
		go r.close()
	case "closed":
		go r.close()
	case "password":
		r.s.SendPassword(arg)
	case "keyframe":
		r.s.RequestKeyframe()
	case "blur":
		r.input.ReleaseAll()
	case "hidden", "visible":
		r.s.SetHidden(name == "hidden")
	case "fullscreen":
		r.view.ToggleFullScreen()
	case "info":
		r.mu.Lock()
		r.hud = !r.hud
		on := r.hud
		r.mu.Unlock()
		r.view.ToolState("info", on, true)
		if on {
			r.view.HUD(r.hudText())
		} else {
			r.view.HUD("")
		}
	case "sound":
		r.mu.Lock()
		r.muted = !r.muted
		muted, allowed := r.muted, r.perms.Audio
		r.mu.Unlock()
		r.view.ToolState("sound", !muted && allowed, allowed)
		r.setAudio(allowed && !muted)
	case "chat":
		fyne.Do(func() { r.chatWindow().show() })
	case "files":
		fyne.Do(func() { r.filesWindow().show() })
	case "display":
		r.view.Menu("display", r.displayMenu())
	case "quality":
		r.view.Menu("quality", r.qualityMenu())
	case "keys":
		r.view.Menu("keys", r.keysMenu())
	case "drop", "upload":
		r.upload(strings.Split(arg, "\n"))
	case "menu":
		r.choose(arg)
	}
}

func (r *remote) displayMenu() []remoteview.MenuItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []remoteview.MenuItem
	for i, d := range r.displays {
		title := d.Name
		if title == "" {
			title = Lf("Display %d", i+1)
		}
		items = append(items, remoteview.MenuItem{ID: fmt.Sprintf("display:%d", d.ID), Title: fmt.Sprintf("%s (%d×%d)", title, d.Width, d.Height), Checked: d.ID == r.current})
	}
	if len(items) == 0 {
		items = append(items, remoteview.MenuItem{Title: L("Only one picture"), Disabled: true})
	}
	return items
}

func (r *remote) qualityMenu() []remoteview.MenuItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []remoteview.MenuItem{
		{ID: "preset:speed", Title: L("Speed"), Checked: r.preset == viewer.PresetSpeed},
		{ID: "preset:balanced", Title: L("Balanced"), Checked: r.preset == viewer.PresetBalanced},
		{ID: "preset:quality", Title: L("Quality"), Checked: r.preset == viewer.PresetQuality},
		{Separator: true},
		{ID: "retina", Title: L("Retina (the display's own pixels)"), Checked: r.retina},
	}
}

func (r *remote) keysMenu() []remoteview.MenuItem {
	r.mu.Lock()
	os := r.host.OS
	r.mu.Unlock()
	items := []remoteview.MenuItem{{ID: "type-clipboard", Title: L("Type the clipboard")}, {Separator: true}}
	if os == "darwin" || os == "" {
		items = append(items,
			remoteview.MenuItem{ID: "keys:MetaLeft+Tab", Title: L("Switch apps (Cmd+Tab)")},
			remoteview.MenuItem{ID: "keys:MetaLeft+Space", Title: L("Spotlight (Cmd+Space)")},
			remoteview.MenuItem{ID: "keys:MetaLeft+AltLeft+Escape", Title: L("Force quit (Cmd+Option+Esc)")},
			remoteview.MenuItem{ID: "keys:ControlLeft+MetaLeft+KeyQ", Title: L("Lock screen (Ctrl+Cmd+Q)")},
			remoteview.MenuItem{ID: "keys:MetaLeft+ShiftLeft+Digit3", Title: L("Screenshot (Cmd+Shift+3)")},
		)
	} else {
		items = append(items,
			remoteview.MenuItem{ID: "keys:ControlLeft+AltLeft+Delete", Title: "Ctrl+Alt+Del"},
			remoteview.MenuItem{ID: "keys:AltLeft+Tab", Title: L("Switch apps (Alt+Tab)")},
			remoteview.MenuItem{ID: "keys:MetaLeft", Title: L("Start menu (Win)")},
			remoteview.MenuItem{ID: "keys:MetaLeft+KeyL", Title: L("Lock screen (Win+L)")},
		)
	}
	return items
}

func (r *remote) choose(id string) {
	switch {
	case strings.HasPrefix(id, "display:"):
		var d int
		if _, err := fmt.Sscanf(id, "display:%d", &d); err == nil {
			r.s.SelectDisplay(d)
		}
	case strings.HasPrefix(id, "preset:"):
		r.mu.Lock()
		r.preset = viewer.Preset(strings.TrimPrefix(id, "preset:"))
		p, retina := r.preset, r.retina
		r.mu.Unlock()
		r.s.SetQuality(p, retina)
	case id == "retina":
		r.mu.Lock()
		r.retina = !r.retina
		p, retina := r.preset, r.retina
		r.mu.Unlock()
		r.s.SetQuality(p, retina)
	case id == "type-clipboard":
		if text, ok := r.board.Text(); ok {
			r.input.TypeText(text)
		}
	case strings.HasPrefix(id, "keys:"):
		r.input.Combo(strings.Split(strings.TrimPrefix(id, "keys:"), "+")...)
	}
}

// upload sends files to the computer, one after the other.
func (r *remote) upload(paths []string) {
	r.mu.Lock()
	f, allowed := r.files, r.perms.Files
	r.mu.Unlock()
	if !allowed || f == nil {
		r.notify(L("The computer does not let you send files in this session."))
		return
	}
	fyne.Do(func() {
		w := r.filesWindow()
		w.show()
		for _, p := range paths {
			if p != "" {
				w.queueUpload(p)
			}
		}
	})
}

// downloadsDir is where files from the computer land.
func downloadsDir() string {
	if me, err := user.Current(); err == nil {
		return filepath.Join(me.HomeDir, "Downloads")
	}
	return "."
}
