// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import "time"

// RemoteDevice is a computer this app linked to for unattended access
// (its pairing code, accepted there): the app comes back to it without a
// code. Token is this app's secret for it; the computer keeps only its
// hash. Kept in remotes.json with 0600 permissions.
type RemoteDevice struct {
	DeviceID string `json:"device_id"`
	LinkID   string `json:"link_id"`
	Token    string `json:"token"`
	// SignalURL is the signaling server the link was made through.
	SignalURL string `json:"signal_url,omitempty"`
	// Name and OS are what the computer said of itself; Label is the
	// name given to it here ("" for Name).
	Name     string    `json:"name,omitempty"`
	Label    string    `json:"label,omitempty"`
	OS       string    `json:"os,omitempty"`
	Favorite bool      `json:"favorite,omitempty"`
	SavedAt  time.Time `json:"saved_at"`
	LastUsed time.Time `json:"last_used,omitzero"`
	// Thumb is a small JPEG of its screen, from the last session.
	Thumb []byte `json:"thumb,omitempty"`
	// Bound: the computer once proved itself bound to the connection
	// (proof2); from then on a proof without it is refused, so nobody in
	// the middle can fall back to the plain one.
	Bound bool `json:"bound,omitempty"`
}

// Title is how the computer is shown: its label, or its name.
func (d RemoteDevice) Title() string {
	if d.Label != "" {
		return d.Label
	}
	return d.Name
}

// RecentSession is a session this app had, to go back with its code.
type RecentSession struct {
	// Code is the session code it joined with (9 digits); DeviceID is set
	// instead for a linked computer.
	Code     string    `json:"code,omitempty"`
	DeviceID string    `json:"device_id,omitempty"`
	Name     string    `json:"name,omitempty"`
	OS       string    `json:"os,omitempty"`
	At       time.Time `json:"at"`
	Thumb    []byte    `json:"thumb,omitempty"`
}

// Remotes is everything the app keeps of the computers it connects to.
type Remotes struct {
	Devices []RemoteDevice  `json:"devices,omitempty"`
	Recents []RecentSession `json:"recents,omitempty"`
}

// Limits of what Remotes keeps.
const (
	MaxRecents = 12
	// MaxThumb bounds a thumbnail's JPEG.
	MaxThumb = 60 << 10
)
