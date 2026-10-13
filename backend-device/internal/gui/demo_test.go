// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDemoCode(t *testing.T) {
	for _, c := range []struct {
		body, want string
		status     int
	}{
		{`{"online":true,"code":"287 200 020"}`, "287200020", 200},
		{`{"online":false}`, "", 200},
		{`{"online":true,"code":"12"}`, "", 200},
		{`<html>`, "", 200},
		{`{"online":true,"code":"287 200 020"}`, "", 404},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		got, err := fetchDemoCode(context.Background(), srv.Client(), srv.URL)
		srv.Close()
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("%d %s: %q %v, want %q", c.status, c.body, got, err, c.want)
		}
	}
}
