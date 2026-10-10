// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package jsonkeys guards against keys that differ only in case. Go's
// encoding/json matches object keys to struct fields in any case and keeps
// the last one, so {"type":"rom_test","TYPE":"factory_reset"} decodes as
// factory_reset: a filter that checked "type" elsewhere (in JavaScript,
// where keys are case-sensitive) would be bypassed.
package jsonkeys

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Distinct reports whether data is JSON in which no object, at any depth,
// has two keys that are equal ignoring case. Invalid JSON is not distinct.
func Distinct(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if !value(dec, 0) {
		return false
	}
	_, err := dec.Token()
	return err != nil // nothing after the value
}

// maxDepth bounds nesting, so a hostile message cannot recurse deeply.
const maxDepth = 32

func value(dec *json.Decoder, depth int) bool {
	if depth > maxDepth {
		return false
	}
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return false
			}
			k, ok := key.(string)
			if !ok {
				return false
			}
			low := strings.ToLower(k)
			if seen[low] {
				return false
			}
			seen[low] = true
			if !value(dec, depth+1) {
				return false
			}
		}
		_, err := dec.Token() // }
		return err == nil
	case json.Delim('['):
		for dec.More() {
			if !value(dec, depth+1) {
				return false
			}
		}
		_, err := dec.Token() // ]
		return err == nil
	}
	return true
}
