// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package appsock

import (
	"testing"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/viewer"
)

func TestStatusWords(t *testing.T) {
	for _, c := range []struct {
		st   viewer.Status
		word string
	}{
		{viewer.Status{Kind: viewer.StatusJoining}, "connecting"},
		{viewer.Status{Kind: viewer.StatusWaiting}, "waiting"},
		{viewer.Status{Kind: viewer.StatusPassword}, "password"},
		{viewer.Status{Kind: viewer.StatusPassword, PasswordError: "wrong"}, "wrong"},
		{viewer.Status{Kind: viewer.StatusPassword, Checking: true}, "checking"},
		{viewer.Status{Kind: viewer.StatusLive}, "live"},
		{viewer.Status{Kind: viewer.StatusReconnecting}, "reconnecting"},
		{viewer.Status{Kind: viewer.StatusRejected, Reason: models.RejectTimeout}, "timeout"},
		{viewer.Status{Kind: viewer.StatusRejected, Reason: models.RejectBusy}, "busy"},
		{viewer.Status{Kind: viewer.StatusRejected, Reason: models.RejectUnavailable}, "unavailable"},
		{viewer.Status{Kind: viewer.StatusRejected, Reason: models.RejectOff}, "off"},
		{viewer.Status{Kind: viewer.StatusRejected}, "declined"},
		{viewer.Status{Kind: viewer.StatusEnded, Reason: models.EndHost}, "ended_host"},
		{viewer.Status{Kind: viewer.StatusEnded, Reason: models.EndRevoked}, "ended_revoked"},
		{viewer.Status{Kind: viewer.StatusEnded}, "ended"},
		{viewer.Status{Kind: viewer.StatusNotFound}, "notfound"},
		{viewer.Status{Kind: viewer.StatusRateLimited}, "ratelimited"},
		{viewer.Status{Kind: viewer.StatusUnreachable}, "unreachable"},
		{viewer.Status{Kind: viewer.StatusUnlinked}, "unlinked"},
		{viewer.Status{Kind: viewer.StatusOffline}, "offline"},
		{viewer.Status{Kind: viewer.StatusUntrusted}, "untrusted"},
		{viewer.Status{Kind: viewer.StatusSelf}, "self"},
	} {
		if got := statusWord(c.st); got != c.word {
			t.Errorf("%+v: %s", c.st, got)
		}
	}
}

func TestDigits(t *testing.T) {
	if digits("113 134-323") != "113134323" {
		t.Fatal(digits("113 134-323"))
	}
}
