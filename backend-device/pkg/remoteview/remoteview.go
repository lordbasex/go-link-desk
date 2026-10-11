// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package remoteview is the window where another computer's screen is
// shown and controlled: its picture (H.264, decoded and drawn by the
// system), its pointer, a toolbar, and the mouse and keyboard taken while
// it has the focus. On macOS each window opens as a tab of the other
// session windows (or on its own), and a tab can be dragged out to a
// window of its own.
package remoteview

import "errors"

// ErrUnsupported: this system has no remote view in this build.
var ErrUnsupported = errors.New("remoteview: not supported on this system")

// Buttons, as the input protocol numbers them.
const (
	ButtonLeft    = 0
	ButtonMiddle  = 1
	ButtonRight   = 2
	ButtonBack    = 3
	ButtonForward = 4
)

// Modifier bits, as the input protocol has them.
const (
	ModShift = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
	ModCaps
)

// Tool is one toolbar button.
type Tool struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Symbol string `json:"symbol"` // an SF Symbols name
}

// MenuItem is one entry of a toolbar button's menu.
type MenuItem struct {
	ID        string `json:"id,omitempty"`
	Title     string `json:"title,omitempty"`
	Checked   bool   `json:"checked,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
	Separator bool   `json:"separator,omitempty"`
}

// Handler receives what happens in the window. It is called on the
// system's main thread: it must not block.
type Handler interface {
	// Move: the pointer is at x, y (0..1 over the picture); inside is
	// false when it is over the window but not the picture.
	Move(x, y float64, inside bool)
	Button(button int, down bool, x, y float64, inside bool)
	// Wheel: the browser's sign (dy > 0 scrolls the content down), in
	// points, or in lines.
	Wheel(dx, dy float64, lines bool)
	// Key: macOS virtual key code, with the modifiers held.
	Key(code int, down, repeat bool, mods int)
	// Resize: the picture's area, in pixels.
	Resize(width, height int)
	// Action: a toolbar button ("disconnect", ...), a menu entry
	// (item "menu", arg the entry's id), "password" (arg: what was
	// typed), "drop" (arg: file paths, one a line), "upload" (the same,
	// from the open panel), "keyframe" (the picture needs one), "blur",
	// "hidden", "visible", "closed".
	Action(name, arg string)
	// Thumb: a small JPEG of the picture (Thumbnail).
	Thumb(jpeg []byte)
}

// Config opens a window.
type Config struct {
	Title string
	// Tabs opens it as a tab of the session windows (false: a window of
	// its own).
	Tabs  bool
	Tools []Tool
}

// MainHandler hears the main window's actions: "copy", "newcode",
// "settings", "allow" (arg: the permission's id), "connect" (arg:
// "join:<code>" or "claim:<code>"), "device" and "favorite", "forget"
// (arg: a linked computer's id), "rename" (arg: id, a line break, the new
// name), "recent" (arg: its code), "clear".
type MainHandler interface {
	MainAction(name, arg string)
}

// Home is what the main window's Home tab shows.
type Home struct {
	Code      string `json:"code"`       // formatted, "" while there is none
	Link      string `json:"link"`       // the invitation's address
	Status    string `json:"status"`     // the connection, in words
	StatusOK  bool   `json:"status_ok"`  // connected to the server
	InSession bool   `json:"in_session"` // someone is connected to this computer
	CanNew    bool   `json:"can_new"`
	Version   string `json:"version"`
	Perms     []Perm `json:"perms"`
	Devices   []Tile `json:"devices"`
	Recents   []Tile `json:"recents"`
}

// Perm is one of the system permissions the host needs.
type Perm struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Granted bool   `json:"granted"`
}

// Tile is a computer in My computers, or a recent session.
type Tile struct {
	ID       string `json:"id"`
	Title    string `json:"title"`    // over the card's picture
	Subtitle string `json:"subtitle"` // under the title in the list
	Name     string `json:"name"`     // the detailed list's columns
	Code     string `json:"code"`
	System   string `json:"system"`
	When     string `json:"when"`
	Thumb    []byte `json:"thumb,omitempty"` // a JPEG
	Favorite bool   `json:"favorite,omitempty"`
}

// Prefs is the Settings window: a page per toolbar button.
type Prefs struct {
	Pages []PrefPage `json:"pages"`
}

// PrefPage is one of Settings' pages.
type PrefPage struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Symbol   string        `json:"symbol"` // an SF Symbols name
	Sections []PrefSection `json:"sections"`
}

// PrefSection is a titled box of rows, with a note under it.
type PrefSection struct {
	Title  string    `json:"title,omitempty"`
	Footer string    `json:"footer,omitempty"`
	Rows   []PrefRow `json:"rows"`
}

// PrefRow is a label on the left and, by Kind, on the right: "info" (a
// value), "toggle", "select" (Options), "text", "password" (a field and
// Button), "button", "code" (a big value); or "note", text across.
type PrefRow struct {
	ID          string       `json:"id,omitempty"`
	Kind        string       `json:"kind"`
	Label       string       `json:"label"`
	Detail      string       `json:"detail,omitempty"`
	Value       string       `json:"value,omitempty"`
	On          bool         `json:"on,omitempty"`
	Enabled     *bool        `json:"enabled,omitempty"` // nil: enabled
	Tint        string       `json:"tint,omitempty"`    // ok, warn, danger
	Placeholder string       `json:"placeholder,omitempty"`
	Button      string       `json:"button,omitempty"`
	Repeat      string       `json:"repeat,omitempty"` // password: a second field to type it again (its placeholder)
	Options     []PrefOption `json:"options,omitempty"`
	Confirm     *PrefConfirm `json:"confirm,omitempty"` // asked before a button acts
}

// PrefOption is one choice of a select.
type PrefOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// PrefConfirm asks before a button acts.
type PrefConfirm struct {
	Title  string `json:"title"`
	Text   string `json:"text"`
	OK     string `json:"ok"`
	Cancel string `json:"cancel"`
}
