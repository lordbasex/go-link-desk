// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"fyne.io/fyne/v2"

	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
)

// The demonstration computer (deploy/demo): its current code, and the
// page that says when it is reconnecting.
const (
	demoCodeURL = "https://desk.go-link.org/demo.json"
	demoPageURL = "https://desk.go-link.org/demo"
)

// tryDemo is Try the demo: a session to go-link Desk's demonstration
// computer with its current code (it changes whenever that computer
// reconnects), or its page when it cannot be read.
func (u *ui) tryDemo() {
	ctx, cancel := context.WithTimeout(u.opts.Ctx, 10*time.Second)
	defer cancel()
	code, err := fetchDemoCode(ctx, http.DefaultClient, demoCodeURL)
	if err != nil {
		u.opts.Logger.Warn("the demo computer is not available", "err", err)
		if page, perr := url.Parse(demoPageURL); perr == nil {
			fyne.Do(func() { _ = u.app.OpenURL(page) })
		}
		return
	}
	u.connect(viewer.Door{Kind: viewer.DoorJoin, Code: code})
}

// fetchDemoCode reads demo.json: the code's 9 digits while the demo
// computer is online.
func fetchDemoCode(ctx context.Context, client *http.Client, from string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("demo: %s", resp.Status)
	}
	var f struct {
		Online bool   `json:"online"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&f); err != nil {
		return "", err
	}
	code := digits(f.Code)
	if !f.Online || len(code) != 9 {
		return "", errors.New("demo: offline")
	}
	return code, nil
}
