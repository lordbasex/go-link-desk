// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package repositories

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

func TestFirstRunCreatesPersistentID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "device.json")
	f := NewConfigFile(path)

	cfg, created, err := f.LoadOrCreate()
	if err != nil || !created || cfg.DeviceID == "" {
		t.Fatalf("first run: %+v %v %v", cfg, created, err)
	}
	again, created, err := f.LoadOrCreate()
	if err != nil || created || again.DeviceID != cfg.DeviceID {
		t.Fatalf("second run: %+v %v %v", again, created, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", info.Mode().Perm())
		}
	}
}

func TestSaveRoundTrip(t *testing.T) {
	f := NewConfigFile(filepath.Join(t.TempDir(), "device.json"))
	cfg, _, _ := f.LoadOrCreate()
	cfg.SignalURL = "wss://signal.example/ws"
	cfg.WebURL = "https://web.example"
	cfg.Language = "es"
	now := time.Now().UTC().Truncate(time.Second)
	key, err := linkauth.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Links = []models.Link{{ID: "3f9c0a1b2c3d4e5f", BrowserKey: key.Public(), CreatedAt: now, LastSeen: now}}
	if err := f.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.LoadOrCreate()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got %+v %v", got, err)
	}
	if got.EffectiveSignalURL() != cfg.SignalURL {
		t.Fatalf("effective values: %+v", got)
	}
	raw, err := os.ReadFile(f.Path())
	if err != nil || !strings.Contains(string(raw), `"language": "es"`) {
		t.Fatalf("device.json lacks the language: %s %v", raw, err)
	}
}

func TestFirstRunMakesADeviceKeyAndDropsLegacyLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	f := NewConfigFile(path)
	cfg, created, err := f.LoadOrCreate()
	if err != nil || !created || !validHostKey(cfg.HostKey) {
		t.Fatalf("first run: created=%v host_key valid=%v err=%v", created, validHostKey(cfg.HostKey), err)
	}
	// Write a config whose link has only the old token hash and no device key.
	legacy := `{"device_id":"` + cfg.DeviceID + `","links":[{"id":"abc","token_hash":"dead"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Links) != 0 {
		t.Fatalf("a legacy link survived: %+v", got.Links)
	}
	if !validHostKey(got.HostKey) {
		t.Fatal("a config without a device key did not get one")
	}
	// The cleaned, keyed config is written back.
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "token_hash") || !strings.Contains(string(raw), "host_key") {
		t.Fatalf("device.json not rewritten: %s", raw)
	}
}

func TestLooseModeIsTightened(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	path := filepath.Join(t.TempDir(), "device.json")
	f := NewConfigFile(path)
	_, _, _ = f.LoadOrCreate()
	_ = os.Chmod(path, 0o644)
	_, _, _ = f.LoadOrCreate()
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestCorruptConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	for _, content := range []string{"{not json", `{"device_id":"nope"}`} {
		_ = os.WriteFile(path, []byte(content), 0o600)
		if _, _, err := NewConfigFile(path).LoadOrCreate(); err == nil {
			t.Errorf("accepted %q", content)
		}
	}
}
