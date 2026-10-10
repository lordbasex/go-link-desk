// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package ids

import (
	"regexp"
	"testing"
)

func TestNew(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(New()) {
		t.Fatal("bad format")
	}
	if New() == New() {
		t.Fatal("not random")
	}
}

func TestNewUUIDv4(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		if id := NewUUIDv4(); !v4.MatchString(id) || !ValidUUID(id) {
			t.Fatalf("bad uuid %q", id)
		}
	}
	if ValidUUID("nope") {
		t.Fatal("accepted garbage")
	}
}
