// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"log/slog"
	"sync"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// SettingsService holds the host's configuration while it runs: the
// settings window changes it, the services read it, and every change is
// written to device.json.
type SettingsService struct {
	save func(models.Config) error
	log  *slog.Logger

	mu        sync.Mutex
	cfg       models.Config
	listeners []func(models.Config)
}

// NewSettingsService starts from cfg; save (may be nil) persists changes.
func NewSettingsService(cfg models.Config, save func(models.Config) error, log *slog.Logger) *SettingsService {
	if save == nil {
		save = func(models.Config) error { return nil }
	}
	if log == nil {
		log = slog.Default()
	}
	return &SettingsService{cfg: cfg, save: save, log: log}
}

// Get returns the configuration now.
func (s *SettingsService) Get() models.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update changes the configuration, saves it and tells the listeners.
func (s *SettingsService) Update(change func(*models.Config)) error {
	s.mu.Lock()
	change(&s.cfg)
	if s.cfg.IdleMinutes < 0 || s.cfg.IdleMinutes > models.MaxIdleMinutes {
		s.cfg.IdleMinutes = 0
	}
	cfg := s.cfg
	err := s.save(cfg)
	listeners := append([]func(models.Config){}, s.listeners...)
	s.mu.Unlock()
	if err != nil {
		s.log.Warn("cannot save the settings", "err", err)
	}
	for _, fn := range listeners {
		fn(cfg)
	}
	return err
}

// OnChange registers fn, told after every change.
func (s *SettingsService) OnChange(fn func(models.Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// UnattendedOn reports whether linked browsers may come in.
func (s *SettingsService) UnattendedOn() bool {
	return s.Get().Unattended.Enabled
}
