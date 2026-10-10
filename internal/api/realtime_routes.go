package api

import (
	"context"
	"net/http"
	"time"

	"konkit/internal/realtime"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	realtimePingInterval = 25 * time.Second
	realtimeWriteTimeout = 10 * time.Second
)

// handleRealtime upgrades to a WebSocket that pushes change signals
// ({"t":"slots"|"candidates"|"activities", ...}) for the account's regencies.
// The app pulls the delta on each signal and runs a full delta sync after
// every (re)connect, like aergas-mobile's Reverb connection.
func (h *Handler) handleRealtime(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Realtime == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	// The app sends no Origin header; browsers are checked against Host.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	subscriber := h.deps.Realtime.Subscribe(scope)
	defer h.deps.Realtime.Unsubscribe(subscriber)
	// Incoming messages are not used; CloseRead answers pings and ends ctx on close.
	ctx := conn.CloseRead(r.Context())

	write := func(v any) error {
		writeCtx, cancel := context.WithTimeout(ctx, realtimeWriteTimeout)
		defer cancel()
		return wsjson.Write(writeCtx, conn, v)
	}
	if write(realtime.Event{Kind: "connected"}) != nil {
		return
	}
	ticker := time.NewTicker(realtimePingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-subscriber.Events():
			if !open || write(event) != nil {
				return
			}
		case <-ticker.C:
			// A signed-out or revoked session loses the stream too.
			if _, err := h.deps.Auth.Authenticate(ctx, rc.token); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "session ended")
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, realtimeWriteTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
