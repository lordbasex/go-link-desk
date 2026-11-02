// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

type shown struct {
	mu sync.Mutex
	p  []*ConsentPrompt
	c  chan *ConsentPrompt
}

func newShown() *shown { return &shown{c: make(chan *ConsentPrompt, 8)} }
func (s *shown) show(p *ConsentPrompt) {
	s.mu.Lock()
	s.p = append(s.p, p)
	s.mu.Unlock()
	s.c <- p
}

func asked(c *SocketConsent, ctx context.Context) chan error {
	out := make(chan error, 1)
	go func() {
		_, err := c.Ask(ctx, ConsentRequest{Knock: models.Knock{Name: "Ana", Browser: "Chrome"}})
		out <- err
	}()
	return out
}

func TestSocketConsentAnswered(t *testing.T) {
	s := newShown()
	c := &SocketConsent{Show: s.show}
	got := make(chan models.Permissions, 1)
	go func() {
		p, err := c.Ask(context.Background(), ConsentRequest{Knock: models.Knock{Name: "Ana"}})
		if err != nil {
			t.Error(err)
		}
		got <- p
	}()
	p := <-s.c
	if p.Name != "Ana" || p.Deadline <= time.Now().UnixMilli() {
		t.Fatalf("%+v", p)
	}
	if c.Answer("other", true, models.ViewOnly) {
		t.Fatal("an answer to another request counted")
	}
	if !c.Answer(p.ID, true, models.Permissions{View: true, Files: true}) {
		t.Fatal("not taken")
	}
	if v := <-got; !v.Files {
		t.Fatal("lost the permissions")
	}
	if <-s.c != nil {
		t.Fatal("the prompt stayed up")
	}
}

func TestSocketConsentDeclined(t *testing.T) {
	s := newShown()
	c := &SocketConsent{Show: s.show}
	e := asked(c, context.Background())
	c.Answer((<-s.c).ID, false, models.Permissions{})
	if err := <-e; !errors.Is(err, ErrDeclined) {
		t.Fatal(err)
	}
}

// The app gone while it was being asked, and after: never a default.
func TestSocketConsentAppGoneIsRefused(t *testing.T) {
	s := newShown()
	c := &SocketConsent{Show: s.show}
	e := asked(c, context.Background())
	<-s.c
	c.Gone()
	if err := <-e; !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := <-asked(c, context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	c.Gone() // twice is fine
}

func TestSocketConsentTimeOut(t *testing.T) {
	s := newShown()
	c := &SocketConsent{Show: s.show}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	e := asked(c, ctx)
	p := <-s.c
	if err := <-e; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if c.Answer(p.ID, true, models.ViewOnly) {
		t.Fatal("a late answer counted")
	}
}

func TestSocketConsentReadyTurnsDownBeforeAsking(t *testing.T) {
	s := newShown()
	c := &SocketConsent{Show: s.show, Ready: func() error { return ErrUnavailable }}
	if err := <-asked(c, context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if len(s.p) != 0 {
		t.Fatal("the person was bothered")
	}
}

func TestSocketConsentWarnsOnce(t *testing.T) {
	s := newShown()
	seen := false
	c := &SocketConsent{Show: s.show, Warned: func() bool { return seen }, SetWarned: func() { seen = true }}
	for _, want := range []bool{true, false} {
		e := asked(c, context.Background())
		p := <-s.c
		if p.Warning != want {
			t.Fatalf("warning %v", p.Warning)
		}
		c.Answer(p.ID, false, models.Permissions{})
		<-e
		<-s.c
	}
}

func TestSocketCallConsent(t *testing.T) {
	asks := make(chan string, 4)
	c := &SocketCallConsent{Show: func(n string, _ bool) { asks <- n }}
	got := make(chan CallAnswer, 1)
	go func() {
		a, _ := c.AskCall(context.Background(), "Ana", true)
		got <- a
	}()
	if <-asks != "Ana" {
		t.Fatal("not asked")
	}
	c.Answer(CallAnswer{Accept: true, Mic: true})
	if a := <-got; !a.Accept || !a.Mic {
		t.Fatalf("%+v", a)
	}
	<-asks
	c.Gone()
	if _, err := c.AskCall(context.Background(), "Ana", true); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
