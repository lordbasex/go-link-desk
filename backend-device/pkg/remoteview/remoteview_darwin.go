// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin && !headless

package remoteview

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Cocoa -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework QuartzCore -framework VideoToolbox -framework AudioToolbox
#include <stdint.h>
#include <stdlib.h>

void rv_main_open(uintptr_t handle, const char *labelsJSON);
void rv_main_state(const char *stateJSON);
void rv_main_show(void);
void rv_about(const char *version, const char *credits);
void rv_app_menu(const char *labelsJSON);
void rv_prefs(uintptr_t handle, const char *specJSON, int show);
void rv_dock_icon(const uint8_t *png, int n);
void rv_theme(int mode);
void *rv_open(uintptr_t handle, const char *title, int tabs, const char *toolsJSON);
void rv_title(void *ref, const char *title);
void rv_overlay(void *ref, const char *text, int password, const char *button, int spinner);
void rv_hud(void *ref, const char *text);
void rv_video(void *ref, const uint8_t *data, int n);
void rv_picture(void *ref, int w, int h, const uint8_t *y, const uint8_t *u, const uint8_t *v, int sy, int suv);
void rv_cursor_shape(void *ref, const uint8_t *png, int n, double hotX, double hotY, double wFrac, double hFrac);
void rv_cursor_pos(void *ref, double x, double y, int visible);
void rv_control(void *ref, int on);
void rv_menu(void *ref, const char *tool, const char *itemsJSON);
void rv_screenshot(void *ref, const char *path);
void rv_rec_start(void *ref, const char *path, int audio);
void rv_rec_audio(void *ref, const int16_t *pcm, int n);
void rv_rec_stop(void *ref);
void rv_tool_state(void *ref, const char *tool, int on, int enabled);
void rv_fullscreen(void *ref);
void rv_open_panel(void *ref, const char *prompt);
void rv_thumb(void *ref);
void rv_focus(void *ref);
void rv_close(void *ref);
*/
import "C"

import (
	"encoding/json"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

// Supported reports whether this build has the remote view.
const Supported = true

// mainHandle keeps the main window's handler alive.
var mainHandle cgo.Handle

// OpenMain builds the main window (hidden until ShowMain): Home and the
// sessions' tabs. labels are its texts (MainLabels); h hears its actions.
func OpenMain(labels map[string]string, h MainHandler) error {
	b, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	mainHandle = cgo.NewHandle(mainAdapter{h})
	c := C.CString(string(b))
	defer C.free(unsafe.Pointer(c))
	C.rv_main_open(C.uintptr_t(mainHandle), c)
	return nil
}

// SetHome shows Home's state.
func SetHome(st Home) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	c := C.CString(string(b))
	defer C.free(unsafe.Pointer(c))
	C.rv_main_state(c)
}

// SetPrefs shows Settings as described (show: bring it to the front);
// changes reach the main window's handler as "setting" ("id\nvalue").
func SetPrefs(p Prefs, show bool) {
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	c := C.CString(string(b))
	defer C.free(unsafe.Pointer(c))
	C.rv_prefs(C.uintptr_t(mainHandle), c, cbool(show))
}

// SetDockIcon shows png as the running app's icon in the Dock.
func SetDockIcon(png []byte) {
	if len(png) == 0 {
		return
	}
	C.rv_dock_icon((*C.uint8_t)(unsafe.Pointer(&png[0])), C.int(len(png)))
}

// Themes of the windows.
const (
	ThemeDark   = 0
	ThemeLight  = 1
	ThemeSystem = 2
)

// SetTheme picks the windows' theme (ThemeSystem follows macOS).
func SetTheme(mode int) { C.rv_theme(C.int(mode)) }

// ShowAbout shows the About panel: the version, and what the app is.
func ShowAbout(version, credits string) {
	v, c := C.CString(version), C.CString(credits)
	defer C.free(unsafe.Pointer(v))
	defer C.free(unsafe.Pointer(c))
	C.rv_about(v, c)
}

// SetAppMenu makes the app menu go-link Desk's: About, Settings… (⌘,),
// the system's entries, and Quit. The labels: about, settings, services,
// hide, hideOthers, showAll, quit.
func SetAppMenu(labels map[string]string) {
	b, _ := json.Marshal(labels)
	c := C.CString(string(b))
	defer C.free(unsafe.Pointer(c))
	C.rv_app_menu(c)
}

// ShowMain brings the main window to the front.
func ShowMain() { C.rv_main_show() }

// mainAdapter lets the main window's actions travel the sessions' path.
type mainAdapter struct{ h MainHandler }

func (mainAdapter) Move(float64, float64, bool)              {}
func (mainAdapter) Button(int, bool, float64, float64, bool) {}
func (mainAdapter) Wheel(float64, float64, bool)             {}
func (mainAdapter) Key(int, bool, bool, int)                 {}
func (mainAdapter) Resize(int, int)                          {}
func (mainAdapter) Thumb([]byte)                             {}
func (m mainAdapter) Action(name, arg string)                { m.h.MainAction(name, arg) }

// View is one open session (a tab of the main window, or a window).
type View struct {
	mu     sync.Mutex
	ref    unsafe.Pointer
	handle cgo.Handle
}

// Open shows a new window; h hears what happens in it.
func Open(cfg Config, h Handler) (*View, error) {
	tools, err := json.Marshal(cfg.Tools)
	if err != nil {
		return nil, err
	}
	v := &View{}
	v.handle = cgo.NewHandle(h)
	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))
	ctools := C.CString(string(tools))
	defer C.free(unsafe.Pointer(ctools))
	v.ref = C.rv_open(C.uintptr_t(v.handle), title, cbool(cfg.Tabs), ctools)
	return v, nil
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// with runs fn with the window, unless it is closed.
func (v *View) with(fn func(ref unsafe.Pointer)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.ref != nil {
		fn(v.ref)
	}
}

// SetTitle names the window (and its tab).
func (v *View) SetTitle(title string) {
	v.with(func(ref unsafe.Pointer) {
		c := C.CString(title)
		defer C.free(unsafe.Pointer(c))
		C.rv_title(ref, c)
	})
}

// Overlay shows a message over the picture ("" hides it), with a
// password field and its button, or a spinner.
func (v *View) Overlay(text string, password bool, button string, spinner bool) {
	v.with(func(ref unsafe.Pointer) {
		ct, cb := C.CString(text), C.CString(button)
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cb))
		C.rv_overlay(ref, ct, cbool(password), cb, cbool(spinner))
	})
}

// HUD shows the session's figures in a corner ("" hides them).
func (v *View) HUD(text string) {
	v.with(func(ref unsafe.Pointer) {
		c := C.CString(text)
		defer C.free(unsafe.Pointer(c))
		C.rv_hud(ref, c)
	})
}

// Video draws an H.264 access unit (Annex B).
func (v *View) Video(au []byte) {
	if len(au) == 0 {
		return
	}
	v.with(func(ref unsafe.Pointer) {
		C.rv_video(ref, (*C.uint8_t)(unsafe.Pointer(&au[0])), C.int(len(au)))
	})
}

// Picture draws a decoded I420 picture (VP8's). The planes are copied
// before it returns.
func (v *View) Picture(w, h int, y, u, vv []byte, strideY, strideUV int) {
	if w <= 0 || h <= 0 || len(y) < strideY*(h-1)+w || len(u) < strideUV*((h+1)/2-1)+(w+1)/2 || len(vv) < len(u) {
		return
	}
	v.with(func(ref unsafe.Pointer) {
		C.rv_picture(ref, C.int(w), C.int(h), (*C.uint8_t)(unsafe.Pointer(&y[0])), (*C.uint8_t)(unsafe.Pointer(&u[0])),
			(*C.uint8_t)(unsafe.Pointer(&vv[0])), C.int(strideY), C.int(strideUV))
	})
}

// CursorShape sets the computer's pointer image: hot spot in the image's
// pixels, size as a fraction of the picture's width and height.
func (v *View) CursorShape(png []byte, hotX, hotY, wFrac, hFrac float64) {
	if len(png) == 0 {
		return
	}
	v.with(func(ref unsafe.Pointer) {
		C.rv_cursor_shape(ref, (*C.uint8_t)(unsafe.Pointer(&png[0])), C.int(len(png)), C.double(hotX), C.double(hotY), C.double(wFrac), C.double(hFrac))
	})
}

// CursorPos places the computer's pointer (0..1 over the picture).
func (v *View) CursorPos(x, y float64, visible bool) {
	v.with(func(ref unsafe.Pointer) {
		C.rv_cursor_pos(ref, C.double(x), C.double(y), cbool(visible))
	})
}

// SetControl says whether this viewer controls the computer: the local
// pointer takes the computer's image, and shortcuts go to it.
func (v *View) SetControl(on bool) {
	v.with(func(ref unsafe.Pointer) { C.rv_control(ref, cbool(on)) })
}

// Menu pops a menu under a toolbar button.
func (v *View) Menu(tool string, items []MenuItem) {
	b, err := json.Marshal(items)
	if err != nil {
		return
	}
	v.with(func(ref unsafe.Pointer) {
		ct, ci := C.CString(tool), C.CString(string(b))
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(ci))
		C.rv_menu(ref, ct, ci)
	})
}

// ToolState highlights a toolbar button (on) or greys it out.
func (v *View) ToolState(tool string, on, enabled bool) {
	v.with(func(ref unsafe.Pointer) {
		c := C.CString(tool)
		defer C.free(unsafe.Pointer(c))
		C.rv_tool_state(ref, c, cbool(on), cbool(enabled))
	})
}

// Screenshot saves the picture as shown, full size, as a PNG at path; the
// Handler's Action("screenshot", why) says how it went ("" saved).
func (v *View) Screenshot(path string) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	v.with(func(ref unsafe.Pointer) { C.rv_screenshot(ref, p) })
}

// StartRecording writes the session's pictures, and with audio the sound
// given to RecordAudio, to an MPEG-4 file at path, until StopRecording.
func (v *View) StartRecording(path string, audio bool) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	v.with(func(ref unsafe.Pointer) { C.rv_rec_start(ref, p, cbool(audio)) })
}

// RecordAudio hands the recording the computer's sound (16-bit mono, 48 kHz).
func (v *View) RecordAudio(samples []int16) {
	if len(samples) == 0 {
		return
	}
	v.with(func(ref unsafe.Pointer) {
		C.rv_rec_audio(ref, (*C.int16_t)(unsafe.Pointer(&samples[0])), C.int(len(samples)))
	})
}

// StopRecording closes the file; Action("recorded", why) says how it went.
func (v *View) StopRecording() {
	v.with(func(ref unsafe.Pointer) { C.rv_rec_stop(ref) })
}

// ToggleFullScreen enters or leaves full screen.
func (v *View) ToggleFullScreen() {
	v.with(func(ref unsafe.Pointer) { C.rv_fullscreen(ref) })
}

// ChooseFiles asks for files to send (Action "upload").
func (v *View) ChooseFiles(prompt string) {
	v.with(func(ref unsafe.Pointer) {
		c := C.CString(prompt)
		defer C.free(unsafe.Pointer(c))
		C.rv_open_panel(ref, c)
	})
}

// Thumbnail asks for a small JPEG of the picture (Handler.Thumb).
func (v *View) Thumbnail() {
	v.with(func(ref unsafe.Pointer) { C.rv_thumb(ref) })
}

// Focus brings the window to the front.
func (v *View) Focus() {
	v.with(func(ref unsafe.Pointer) { C.rv_focus(ref) })
}

// Close closes the window. Nothing reaches the Handler after it.
func (v *View) Close() {
	v.mu.Lock()
	ref := v.ref
	v.ref = nil
	v.mu.Unlock()
	if ref == nil {
		return
	}
	C.rv_close(ref)
	// Calls already on their way from the main thread still find the
	// handler for a while; a stale one is ignored after that.
	h := v.handle
	time.AfterFunc(2*time.Second, h.Delete)
}

func handler(h C.uintptr_t) (Handler, bool) {
	defer func() { _ = recover() }()
	hd, ok := cgo.Handle(h).Value().(Handler)
	return hd, ok
}

//export goRVMove
func goRVMove(h C.uintptr_t, x, y C.double, inside C.int) {
	if hd, ok := handler(h); ok {
		hd.Move(float64(x), float64(y), inside != 0)
	}
}

//export goRVButton
func goRVButton(h C.uintptr_t, button, down C.int, x, y C.double, inside C.int) {
	if hd, ok := handler(h); ok {
		hd.Button(int(button), down != 0, float64(x), float64(y), inside != 0)
	}
}

//export goRVWheel
func goRVWheel(h C.uintptr_t, dx, dy C.double, lines C.int) {
	if hd, ok := handler(h); ok {
		hd.Wheel(float64(dx), float64(dy), lines != 0)
	}
}

//export goRVKey
func goRVKey(h C.uintptr_t, code, down, repeat, mods C.int) {
	if hd, ok := handler(h); ok {
		hd.Key(int(code), down != 0, repeat != 0, int(mods))
	}
}

//export goRVResize
func goRVResize(h C.uintptr_t, w, hgt C.int) {
	if hd, ok := handler(h); ok {
		hd.Resize(int(w), int(hgt))
	}
}

//export goRVAction
func goRVAction(h C.uintptr_t, name, arg *C.char) {
	if hd, ok := handler(h); ok {
		hd.Action(C.GoString(name), C.GoString(arg))
	}
}

//export goRVThumb
func goRVThumb(h C.uintptr_t, data unsafe.Pointer, n C.int) {
	if hd, ok := handler(h); ok && n > 0 {
		hd.Thumb(C.GoBytes(data, n))
	}
}
