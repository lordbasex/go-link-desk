// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
)

// remoteChat is the chat with the person at the other computer.
type remoteChat struct {
	r      *remote
	win    fyne.Window
	lines  *fyne.Container
	scroll *container.Scroll
	entry  *widget.Entry
	out    *typingSender
	in     *typingShown
}

// chatWindow returns the session's chat window (on Fyne's thread).
func (r *remote) chatWindow() *remoteChat {
	if r.chat != nil {
		return r.chat
	}
	c := &remoteChat{r: r}
	c.win = r.u.app.NewWindow(r.windowTitle(L("Chat")))
	c.lines = container.NewVBox()
	c.scroll = container.NewVScroll(c.lines)
	c.entry = widget.NewEntry()
	c.entry.SetPlaceHolder(L("Write a message"))
	c.entry.OnSubmitted = func(string) { c.submit() }
	c.out = &typingSender{send: r.s.SendTyping}
	c.in = newTypingShown()
	c.entry.OnChanged = func(text string) { c.out.wrote(text != "") }
	send := widget.NewButtonWithIcon("", theme.MailSendIcon(), c.submit)
	c.win.SetContent(container.NewPadded(container.NewBorder(nil, container.NewVBox(c.in.label, container.NewBorder(nil, nil, nil, send, c.entry)), nil, nil, c.scroll)))
	c.win.Resize(fyne.NewSize(340, 420))
	c.win.SetCloseIntercept(c.win.Hide)
	r.chat = c
	return c
}

func (r *remote) windowTitle(what string) string {
	r.mu.Lock()
	name := r.host.Name
	r.mu.Unlock()
	if name == "" {
		return what
	}
	return what + " · " + name
}

func (c *remoteChat) show() {
	c.win.SetTitle(c.r.windowTitle(L("Chat")))
	c.win.Show()
	styleWindow(c.win)
}

func (c *remoteChat) submit() {
	if line, ok := c.r.s.SendChat(c.entry.Text); ok {
		c.entry.SetText("")
		c.out.quiet()
		c.add(line)
	}
}

// showTyping shows (or hides) that the person at the other computer is writing.
func (c *remoteChat) showTyping(on bool) {
	c.r.mu.Lock()
	name := c.r.host.Name
	c.r.mu.Unlock()
	if name == "" {
		name = L("The other computer")
	}
	c.in.show(on, name)
}

func (c *remoteChat) add(line viewer.ChatLine) {
	who := L("You")
	if line.FromHost {
		c.in.show(false, "")
	}
	if line.FromHost {
		c.r.mu.Lock()
		who = c.r.host.Name
		c.r.mu.Unlock()
		if who == "" {
			who = L("The other computer")
		}
	}
	l := widget.NewLabel(who + " · " + line.At.Local().Format("15:04") + "\n" + line.Text)
	l.Wrapping = fyne.TextWrapWord
	if !line.FromHost {
		l.Importance = widget.LowImportance
	}
	c.lines.Add(l)
	c.scroll.ScrollToBottom()
}

// note shows a message from the app itself.
func (c *remoteChat) note(text string) {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	l.Wrapping = fyne.TextWrapWord
	c.lines.Add(l)
	c.scroll.ScrollToBottom()
	c.show()
}

// remoteFiles browses the other computer's home folder, downloads from
// it and sends to it.
type remoteFiles struct {
	r       *remote
	win     fyne.Window
	where   *widget.Label
	list    *widget.List
	entries []viewer.FileEntry
	path    string
	status  *widget.Label
	bar     *widget.ProgressBar
	queue   []string
	busy    bool
	stop    chan struct{}
}

// filesWindow returns the session's files window (on Fyne's thread).
func (r *remote) filesWindow() *remoteFiles {
	if r.browse != nil {
		return r.browse
	}
	f := &remoteFiles{r: r}
	f.win = r.u.app.NewWindow(r.windowTitle(L("Files")))
	f.where = widget.NewLabelWithStyle("~", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	f.list = widget.NewList(
		func() int { return len(f.entries) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, widget.NewIcon(theme.FileIcon()), widget.NewLabel(""), widget.NewLabel(""))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i >= len(f.entries) {
				return
			}
			e := f.entries[i]
			row := o.(*fyne.Container)
			name := row.Objects[0].(*widget.Label)
			icon := row.Objects[1].(*widget.Icon)
			size := row.Objects[2].(*widget.Label)
			name.SetText(e.Name)
			if e.Dir {
				icon.SetResource(theme.FolderIcon())
				size.SetText("")
			} else {
				icon.SetResource(theme.FileIcon())
				size.SetText(humanSize(e.Size))
			}
		},
	)
	f.list.OnSelected = func(i widget.ListItemID) {
		f.list.UnselectAll()
		if i >= len(f.entries) {
			return
		}
		e := f.entries[i]
		if e.Dir {
			f.open(path.Join(f.path, e.Name))
			return
		}
		f.download(path.Join(f.path, e.Name), e.Name)
	}
	up := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if f.path != "" {
			f.open(path.Dir(f.path))
		}
	})
	home := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { f.open("") })
	send := widget.NewButtonWithIcon(L("Send files…"), theme.UploadIcon(), func() { r.view.ChooseFiles(L("Send")) })
	reveal := widget.NewButtonWithIcon(L("Show downloads"), theme.FolderOpenIcon(), func() { reveal(downloadsDir()) })
	f.status = widget.NewLabel(L("Click a file to download it to your Downloads folder, or drop files on the screen to send them."))
	f.status.Wrapping = fyne.TextWrapWord
	f.bar = widget.NewProgressBar()
	f.bar.Hide()
	stop := widget.NewButtonWithIcon(L("Stop"), theme.CancelIcon(), func() {
		if f.stop != nil {
			close(f.stop)
			f.stop = nil
		}
		f.queue = nil
	})
	top := container.NewBorder(nil, nil, container.NewHBox(up, home), container.NewHBox(send, reveal), f.where)
	bottom := container.NewVBox(f.status, container.NewBorder(nil, nil, nil, stop, f.bar))
	f.win.SetContent(container.NewPadded(container.NewBorder(top, bottom, nil, nil, f.list)))
	f.win.Resize(fyne.NewSize(520, 480))
	f.win.SetCloseIntercept(f.win.Hide)
	r.browse = f
	return f
}

func (f *remoteFiles) show() {
	f.win.SetTitle(f.r.windowTitle(L("Files")))
	f.win.Show()
	styleWindow(f.win)
	if f.entries == nil {
		f.open(f.path)
	}
}

// client is the files channel, when the session allows files.
func (f *remoteFiles) client() *viewer.Files {
	f.r.mu.Lock()
	defer f.r.mu.Unlock()
	if !f.r.perms.Files {
		return nil
	}
	return f.r.files
}

func (f *remoteFiles) open(p string) {
	files := f.client()
	if files == nil {
		f.status.SetText(L("The computer does not let you use its files in this session."))
		return
	}
	f.status.SetText(L("Loading…"))
	go func() {
		entries, err := files.List(p)
		fyne.Do(func() {
			if err != nil {
				f.status.SetText(Lf("Cannot open the folder: %s", err))
				return
			}
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Dir != entries[j].Dir {
					return entries[i].Dir
				}
				return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			})
			f.path, f.entries = p, entries
			f.where.SetText("~/" + p)
			f.list.Refresh()
			f.list.ScrollToTop()
			f.status.SetText(Lf("%d items", len(entries)))
		})
	}()
}

func (f *remoteFiles) download(p, name string) {
	files := f.client()
	if files == nil || f.busy {
		return
	}
	f.begin(Lf("Downloading %s…", name))
	stop := f.stop
	go func() {
		dest, err := files.Download(p, downloadsDir(), f.progress, stop)
		fyne.Do(func() {
			f.finish()
			if err != nil {
				f.status.SetText(Lf("%s was not downloaded: %s", name, err))
				return
			}
			f.status.SetText(Lf("Downloaded to %s", dest))
			f.next()
		})
	}()
}

// queueUpload sends a file after the ones before it.
func (f *remoteFiles) queueUpload(p string) {
	f.queue = append(f.queue, p)
	if !f.busy {
		f.next()
	}
}

func (f *remoteFiles) next() {
	if f.busy || len(f.queue) == 0 {
		return
	}
	files := f.client()
	if files == nil {
		f.queue = nil
		return
	}
	p := f.queue[0]
	f.queue = f.queue[1:]
	name := filepath.Base(p)
	f.begin(Lf("Sending %s…", name))
	stop := f.stop
	go func() {
		saved, err := files.Upload(p, f.progress, stop)
		fyne.Do(func() {
			f.finish()
			if err != nil {
				f.status.SetText(Lf("%s was not sent: %s", name, err))
			} else {
				f.status.SetText(Lf("Sent: it is in Downloads/go-link Desk/%s on the other computer", saved))
				if strings.HasPrefix(f.path, "Downloads") {
					f.open(f.path)
				}
			}
			f.next()
		})
	}()
}

func (f *remoteFiles) begin(text string) {
	f.busy = true
	f.stop = make(chan struct{})
	f.status.SetText(text)
	f.bar.SetValue(0)
	f.bar.Show()
}

func (f *remoteFiles) finish() {
	f.busy = false
	f.stop = nil
	f.bar.Hide()
}

// progress is called from the transfer's goroutine, often: it updates the
// bar a few times a second.
func (f *remoteFiles) progress(done, size int64) {
	if size <= 0 {
		return
	}
	v := float64(done) / float64(size)
	fyne.Do(func() {
		if f.busy && (v-f.bar.Value > 0.01 || v >= 1) {
			f.bar.SetValue(v)
		}
	})
}

// humanSize is 1.2 MB and so on.
func humanSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// reveal opens a folder in the system's file manager.
func reveal(dir string) {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	c := exec.Command(cmd, dir)
	if err := c.Start(); err == nil {
		go func() { _ = c.Wait() }()
	}
	_ = time.Second
}
