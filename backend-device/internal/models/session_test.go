// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package models

import (
	"strings"
	"testing"
)

func TestCleanChatKeepsLinesDropsControls(t *testing.T) {
	got := CleanChat("  hola‮\n\tmundo  \n" + strings.Repeat("x", MaxChat))
	if !strings.HasPrefix(got, "hola\nmundo\n") || len([]rune(got)) > MaxChat {
		t.Fatalf("%q… (%d)", got[:min(20, len(got))], len([]rune(got)))
	}
	if CleanChat(strings.Repeat("a\n", 50)) != strings.TrimSpace(strings.Repeat("a\n", 20)) {
		t.Fatal("more than 20 lines kept")
	}
}
