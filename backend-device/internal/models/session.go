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
	// Record: the viewer may save pictures of the screen and record it
	// (never by default). The viewer asks before each; the host shows it.
	Record bool `json:"record"`
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
	RejectOff         = "off"         // a linked browser, but unattended access is off
)

// Why a session ended ("session_end" reason).
const (
	EndHost    = "host"    // the person at the host pressed Disconnect
	EndViewer  = "viewer"  // the viewer hung up or left
	EndNetwork = "network" // the connection failed and could not recover
	EndSignal  = "signal"  // the host lost signalhub (the room is gone)
	EndIdle    = "idle"    // nothing from the viewer for the time set at the host
	EndRevoked = "revoked" // the person at the host revoked the viewer's link
)

// Knock is what a viewer declares when it asks to come in: a name and its
// browser. Both are shown as declared, never trusted.
type Knock struct {
	Name    string `json:"name"`
	Browser string `json:"ua"`
	// Codecs are the video codecs the browser takes ("h264-high",
	// "h264", "vp8"), to pick the session's.
	Codecs []string `json:"codecs,omitempty"`
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
	var codecs []string
	for _, c := range k.Codecs {
		if len(codecs) < 8 && len(c) <= 16 {
			codecs = append(codecs, c)
		}
	}
	return Knock{Name: cleanText(k.Name, MaxKnockName), Browser: cleanText(k.Browser, MaxKnockBrowser), Codecs: codecs}
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
	// Unattended: a linked browser that came in without anyone accepting.
	Unattended bool `json:"unattended,omitempty"`
	// RecordingSince: the viewer records the screen since then (zero:
	// it does not).
	RecordingSince time.Time `json:"recording_since,omitzero"`
	// Screenshots the viewer saved, and when the last one.
	Screenshots    int       `json:"screenshots,omitempty"`
	LastScreenshot time.Time `json:"last_screenshot,omitzero"`
}

// MaxChat bounds a chat message, in characters.
const MaxChat = 2000

// CleanChat trims a chat message to printable text of bounded length:
// line breaks stay (at most 20 lines), control and format characters go.
func CleanChat(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 20 {
		lines = lines[:20]
	}
	left := MaxChat
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if left <= 0 {
			break
		}
		c := cleanText(l, left)
		left -= len([]rune(c)) + 1
		out = append(out, c)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
