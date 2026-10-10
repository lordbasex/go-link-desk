// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package models holds the host's domain types.
package models

import "time"

// App is the signalhub namespace of this project.
const App = "go-link-desk"

// DefaultSignalURL is the public signalhub operated by the project. Users
// who run their own server override it with --server-signaling or with
// signal_url in device.json. STUN and TURN are never configured here: the
// signaling server sends them in its first message (hello).
const DefaultSignalURL = "wss://signal.go-link.org/ws"

// DefaultWebURL is the website where viewers type the session code.
const DefaultWebURL = "https://desk.go-link.org"

// Config is persisted in device.json with 0600 permissions.
type Config struct {
	DeviceID string `json:"device_id"`
	// DeviceSecret binds the device_id to this host on signalhub (sent
	// with register): 32 random bytes, base64url. Made on first run.
	DeviceSecret string `json:"device_secret,omitempty"`
	SignalURL    string `json:"signal_url,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
	// UDPPort carries every WebRTC connection on one UDP port, so a single
	// port forward on a router opens a direct path (0: random ports).
	UDPPort int `json:"udp_port,omitempty"`
	// AnnounceIPs are addresses where browsers reach UDPPort through a
	// router that forwards it.
	AnnounceIPs []string `json:"announce_ips,omitempty"`
	// Links are the browsers linked for unattended access. They come back
	// without a code; only a hash of their secret is kept.
	Links []Link `json:"links,omitempty"`
	// Language of the host window: en, es or pt; empty follows the
	// computer's language.
	Language string `json:"language,omitempty"`
}

// Link is one browser linked to this host.
type Link struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"token_hash"` // hex SHA-256 of the token the browser keeps
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Terms is the version of the terms of use the browser's user accepted
	// when linking (or later), and TermsAt when the host learned it.
	Terms   string    `json:"terms,omitempty"`
	TermsAt time.Time `json:"terms_at,omitempty"`
}

// FactoryDefaults is the config after a factory reset, keeping only what
// makes this host itself and reachable: its identity (device_id,
// device_secret), its signaling server and its network settings (udp_port,
// announce_ips). Links, the website and the language go back to their
// defaults.
func (c Config) FactoryDefaults() Config {
	return Config{
		DeviceID: c.DeviceID, DeviceSecret: c.DeviceSecret, SignalURL: c.SignalURL,
		UDPPort: c.UDPPort, AnnounceIPs: c.AnnounceIPs,
	}
}

// EffectiveWebURL returns the configured website or the default.
func (c Config) EffectiveWebURL() string {
	if c.WebURL != "" {
		return c.WebURL
	}
	return DefaultWebURL
}

// EffectiveSignalURL returns the configured URL or the default.
func (c Config) EffectiveSignalURL() string {
	if c.SignalURL != "" {
		return c.SignalURL
	}
	return DefaultSignalURL
}
