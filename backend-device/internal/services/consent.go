// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"errors"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// ConsentRequest is a viewer asking to come in, with what it declared.
type ConsentRequest struct {
	PeerID string
	Knock  models.Knock
}

// Consent asks the person at the host whether a viewer may come in. Ask
// blocks until they answer or ctx ends (the time to answer is over, or the
// viewer left); the dialog must close then. It returns what the viewer may
// do, or an error: ErrDeclined, ErrUnavailable or ctx's.
type Consent interface {
	Ask(ctx context.Context, req ConsentRequest) (models.Permissions, error)
}

var (
	// ErrDeclined: the person at the host said no.
	ErrDeclined = errors.New("consent: declined")
	// ErrUnavailable: nobody can answer (a host without a window).
	ErrUnavailable = errors.New("consent: nobody to ask")
)

// ConsentFunc adapts a function to Consent.
type ConsentFunc func(ctx context.Context, req ConsentRequest) (models.Permissions, error)

// Ask calls f.
func (f ConsentFunc) Ask(ctx context.Context, req ConsentRequest) (models.Permissions, error) {
	return f(ctx, req)
}

// NobodyToAsk is the consent of a host without a window: every viewer is
// turned down, because nobody is there to accept it.
var NobodyToAsk = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.Permissions{}, ErrUnavailable
})

// AutoAccept lets every viewer in, view only. It exists for the
// end-to-end tests (--test-auto-accept), which also force the test card:
// it never shows a real screen to anyone.
var AutoAccept = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.ViewOnly, nil
})

// AutoAcceptAll lets every viewer in with everything a session can give
// (the mouse and keyboard, the sound, the clipboard): measurement builds
// and the end-to-end tests that log the input instead of injecting it
// (--test-input-log).
var AutoAcceptAll = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.Permissions{View: true, Control: true, Audio: true, Clipboard: true}, nil
})
