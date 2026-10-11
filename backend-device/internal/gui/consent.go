// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// scamWarningKey remembers, in Fyne's preferences, that the person at the
// computer has seen the warning about phone scams once.
const scamWarningKey = "scam_warning_seen"

// request is the window that asks whether a viewer may come in.
type request struct {
	win       fyne.Window
	accept    *widget.Button
	reject    *widget.Button
	who       *widget.Label
	browser   *widget.Label
	warning   *widget.Label
	countdown *widget.Label
	perms     *permChecks // what else the viewer may do
	deadline  time.Time

	once   sync.Once
	answer func(error) // nil: accepted
}

// Ask shows the connection request and waits for the answer, the end of
// the time to answer or the viewer leaving (ctx). It implements
// services.Consent for the window.
func (u *ui) Ask(ctx context.Context, req services.ConsentRequest) (models.Permissions, error) {
	answered := make(chan error, 1)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(services.ConsentTimeout)
	}
	var r *request
	fyne.Do(func() {
		r = u.newRequest(req, deadline, func(err error) { answered <- err })
		r.win.Show()
		floatWindow(r.win.Title(), true)
	})
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-answered:
			if err != nil {
				return models.Permissions{}, err
			}
			perm := models.ViewOnly
			fyne.DoAndWait(func() { perm = r.perms.value() })
			return perm, nil
		case <-ctx.Done():
			fyne.Do(func() {
				if r != nil {
					r.close()
				}
			})
			return models.Permissions{}, ctx.Err()
		case <-tick.C:
			fyne.Do(func() {
				if r != nil {
					r.tick(time.Now())
				}
			})
		}
	}
}

// newRequest builds the request window without showing it (tests tap it).
func (u *ui) newRequest(req services.ConsentRequest, deadline time.Time, answer func(error)) *request {
	r := &request{deadline: deadline, answer: answer}
	r.win = u.app.NewWindow(L("Connection request"))
	name := req.Knock.Name
	if name == "" {
		name = L("Someone")
	}
	who := Lf("%s wants to see your screen.", name)
	if req.Link {
		who = Lf("%s wants to link this browser to your computer.", name)
	}
	r.who = widget.NewLabelWithStyle(who, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	r.who.Wrapping = fyne.TextWrapWord
	browser := req.Knock.Browser
	if browser == "" {
		browser = L("unknown")
	}
	r.browser = widget.NewLabel(Lf("Browser: %s", browser))
	r.browser.Wrapping = fyne.TextWrapWord
	note := widget.NewLabel(L("The name and the browser are what the other side says; go-link Desk cannot check them."))
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance
	perms := widget.NewLabel(L("If you accept, they will be able to:") + "\n  • " + L("See your screen"))
	r.perms = newPermChecks(nil)
	r.countdown = widget.NewLabel("")
	r.countdown.Importance = widget.LowImportance

	items := []fyne.CanvasObject{r.who, r.browser, note}
	if req.Link {
		// Accepting a link is accepting every later visit of that browser.
		link := widget.NewLabel(L("If you accept, this browser will come back without asking you, whenever go-link Desk is open, with the permissions you tick now. You can revoke it in Settings."))
		link.Wrapping = fyne.TextWrapWord
		link.Importance = widget.WarningImportance
		items = append(items, link)
	}
	items = append(append(items, widget.NewSeparator(), perms), r.perms.objects()...)
	// The first time, the warning about the phone scam: whoever calls and
	// asks for this is not who they say.
	if !u.app.Preferences().Bool(scamWarningKey) {
		r.warning = widget.NewLabelWithStyle(L("Did someone call you and ask you to do this? Reject it and hang up. Nobody legitimate calls asking for access to your computer: not your bank, not the police, not a support line."), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		r.warning.Wrapping = fyne.TextWrapWord
		r.warning.Importance = widget.DangerImportance
		items = append(items, widget.NewSeparator(), r.warning)
	}
	r.reject = widget.NewButtonWithIcon(L("Reject"), theme.CancelIcon(), func() { r.finish(services.ErrDeclined) })
	r.accept = widget.NewButtonWithIcon(L("Accept"), theme.ConfirmIcon(), func() { r.finish(nil) })
	r.accept.Importance = widget.HighImportance
	items = append(items, r.countdown, container.NewHBox(layout.NewSpacer(), r.reject, r.accept))
	r.win.SetContent(container.NewPadded(container.NewVBox(items...)))
	r.win.Resize(fyne.NewSize(440, 0))
	r.win.SetFixedSize(true)
	// Closing the window is a no.
	r.win.SetCloseIntercept(func() { r.finish(services.ErrDeclined) })
	r.tick(time.Now())
	r.onAnswer(func() { u.app.Preferences().SetBool(scamWarningKey, true) })
	return r
}

// onAnswer runs fn on the first answer, before it is passed on.
func (r *request) onAnswer(fn func()) {
	next := r.answer
	r.answer = func(err error) {
		fn()
		next(err)
	}
}

// tick shows the seconds left.
func (r *request) tick(now time.Time) {
	left := int(math.Ceil(r.deadline.Sub(now).Seconds()))
	r.countdown.SetText(Lf("Rejected on its own in %d s.", max(left, 0)))
}

// finish answers once and closes the window.
func (r *request) finish(err error) {
	r.once.Do(func() {
		r.answer(err)
		r.win.Close()
	})
}

// close closes the window without answering (the time ran out, or the
// viewer left).
func (r *request) close() {
	r.once.Do(func() { r.win.Close() })
}
