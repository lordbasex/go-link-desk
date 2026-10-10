// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import (
	"strings"
	"time"
	"unicode"
)

// Permissions are what the person at the host lets a viewer do. Each one
// is shown before the session starts and travels to the viewer in the
// "accepted" message. The defaults are the minimum: only View.
type Permissions struct {
	View      bool `json:"view"`
	Control   bool `json:"control"`
	Audio     bool `json:"audio"`
	Clipboard bool `json:"clipboard"`
	Files     bool `json:"files"`
}

// ViewOnly is the default grant: the viewer sees the screen and nothing
// else.
var ViewOnly = Permissions{View: true}

// Why a knock was turned down ("rejected" reason).
const (
	RejectDeclined    = "declined"    // the person at the host said no
	RejectTimeout     = "timeout"     // nobody answered in time
	RejectBusy        = "busy"        // another session is on
	RejectUnavailable = "unavailable" // nobody can answer: a host without a window
)

// Why a session ended ("session_end" reason).
const (
	EndHost    = "host"    // the person at the host pressed Disconnect
	EndViewer  = "viewer"  // the viewer hung up or left
	EndNetwork = "network" // the connection failed and could not recover
	EndSignal  = "signal"  // the host lost signalhub (the room is gone)
)

// Knock is what a viewer declares when it asks to come in: a name and its
// browser. Both are shown as declared, never trusted.
type Knock struct {
	Name    string `json:"name"`
	Browser string `json:"ua"`
}

// Limits of the declared fields, in characters.
const (
	MaxKnockName    = 40
	MaxKnockBrowser = 60
)

// Clean trims a knock's fields to printable text of bounded length, so
// what the consent dialog shows is what the viewer typed and nothing
// that could pass for the dialog's own text (no line breaks, no control
// or format characters).
func (k Knock) Clean() Knock {
	return Knock{Name: cleanText(k.Name, MaxKnockName), Browser: cleanText(k.Browser, MaxKnockBrowser)}
}

func cleanText(s string, max int) string {
	var b strings.Builder
	n := 0
	space := false
	for _, r := range strings.TrimSpace(s) {
		if n >= max {
			break
		}
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case !unicode.IsPrint(r):
			continue
		}
		if space && n > 0 {
			if n+1 >= max {
				break
			}
			b.WriteByte(' ')
			n++
		}
		space = false
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// ActiveSession is the viewer that is in, for the window and the tray.
type ActiveSession struct {
	PeerID      string      `json:"-"`
	Name        string      `json:"name"`
	Browser     string      `json:"browser"`
	Since       time.Time   `json:"since"`
	Permissions Permissions `json:"permissions"`
}
