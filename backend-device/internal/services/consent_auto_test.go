// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

package services

// consent_auto.go is not in a lordos build; its tests still use it.

import (
	"context"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// AutoAccept lets every viewer in, view only. It exists for the
// end-to-end tests (--test-auto-accept), which also force the test card:
// it never shows a real screen to anyone.
var AutoAccept = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.ViewOnly, nil
})

// AutoAcceptAll lets every viewer in with everything a session can give
// (the mouse and keyboard, the sound, the clipboard, files, recording): measurement builds
// and the end-to-end tests that log the input instead of injecting it
// (--test-input-log).
var AutoAcceptAll = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.Permissions{View: true, Control: true, Audio: true, Clipboard: true, Files: true, Record: true}, nil
})

// AutoAcceptDemo lets every viewer of a public demonstration host in
// (--test-code-file) with everything, none of it the computer's own: the
// test card, the mouse and keyboard only logged (--test-input-log), a
// clipboard kept in a file (--test-clipboard-file), and files in a folder
// of their own (--test-files-folder); nothing private to record either.
var AutoAcceptDemo = ConsentFunc(func(context.Context, ConsentRequest) (models.Permissions, error) {
	return models.Permissions{View: true, Control: true, Audio: true, Clipboard: true, Files: true, Record: true}, nil
})

// AutoAcceptCall takes every call with the camera and microphone on: the
// end-to-end tests and measurement builds.
var AutoAcceptCall = CallConsentFunc(func(context.Context, string, bool) (CallAnswer, error) {
	return CallAnswer{Accept: true, Camera: true, Mic: true}, nil
})
