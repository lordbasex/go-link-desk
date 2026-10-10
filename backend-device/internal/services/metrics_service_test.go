// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

func TestDeviceStatusNeverCarriesTheCode(t *testing.T) {
	st := models.Status{
		DeviceID: "d1",
		Session:  &models.SessionStatus{RoomID: "r1", Code: "915355636", Invite: "abcdefghijklmnopqrstuv", Viewers: 1},
		Peers:    []models.LinkedPeer{{PeerID: "p1"}},
	}
	out, err := json.Marshal(NewDeviceStatusMessage(st))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"session":{"room_id":"r1","viewers":1}`, `"linked_browsers":1`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("device_status lacks %s: %s", want, out)
		}
	}
	for _, secret := range []string{"915355636", "abcdefghijklmnopqrstuv"} {
		if strings.Contains(string(out), secret) {
			t.Errorf("device_status leaks %s: %s", secret, out)
		}
	}
}
