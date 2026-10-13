// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"image"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
)

// AskCall shows "… wants a video call" and waits for the answer, the time
// to answer running out, or the call being dropped (ctx). It implements
// services.CallConsent for the window.
func (u *ui) AskCall(ctx context.Context, name string, video bool) (services.CallAnswer, error) {
	if name == "" {
		name = L("Someone")
	}
	answered := make(chan services.CallAnswer, 1)
	var win fyne.Window
	var once sync.Once
	finish := func(a services.CallAnswer) {
		once.Do(func() {
			answered <- a
			win.Close()
		})
	}
	fyne.DoAndWait(func() {
		win = u.app.NewWindow(L("Video call"))
		title := widget.NewLabelWithStyle(Lf("%s wants a video call.", name), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		title.Wrapping = fyne.TextWrapWord
		cam := widget.NewCheck(L("Turn on my camera"), nil)
		cam.SetChecked(video)
		mic := widget.NewCheck(L("Turn on my microphone"), nil)
		mic.SetChecked(true)
		decline := widget.NewButtonWithIcon(L("Decline"), theme.CancelIcon(), func() { finish(services.CallAnswer{}) })
		accept := widget.NewButtonWithIcon(L("Accept"), theme.ConfirmIcon(), func() {
			finish(services.CallAnswer{Accept: true, Camera: cam.Checked, Mic: mic.Checked})
		})
		accept.Importance = widget.HighImportance
		win.SetContent(container.NewPadded(container.NewVBox(title, cam, mic, container.NewHBox(layout.NewSpacer(), decline, accept))))
		win.Resize(fyne.NewSize(380, 0))
		win.SetFixedSize(true)
		win.SetCloseIntercept(func() { finish(services.CallAnswer{}) })
		win.Show()
		styleWindow(win)
		floatWindow(win.Title(), true)
	})
	select {
	case a := <-answered:
		return a, nil
	case <-ctx.Done():
		fyne.Do(func() { once.Do(win.Close) })
		return services.CallAnswer{}, ctx.Err()
	}
}

// callWindow is the call while it lasts: the viewer's camera, and the
// host's camera, microphone and Hang up.
type callWindow struct {
	call    *services.CallService
	win     fyne.Window
	picture *canvas.Image
	note    *widget.Label
	camera  *widget.Check
	mic     *widget.Check
	setting bool
	shown   bool
	last    time.Time // the last picture drawn
}

func (u *ui) newCallWindow(call *services.CallService) *callWindow {
	w := &callWindow{call: call}
	w.win = u.app.NewWindow(L("Video call"))
	w.picture = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 4, 3)))
	w.picture.FillMode = canvas.ImageFillContain
	w.picture.SetMinSize(fyne.NewSize(480, 360))
	w.note = widget.NewLabel("")
	w.camera = widget.NewCheck(L("My camera"), func(on bool) {
		if !w.setting {
			go call.SetCamera(on)
		}
	})
	w.mic = widget.NewCheck(L("My microphone"), func(on bool) {
		if !w.setting {
			go call.SetMic(on)
		}
	})
	hangUp := widget.NewButtonWithIcon(L("Hang up"), theme.CancelIcon(), func() { go call.End() })
	hangUp.Importance = widget.DangerImportance
	bar := container.NewHBox(w.camera, w.mic, layout.NewSpacer(), hangUp)
	w.win.SetContent(container.NewBorder(nil, container.NewVBox(w.note, bar), nil, nil, w.picture))
	w.win.SetCloseIntercept(func() { go call.End() })
	call.OnRemoteFrame(func(img *image.RGBA) {
		fyne.Do(func() {
			if !w.shown || time.Since(w.last) < 30*time.Millisecond {
				return
			}
			w.last = time.Now()
			w.picture.Image = img
			w.picture.Refresh()
		})
	})
	return w
}

// render shows the window during a call and hides it after.
func (w *callWindow) render(st services.CallStatus) {
	if st.State != services.CallActive {
		if w.shown {
			w.win.Hide()
			w.shown = false
			w.picture.Image = image.NewRGBA(image.Rect(0, 0, 4, 3))
		}
		return
	}
	w.setting = true
	w.camera.SetChecked(st.HostCamera)
	w.mic.SetChecked(st.HostMic)
	w.setting = false
	name := st.Name
	if name == "" {
		name = L("Someone")
	}
	switch {
	case !st.ViewerCamera && !st.ViewerMic:
		w.note.SetText(Lf("%s has the camera and microphone off.", name))
	case !st.ViewerCamera:
		w.note.SetText(Lf("%s has the camera off.", name))
	case !st.ViewerMic:
		w.note.SetText(Lf("%s has the microphone off.", name))
	default:
		w.note.SetText(Lf("In a call with %s", name))
	}
	if !w.shown {
		w.win.Show()
		styleWindow(w.win)
		w.shown = true
	}
}
