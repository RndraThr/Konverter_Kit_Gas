package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/realtime"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestRealtimeStreamsEventsWithinScope(t *testing.T) {
	hub := realtime.NewHub()
	authService := &fakeAuthService{principal: auth.Principal{UserID: "field-1"}, regencyScope: auth.RegencyScope{RegencyIDs: []string{"wonogiri"}}}
	server := httptest.NewServer(NewHandler(Dependencies{Auth: authService, Realtime: hub}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	header.Set("Cookie", auth.SessionCookieName+"="+validSessionToken)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/realtime", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	var event realtime.Event
	if err := wsjson.Read(ctx, conn, &event); err != nil || event.Kind != "connected" {
		t.Fatalf("greeting = %+v err=%v", event, err)
	}
	for hub.Subscribers() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	hub.Publish(realtime.Event{Kind: "slots", ScheduleID: "other", RegencyID: "bangka"})
	hub.Publish(realtime.Event{Kind: "activities", ProgramID: "p1", RegencyID: "wonogiri"})
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatal(err)
	}
	// The out-of-scope event was skipped; the first one received is ours.
	if event.Kind != "activities" || event.RegencyID != "wonogiri" {
		t.Fatalf("event = %+v", event)
	}
}

func TestRealtimeRequiresSession(t *testing.T) {
	server := httptest.NewServer(NewHandler(Dependencies{Auth: &fakeAuthService{}, Realtime: realtime.NewHub()}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/realtime", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got resp=%v err=%v", resp, err)
	}
}
