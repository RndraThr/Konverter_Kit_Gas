// Package realtime pushes change signals to connected mobile apps (like
// aergas-mobile's Reverb WebSocket). PostgreSQL triggers NOTIFY on channel
// konkit_changes (migration 00050); the Listener fans each signal out through
// the Hub to the WebSocket clients allowed to see that regency. Clients then
// pull the delta through the normal sync endpoints, so a signal never carries
// data. Single process: no Redis needed (a multi-instance deployment would
// still work, since every instance LISTENs to the same database).
package realtime

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"sync"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// Channel is the PostgreSQL NOTIFY channel written by the change triggers.
const Channel = "konkit_changes"

// Event is one change signal, as sent to the app.
type Event struct {
	// Kind is "slots", "candidates" or "activities".
	Kind       string `json:"t"`
	ScheduleID string `json:"schedule_id,omitempty"`
	ProgramID  string `json:"program_id,omitempty"`
	RegencyID  string `json:"regency_id,omitempty"`
}

// Subscriber receives the events its regency scope allows.
type Subscriber struct {
	scope  auth.RegencyScope
	events chan Event
}

// Events delivers signals; it is closed when the subscriber is removed.
func (s *Subscriber) Events() <-chan Event { return s.events }

func (s *Subscriber) allows(e Event) bool {
	return s.scope.Unrestricted || slices.Contains(s.scope.RegencyIDs, e.RegencyID)
}

// Hub fans events out to subscribers.
type Hub struct {
	mu          sync.Mutex
	subscribers map[*Subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: map[*Subscriber]struct{}{}}
}

// Subscribe registers a client limited to scope.
func (h *Hub) Subscribe(scope auth.RegencyScope) *Subscriber {
	s := &Subscriber{scope: scope, events: make(chan Event, 64)}
	h.mu.Lock()
	h.subscribers[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes the client and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	if _, ok := h.subscribers[s]; ok {
		delete(h.subscribers, s)
		close(s.events)
	}
	h.mu.Unlock()
}

// Publish delivers e to every subscriber allowed to see it. A subscriber
// whose buffer is full misses the signal; it catches up with a delta sync
// on its next signal or reconnect.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subscribers {
		if !s.allows(e) {
			continue
		}
		select {
		case s.events <- e:
		default:
		}
	}
}

// Subscribers is the number of connected clients.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

// Listen LISTENs on Channel with its own connection and publishes every
// notification until ctx ends, reconnecting after errors.
func Listen(ctx context.Context, databaseURL string, hub *Hub) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := listenOnce(ctx, databaseURL, hub)
		if ctx.Err() != nil {
			return
		}
		log.Printf("realtime: listener stopped (%v); reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func listenOnce(ctx context.Context, databaseURL string, hub *Hub) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var event Event
		if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil || event.Kind == "" {
			continue
		}
		hub.Publish(event)
	}
}
