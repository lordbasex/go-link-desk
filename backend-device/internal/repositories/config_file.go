// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package repositories persists the device configuration.
package repositories

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/ids"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

// ConfigFile stores models.Config as JSON. The file holds no secrets yet,
// but it is private to the user (0600) because later phases may add some.
type ConfigFile struct {
	path string
}

// DefaultConfigPath returns <user config dir>/go-link-desk/device.json,
// for example ~/Library/Application Support/go-link-desk/device.json on
// macOS or %AppData%\go-link-desk\device.json on Windows.
func DefaultConfigPath() (string, error) {
	return ConfigPathFor("go-link-desk")
}

// ConfigPathFor returns <user config dir>/<folder>/device.json: another
// program of go-link Desk (go-link-desk-terminal) keeps an identity, links
// and settings of its own beside the app's.
func ConfigPathFor(folder string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, folder, "device.json"), nil
}

// NewConfigFile uses the file at path.
func NewConfigFile(path string) *ConfigFile {
	return &ConfigFile{path: path}
}

// Path returns the file location.
func (f *ConfigFile) Path() string { return f.path }

// newDeviceSecret returns 32 random bytes in unpadded base64url.
func newDeviceSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func validDeviceSecret(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == 32
}

// newHostKey returns the stored form of a fresh device signing key.
func newHostKey() string {
	k, err := linkauth.NewKey()
	if err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return k.Private()
}

func validHostKey(s string) bool {
	_, err := linkauth.ParseKey(s)
	return err == nil
}

// dropLegacyLinks keeps only links that carry a browser public key. Links
// from before the key-based handshake had only a token hash and cannot
// authenticate any more, so they are forgotten.
func dropLegacyLinks(links []models.Link) []models.Link {
	kept := make([]models.Link, 0, len(links))
	for _, l := range links {
		if _, err := linkauth.ParsePublic(l.BrowserKey); err == nil {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// LoadOrCreate reads the configuration. On first run it creates the file
// with a new permanent device_id and reports created=true.
func (f *ConfigFile) LoadOrCreate() (cfg models.Config, created bool, err error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		cfg = models.Config{DeviceID: ids.NewUUIDv4(), DeviceSecret: newDeviceSecret(), HostKey: newHostKey()}
		if err := f.Save(cfg); err != nil {
			return models.Config{}, false, err
		}
		return cfg, true, nil
	}
	if err != nil {
		return models.Config{}, false, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return models.Config{}, false, fmt.Errorf("parse %s: %w", f.path, err)
	}
	if !ids.ValidUUID(cfg.DeviceID) {
		return models.Config{}, false, fmt.Errorf("parse %s: invalid device_id %q", f.path, cfg.DeviceID)
	}
	// Configurations made before the device secret or the device key existed
	// get them now. Links made before the key-based handshake kept only a
	// token hash and can no longer come back, so they are dropped and their
	// browsers link again with a code.
	resave := false
	if !validDeviceSecret(cfg.DeviceSecret) {
		cfg.DeviceSecret = newDeviceSecret()
		resave = true
	}
	if !validHostKey(cfg.HostKey) {
		cfg.HostKey = newHostKey()
		resave = true
	}
	if kept := dropLegacyLinks(cfg.Links); len(kept) != len(cfg.Links) {
		cfg.Links = kept
		resave = true
	}
	if resave {
		if err := f.Save(cfg); err != nil {
			return models.Config{}, false, err
		}
	}
	// Tighten permissions if someone loosened them.
	if info, err := os.Stat(f.path); err == nil && info.Mode().Perm() != 0o600 {
		_ = os.Chmod(f.path, 0o600)
	}
	return cfg, false, nil
}

// Save writes the configuration atomically: a temporary file is written
// and renamed over the old one, so a crash never leaves half a file.
func (f *ConfigFile) Save(cfg models.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivate(f.path, append(data, '\n')); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// writePrivate writes a file only its owner reads, atomically: a
// temporary file is written and renamed over the old one, so a crash never
// leaves half a file.
func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
