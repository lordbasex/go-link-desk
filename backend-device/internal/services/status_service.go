// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package services holds the host's business logic.
package services

import (
	"slices"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// StatusService keeps the live device state and notifies listeners (the
// tray) when it changes. Snapshots are deep copies.
type StatusService struct {
	mu        sync.Mutex
	st        models.Status
	listeners []func(models.Status)
}

// NewStatusService starts in the "connecting" state.
func NewStatusService(deviceID, version, signalURL string) *StatusService {
	return &StatusService{st: models.Status{
		DeviceID: deviceID,
		Version:  version,
		Signal:   models.SignalStatus{URL: signalURL, State: models.SignalConnecting},
		Peers:    []models.LinkedPeer{},
	}}
}

// Snapshot returns a copy of the current state.
func (s *StatusService) Snapshot() models.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.st)
}

// OnChange registers a listener. It is called after every change, outside
// the lock, with a snapshot.
func (s *StatusService) OnChange(fn func(models.Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *StatusService) update(mutate func(st *models.Status)) {
	s.mu.Lock()
	mutate(&s.st)
	snap := clone(s.st)
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(snap)
	}
}

// SetConnected records a live signalhub link and the ICE URLs it offered.
func (s *StatusService) SetConnected(peerID string, iceURLs []string) {
	s.update(func(st *models.Status) {
		st.Signal.State = models.SignalConnected
		st.Signal.PeerID = peerID
		st.Signal.Error = ""
		st.Signal.ICEURLs = slices.Clone(iceURLs)
	})
}

// SetDisconnected clears everything tied to the old connection: the code
// and the linked peers die with it on the server side.
func (s *StatusService) SetDisconnected(reason string) {
	s.update(func(st *models.Status) {
		st.Signal.State = models.SignalDisconnected
		st.Signal.PeerID = ""
		st.Signal.SessionID = ""
		st.Signal.Error = reason
		st.Signal.ICEURLs = nil
		st.Pairing = models.PairingStatus{}
		st.Peers = []models.LinkedPeer{}
		st.Session = nil
	})
}

// SetError records a protocol error without changing the link state.
func (s *StatusService) SetError(msg string) {
	s.update(func(st *models.Status) { st.Signal.Error = msg })
}

// SetCode shows a new pairing code.
func (s *StatusService) SetCode(code string, issued, refreshes time.Time) {
	s.update(func(st *models.Status) {
		st.Pairing = models.PairingStatus{Code: code, IssuedAt: &issued, RefreshesAt: &refreshes}
		st.Signal.Error = ""
	})
}

// SetStream records what is being streamed.
func (s *StatusService) SetStream(stream models.StreamStatus) {
	s.update(func(st *models.Status) { st.Stream = stream })
}

// SetSession records the session room and its code; nil means none.
func (s *StatusService) SetSession(session *models.SessionStatus) {
	s.update(func(st *models.Status) {
		if session == nil {
			st.Session = nil
			return
		}
		v := *session
		if v.Active == nil && st.Session != nil && st.Session.RoomID == v.RoomID {
			v.Active = st.Session.Active // a new code keeps the session on
		}
		st.Session = &v
	})
}

// SetActive records the viewer in session (nil: none). It does nothing
// while the session room is closed.
func (s *StatusService) SetActive(a *models.ActiveSession) {
	s.update(func(st *models.Status) {
		if st.Session == nil {
			return
		}
		if a == nil {
			st.Session.Active = nil
			st.Session.Viewers = 0
			return
		}
		v := *a
		st.Session.Active = &v
		st.Session.Viewers = 1
	})
}

// SetRecording records what the viewer in session (peerID) saves of the
// screen: recording since (zero: not), and its screenshots.
func (s *StatusService) SetRecording(peerID string, since time.Time, shots int, last time.Time) {
	s.update(func(st *models.Status) {
		if st.Session == nil || st.Session.Active == nil || st.Session.Active.PeerID != peerID {
			return
		}
		a := *st.Session.Active
		a.RecordingSince, a.Screenshots, a.LastScreenshot = since, shots, last
		st.Session.Active = &a
	})
}

// SetSystem records hardware and usage.
func (s *StatusService) SetSystem(sys models.SystemStatus) {
	s.update(func(st *models.Status) { st.System = &sys })
}

// SetPeerLatency records the round trip to a linked browser.
func (s *StatusService) SetPeerLatency(peerID string, ms int) {
	s.update(func(st *models.Status) {
		for i := range st.Peers {
			if st.Peers[i].PeerID == peerID {
				v := ms
				st.Peers[i].LatencyMs = &v
			}
		}
	})
}

// SetUpdate records a newer go-link Desk release (nil: none).
func (s *StatusService) SetUpdate(u *models.UpdateInfo) {
	s.update(func(st *models.Status) { st.Update = u })
}

// SetSavedLinks records how many browsers are remembered.
func (s *StatusService) SetSavedLinks(n int) {
	s.update(func(st *models.Status) { st.SavedLinks = n })
}

// AddPeer records a linked browser.
func (s *StatusService) AddPeer(peerID, sessionID string, since time.Time) {
	s.update(func(st *models.Status) {
		st.Signal.SessionID = sessionID
		if !slices.ContainsFunc(st.Peers, func(p models.LinkedPeer) bool { return p.PeerID == peerID }) {
			st.Peers = append(st.Peers, models.LinkedPeer{PeerID: peerID, Since: since})
		}
	})
}

// RemovePeer forgets a browser that left.
func (s *StatusService) RemovePeer(peerID string) {
	s.update(func(st *models.Status) {
		st.Peers = slices.DeleteFunc(st.Peers, func(p models.LinkedPeer) bool { return p.PeerID == peerID })
	})
}

func clone(st models.Status) models.Status {
	st.Signal.ICEURLs = slices.Clone(st.Signal.ICEURLs)
	if st.Signal.ICEURLs == nil {
		st.Signal.ICEURLs = []string{}
	}
	st.Peers = slices.Clone(st.Peers)
	for i := range st.Peers {
		if st.Peers[i].LatencyMs != nil {
			v := *st.Peers[i].LatencyMs
			st.Peers[i].LatencyMs = &v
		}
	}
	if st.System != nil {
		sys := *st.System
		st.System = &sys
	}
	if st.Peers == nil {
		st.Peers = []models.LinkedPeer{}
	}
	if st.Session != nil {
		r := *st.Session
		if r.Active != nil {
			a := *r.Active
			r.Active = &a
		}
		st.Session = &r
	}
	if st.Pairing.IssuedAt != nil {
		t := *st.Pairing.IssuedAt
		st.Pairing.IssuedAt = &t
	}
	if st.Pairing.RefreshesAt != nil {
		t := *st.Pairing.RefreshesAt
		st.Pairing.RefreshesAt = &t
	}
	return st
}
