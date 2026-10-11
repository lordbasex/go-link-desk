// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"log/slog"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// RemotesService keeps the computers this app connects to as a viewer:
// the ones linked for unattended access, and the recent sessions. Every
// change is saved and told to the listeners (the main window).
type RemotesService struct {
	save func(models.Remotes) error
	log  *slog.Logger
	now  func() time.Time

	mu        sync.Mutex
	r         models.Remotes
	listeners []func(models.Remotes)
}

// NewRemotesService starts from r; save (may be nil) persists changes.
func NewRemotesService(r models.Remotes, save func(models.Remotes) error, log *slog.Logger) *RemotesService {
	if save == nil {
		save = func(models.Remotes) error { return nil }
	}
	if log == nil {
		log = slog.Default()
	}
	return &RemotesService{r: r, save: save, log: log, now: time.Now}
}

// OnChange is told of every change.
func (s *RemotesService) OnChange(fn func(models.Remotes)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Get returns everything: linked computers favorites first, then the most
// recently used; recent sessions newest first.
func (s *RemotesService) Get() models.Remotes {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.copyLocked()
}

func (s *RemotesService) copyLocked() models.Remotes {
	out := models.Remotes{Devices: slices.Clone(s.r.Devices), Recents: slices.Clone(s.r.Recents)}
	sort.SliceStable(out.Devices, func(i, j int) bool {
		a, b := out.Devices[i], out.Devices[j]
		if a.Favorite != b.Favorite {
			return a.Favorite
		}
		return a.LastUsed.After(b.LastUsed)
	})
	return out
}

// Device returns a linked computer.
func (s *RemotesService) Device(deviceID string) (models.RemoteDevice, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.r.Devices {
		if d.DeviceID == deviceID {
			return d, true
		}
	}
	return models.RemoteDevice{}, false
}

// Link keeps a new link (it replaces one to the same computer).
func (s *RemotesService) Link(d models.RemoteDevice) {
	s.change(func(r *models.Remotes) {
		if d.SavedAt.IsZero() {
			d.SavedAt = s.now()
		}
		for i, old := range r.Devices {
			if old.DeviceID == d.DeviceID {
				d.Label, d.Favorite = old.Label, old.Favorite
				r.Devices[i] = d
				return
			}
		}
		r.Devices = append(r.Devices, d)
	})
}

// Update changes a linked computer.
func (s *RemotesService) Update(deviceID string, fn func(*models.RemoteDevice)) {
	s.change(func(r *models.Remotes) {
		for i := range r.Devices {
			if r.Devices[i].DeviceID == deviceID {
				fn(&r.Devices[i])
				if len(r.Devices[i].Thumb) > models.MaxThumb {
					r.Devices[i].Thumb = nil
				}
			}
		}
	})
}

// Forget removes a linked computer (and its recent sessions).
func (s *RemotesService) Forget(deviceID string) {
	s.change(func(r *models.Remotes) {
		r.Devices = slices.DeleteFunc(r.Devices, func(d models.RemoteDevice) bool { return d.DeviceID == deviceID })
		r.Recents = slices.DeleteFunc(r.Recents, func(x models.RecentSession) bool { return x.DeviceID == deviceID })
	})
}

// AddRecent puts a session first in the recent ones (one per code or
// computer), keeping MaxRecents.
func (s *RemotesService) AddRecent(rs models.RecentSession) {
	if rs.Code == "" && rs.DeviceID == "" {
		return
	}
	if rs.At.IsZero() {
		rs.At = s.now()
	}
	if len(rs.Thumb) > models.MaxThumb {
		rs.Thumb = nil
	}
	s.change(func(r *models.Remotes) {
		r.Recents = slices.DeleteFunc(r.Recents, func(x models.RecentSession) bool {
			same := (rs.Code != "" && x.Code == rs.Code) || (rs.DeviceID != "" && x.DeviceID == rs.DeviceID)
			if same && rs.Thumb == nil {
				rs.Thumb = x.Thumb
			}
			return same
		})
		r.Recents = append([]models.RecentSession{rs}, r.Recents...)
		if len(r.Recents) > models.MaxRecents {
			r.Recents = r.Recents[:models.MaxRecents]
		}
	})
}

// RemoveRecent forgets the recent session with that code.
func (s *RemotesService) RemoveRecent(code string) {
	s.change(func(r *models.Remotes) {
		r.Recents = slices.DeleteFunc(r.Recents, func(x models.RecentSession) bool { return x.Code == code })
	})
}

// ClearRecents forgets the recent sessions.
func (s *RemotesService) ClearRecents() {
	s.change(func(r *models.Remotes) { r.Recents = nil })
}

func (s *RemotesService) change(fn func(*models.Remotes)) {
	s.mu.Lock()
	fn(&s.r)
	err := s.save(s.r)
	out := s.copyLocked()
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	if err != nil {
		s.log.Warn("cannot save the remote computers", "err", err)
	}
	for _, fn := range listeners {
		fn(out)
	}
}
