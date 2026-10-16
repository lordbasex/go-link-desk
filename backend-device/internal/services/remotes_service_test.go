// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestRemotesKeepFavoritesFirstAndRecentsBounded(t *testing.T) {
	var saved models.Remotes
	s := NewRemotesService(models.Remotes{}, func(r models.Remotes) error { saved = r; return nil }, nil)
	t0 := time.Unix(1_700_000_000, 0)
	s.Link(models.RemoteDevice{DeviceID: "a", Name: "office", LastUsed: t0})
	s.Link(models.RemoteDevice{DeviceID: "b", Name: "home", LastUsed: t0.Add(time.Hour)})
	s.Update("a", func(d *models.RemoteDevice) { d.Favorite = true; d.Label = "Office iMac" })
	got := s.Get().Devices
	if got[0].DeviceID != "a" || got[0].Title() != "Office iMac" || got[1].Title() != "home" {
		t.Fatalf("%+v", got)
	}
	// A new link to the same computer keeps its label and favorite.
	s.Link(models.RemoteDevice{DeviceID: "a", LinkID: "new", Key: "t2"})
	if d, _ := s.Device("a"); d.LinkID != "new" || d.Label != "Office iMac" || !d.Favorite {
		t.Fatalf("%+v", d)
	}

	for i := 0; i < models.MaxRecents+5; i++ {
		s.AddRecent(models.RecentSession{Code: string(rune('a' + i))})
	}
	s.AddRecent(models.RecentSession{Code: "c", Thumb: []byte{1}})
	s.AddRecent(models.RecentSession{Code: "c"})
	r := s.Get().Recents
	if len(r) != models.MaxRecents || r[0].Code != "c" || len(r[0].Thumb) != 1 {
		t.Fatalf("%d recents, first %+v", len(r), r[0])
	}
	s.AddRecent(models.RecentSession{DeviceID: "b"})
	s.Forget("b")
	if _, ok := s.Device("b"); ok || saved.Recents[0].DeviceID == "b" {
		t.Fatal("b not forgotten")
	}
}
