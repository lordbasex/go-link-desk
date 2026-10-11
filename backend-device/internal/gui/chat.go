// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
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
}

func newChatBox(chat *services.ChatService) *chatBox {
	c := &chatBox{chat: chat}
	c.lines = container.NewVBox()
	c.scroll = container.NewVScroll(c.lines)
	c.scroll.SetMinSize(fyne.NewSize(0, 120))
	c.entry = widget.NewEntry()
	c.entry.SetPlaceHolder(L("Write a message"))
	c.entry.OnSubmitted = func(string) { c.submit() }
	c.send = widget.NewButtonWithIcon("", theme.MailSendIcon(), c.submit)
	c.box = container.NewBorder(widget.NewLabelWithStyle(L("Chat"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), container.NewBorder(nil, nil, nil, c.send, c.entry), nil, nil, c.scroll)
	return c
}

func (c *chatBox) submit() {
	if c.chat.Send(c.entry.Text) {
		c.entry.SetText("")
	}
}

// add shows one message (on the app's goroutine).
func (c *chatBox) add(m services.ChatMessage) {
	who := L("You")
	if m.FromViewer {
		who = L("Viewer")
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
}
