// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"testing"
)

// A public demonstration host never gives the clipboard or files.
func TestAutoAcceptDemoKeepsTheClipboardAndFiles(t *testing.T) {
	p, err := AutoAcceptDemo.Ask(context.Background(), ConsentRequest{})
	if err != nil || !p.View || !p.Control || !p.Audio || p.Clipboard || p.Files {
		t.Fatalf("%+v %v", p, err)
	}
}
