package realtime

import (
	"testing"
	"time"

	"konkit/internal/auth"
)

func receive(t *testing.T, s *Subscriber) (Event, bool) {
	t.Helper()
	select {
	case e := <-s.Events():
		return e, true
	case <-time.After(50 * time.Millisecond):
		return Event{}, false
	}
}

func TestHubDeliversOnlyWithinRegencyScope(t *testing.T) {
	hub := NewHub()
	wonogiri := hub.Subscribe(auth.RegencyScope{RegencyIDs: []string{"wonogiri"}})
	admin := hub.Subscribe(auth.RegencyScope{Unrestricted: true})
	outsider := hub.Subscribe(auth.RegencyScope{RegencyIDs: []string{"bangka"}})

	hub.Publish(Event{Kind: "slots", ScheduleID: "s1", RegencyID: "wonogiri"})

	if e, ok := receive(t, wonogiri); !ok || e.ScheduleID != "s1" {
		t.Fatalf("regency subscriber got %+v ok=%v", e, ok)
	}
	if _, ok := receive(t, admin); !ok {
		t.Fatal("unrestricted subscriber missed the event")
	}
	if e, ok := receive(t, outsider); ok {
		t.Fatalf("other regency received %+v", e)
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe(auth.RegencyScope{Unrestricted: true})
	hub.Unsubscribe(s)
	hub.Unsubscribe(s) // second call is a no-op
	if _, open := <-s.Events(); open {
		t.Fatal("channel still open after unsubscribe")
	}
	if hub.Subscribers() != 0 {
		t.Fatalf("subscribers = %d", hub.Subscribers())
	}
	hub.Publish(Event{Kind: "slots", RegencyID: "x"}) // no panic on closed subscribers
}

func TestHubDropsWhenSubscriberIsFull(t *testing.T) {
	hub := NewHub()
	s := hub.Subscribe(auth.RegencyScope{Unrestricted: true})
	for range 200 {
		hub.Publish(Event{Kind: "slots", RegencyID: "x"})
	}
	if got := len(s.events); got != cap(s.events) {
		t.Fatalf("buffered %d, want full buffer %d without blocking", got, cap(s.events))
	}
}
