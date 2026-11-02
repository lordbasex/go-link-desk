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
	// Link: the viewer came with the pairing code and asks to be linked
	// for unattended access; the permissions accepted become its link's.
	Link bool
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
	// ErrInUse: the session is taken by another door, so nobody is asked.
	ErrInUse = errors.New("consent: the session is in use")
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
