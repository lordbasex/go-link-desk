// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"fmt"
	"os"
	"strings"

	"github.com/jeandeaual/go-locale"
)

// Languages of the window, in the order Settings lists them. "" follows
// the computer's language.
var languages = []struct{ id, label string }{
	{"", "Automatic"},
	{"en", "English"},
	{"es", "Español"},
	{"pt", "Português"},
}

// catalogs translate the window's English texts, keyed by the English
// text itself (i18n_es.go, i18n_pt.go). A missing entry shows English.
var catalogs = map[string]map[string]string{}

// current is the window's language. It is only read and written on
// Fyne's thread.
var current = "en"

// setLanguage picks the window's language: en, es, pt or "" (the
// computer's own, English when it is none of them).
func setLanguage(id string) {
	if id == "" {
		id = systemLanguage()
	}
	if _, ok := catalogs[id]; !ok && id != "en" {
		id = "en"
	}
	current = id
}

func systemLanguage() string {
	if v := os.Getenv("GO_LINK_DESK_LANG"); v != "" {
		return v
	}
	if tags, err := locale.GetLocales(); err == nil {
		for _, t := range tags {
			l := strings.ToLower(strings.SplitN(strings.SplitN(t, "-", 2)[0], "_", 2)[0])
			if l == "en" || catalogs[l] != nil {
				return l
			}
		}
	}
	return "en"
}

// L translates an English text of the window.
func L(s string) string {
	if t, ok := catalogs[current][s]; ok {
		return t
	}
	return s
}

// Lf translates an English format and fills it in.
func Lf(format string, a ...any) string { return fmt.Sprintf(L(format), a...) }
