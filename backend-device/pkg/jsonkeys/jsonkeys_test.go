// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package jsonkeys

import "testing"

func TestDistinct(t *testing.T) {
	cases := []struct {
		name, json string
		want       bool
	}{
		{"plain", `{"type":"rom_test","id":"a","set":"s"}`, true},
		{"nested and arrays", `{"type":"create_room","maker":{"title":"x","labels":["a","b"]},"n":[{"a":1},{"a":2}]}`, true},
		{"scalar", `"x"`, true},
		{"same key in another case", `{"type":"rom_test","TYPE":"factory_reset"}`, false},
		{"exact duplicate", `{"type":"a","type":"b"}`, false},
		{"nested duplicate", `{"type":"create_room","maker":{"title":"x","Title":"y"}}`, false},
		{"duplicate inside an array", `{"a":[{"k":1,"K":2}]}`, false},
		{"invalid", `{"type":`, false},
		{"trailing data", `{"a":1} {"b":2}`, false},
	}
	for _, c := range cases {
		if got := Distinct([]byte(c.json)); got != c.want {
			t.Errorf("%s: Distinct(%s) = %v, want %v", c.name, c.json, got, c.want)
		}
	}
}

func TestDistinctDepth(t *testing.T) {
	deep := ""
	for i := 0; i < 100; i++ {
		deep += `{"a":`
	}
	deep += "1"
	for i := 0; i < 100; i++ {
		deep += "}"
	}
	if Distinct([]byte(deep)) {
		t.Error("a message nested 100 deep passed")
	}
}
