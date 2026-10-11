// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// RemotesFile keeps the computers this app connects to (remotes.json,
// next to device.json, 0600: it holds the links' tokens).
type RemotesFile struct {
	path string
}

// NewRemotesFile keeps them in remotes.json in configDir.
func NewRemotesFile(configDir string) *RemotesFile {
	return &RemotesFile{path: filepath.Join(configDir, "remotes.json")}
}

// Path is where the file is.
func (f *RemotesFile) Path() string { return f.path }

// Load reads the file; a missing one is empty.
func (f *RemotesFile) Load() (models.Remotes, error) {
	var r models.Remotes
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return models.Remotes{}, fmt.Errorf("read %s: %w", f.path, err)
	}
	return r, nil
}

// Save writes it atomically.
func (f *RemotesFile) Save(r models.Remotes) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writePrivate(f.path, append(data, '\n')); err != nil {
		return fmt.Errorf("save remotes: %w", err)
	}
	return nil
}
