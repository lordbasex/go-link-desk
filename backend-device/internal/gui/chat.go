// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// chatBox is the session's chat in the session panel: the messages so far
// and a line to write.
type chatBox struct {
	box    fyne.CanvasObject
	lines  *fyne.Container
	scroll *container.Scroll
	entry  *widget.Entry
	send   *widget.Button
	chat   *services.ChatService
	name   string // the viewer's name, for typing
	out    *typingSender
	in     *typingShown
}

// typingSender tells the other side this one is writing a message:
// "typing" again every few seconds while it writes, "stopped" after a
// pause, when the line is emptied or when the message goes.
type typingSender struct {
	send   func(on bool)
	mu     sync.Mutex
	sentAt time.Time
	stop   *time.Timer
}

func (t *typingSender) wrote(any bool) {
	if !any {
		t.quiet()
		return
	}
	t.mu.Lock()
	send := time.Since(t.sentAt) >= typingEvery
	if send {
		t.sentAt = time.Now()
	}
	if t.stop != nil {
		t.stop.Stop()
	}
	t.stop = time.AfterFunc(typingPause, t.quiet)
	t.mu.Unlock()
	if send {
		go t.send(true)
	}
}

func (t *typingSender) quiet() {
	t.mu.Lock()
	was := !t.sentAt.IsZero()
	t.sentAt = time.Time{}
	if t.stop != nil {
		t.stop.Stop()
		t.stop = nil
	}
	t.mu.Unlock()
	if was {
		go t.send(false)
	}
}

// typingShown is "… is typing" under a chat's lines, hidden when the
// other side goes quiet for a while (on the app's goroutine).
type typingShown struct {
	label *widget.Label
	hide  *time.Timer
}

func newTypingShown() *typingShown {
	l := widget.NewLabel("")
	l.Importance = widget.LowImportance
	l.Hide()
	return &typingShown{label: l}
}

func (t *typingShown) show(on bool, name string) {
	if t.hide != nil {
		t.hide.Stop()
		t.hide = nil
	}
	if !on {
		t.label.Hide()
		return
	}
	t.label.SetText(Lf("%s is typing…", name))
	t.label.Show()
	t.hide = time.AfterFunc(typingShownFor, func() { fyne.Do(func() { t.label.Hide() }) })
}

const (
	typingEvery    = 3 * time.Second // "typing" again while writing
	typingPause    = 4 * time.Second // a pause this long is "stopped"
	typingShownFor = 6 * time.Second // "is typing" goes without news this long
)

func newChatBox(chat *services.ChatService) *chatBox {
	c := &chatBox{chat: chat}
	c.lines = container.NewVBox()
	c.scroll = container.NewVScroll(c.lines)
	c.scroll.SetMinSize(fyne.NewSize(0, 120))
	c.entry = widget.NewEntry()
	c.entry.SetPlaceHolder(L("Write a message"))
	c.entry.OnSubmitted = func(string) { c.submit() }
	c.out = &typingSender{send: chat.Typing}
	c.in = newTypingShown()
	c.entry.OnChanged = func(text string) { c.out.wrote(text != "") }
	c.send = widget.NewButtonWithIcon("", theme.MailSendIcon(), c.submit)
	c.box = container.NewBorder(widget.NewLabelWithStyle(L("Chat"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(c.in.label, container.NewBorder(nil, nil, nil, c.send, c.entry)), nil, nil, c.scroll)
	return c
}

func (c *chatBox) submit() {
	if c.chat.Send(c.entry.Text) {
		c.entry.SetText("")
		c.out.quiet()
	}
}

// showTyping shows (or hides) that the viewer is writing (on the app's goroutine).
func (c *chatBox) showTyping(on bool) {
	name := c.name
	if name == "" {
		name = L("The viewer")
	}
	c.in.show(on, name)
}

// add shows one message (on the app's goroutine).
func (c *chatBox) add(m services.ChatMessage) {
	who := L("You")
	if m.FromViewer {
		who = L("Viewer")
	}
	if m.FromViewer {
		c.showTyping(false) // the message it was writing came
	}
	l := widget.NewLabel(who + " · " + m.At.Local().Format("15:04") + "\n" + m.Text)
	l.Wrapping = fyne.TextWrapWord
	if !m.FromViewer {
		l.Importance = widget.LowImportance
	}
	c.lines.Add(l)
	c.scroll.ScrollToBottom()
}

// clear empties the chat for a new session.
func (c *chatBox) clear() {
	c.lines.RemoveAll()
	c.entry.SetText("")
	c.showTyping(false)
}
