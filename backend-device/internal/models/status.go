// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import (
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/sysinfo"
)

// Signal connection states.
const (
	SignalConnecting   = "connecting"
	SignalConnected    = "connected"
	SignalDisconnected = "disconnected"
)

// Status is the host state shown by the window and, reduced, sent to
// linked browsers in device_status.
type Status struct {
	DeviceID string        `json:"device_id"`
	Version  string        `json:"version"`
	Signal   SignalStatus  `json:"signal"`
	Pairing  PairingStatus `json:"pairing"`
	Peers    []LinkedPeer  `json:"peers"`
	// SavedLinks counts the browsers remembered in device.json.
	SavedLinks int `json:"saved_links"`
	// Stream is only for the window: it is not sent to browsers.
	Stream StreamStatus `json:"-"`
	// Session is the room that carries remote sessions, once open.
	Session *SessionStatus `json:"session,omitempty"`
	System  *SystemStatus  `json:"system,omitempty"`
	// Update is a newer go-link Desk release, when there is one.
	Update *UpdateInfo `json:"update,omitempty"`
}

// UpdateInfo is a newer release of go-link Desk than this build.
type UpdateInfo struct {
	Latest string `json:"latest"` // its version, e.g. v0.2.0
	URL    string `json:"url"`    // its release page, with the downloads
}

// SystemStatus is the machine's hardware and live usage.
type SystemStatus struct {
	Hardware  sysinfo.Hardware `json:"hardware"`
	Usage     sysinfo.Usage    `json:"usage"`
	SampledAt time.Time        `json:"sampled_at"`
}

// SessionStatus is the invite-only room this host keeps open on signalhub
// and the code a viewer types to reach it.
type SessionStatus struct {
	RoomID string `json:"room_id"`
	// Code is the room's 9-digit invitation code, without spaces. It is
	// never sent to linked browsers.
	Code string `json:"-"`
	// Invite is the 22-character invitation the /c/<invite> link carries.
	Invite  string `json:"-"`
	Viewers int    `json:"viewers"`
}

// StreamStatus is what the host is sending right now.
type StreamStatus struct {
	FPS          float64 `json:"fps"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	VideoKbps    float64 `json:"video_kbps"` // encoded video, before each viewer's copy
	VideoViewers int     `json:"video_viewers"`
}

// SignalStatus describes the link with signalhub.
type SignalStatus struct {
	URL       string `json:"url"`
	State     string `json:"state"`
	PeerID    string `json:"peer_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
	// ICEURLs lists the STUN/TURN URLs received in hello. Credentials are
	// never exposed.
	ICEURLs []string `json:"ice_urls"`
}

// PairingStatus is the code that links a browser for unattended access.
// The host replaces it before it expires and right after someone redeems
// it.
type PairingStatus struct {
	Code        string     `json:"code,omitempty"` // "113 134 323"
	IssuedAt    *time.Time `json:"issued_at,omitempty"`
	RefreshesAt *time.Time `json:"refreshes_at,omitempty"`
}

// LinkedPeer is a browser linked to this host's signalhub session.
type LinkedPeer struct {
	PeerID string    `json:"peer_id"`
	Since  time.Time `json:"since"`
	// LatencyMs is the round trip over the WebRTC DataChannel, when known.
	LatencyMs *int `json:"latency_ms,omitempty"`
}
