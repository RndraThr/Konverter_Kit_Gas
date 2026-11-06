package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"konkit/internal/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type senderStub struct {
	sent []Push
	err  map[string]error
}

func (s *senderStub) Send(_ context.Context, push Push) error {
	if err := s.err[push.Token]; err != nil {
		return err
	}
	s.sent = append(s.sent, push)
	return nil
}

func testUser(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	suffix := uuid.NewString()[:8]
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO users (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id::text`, "notif"+suffix, "notif"+suffix+"@test.local").Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, id) })
	return pool, id
}

func add(t *testing.T, pool *pgxpool.Pool, userID, category string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Enqueue(ctx, tx, []string{userID, userID, ""}, Message{Category: category, Title: "Judul", Body: "Isi", Link: "/keuangan/1"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func state(t *testing.T, pool *pgxpool.Pool, userID string) (string, int) {
	t.Helper()
	var value string
	var attempts int
	if err := pool.QueryRow(context.Background(), `SELECT push_state, push_attempts FROM notifications WHERE user_id = $1::uuid ORDER BY created_at DESC LIMIT 1`, userID).Scan(&value, &attempts); err != nil {
		t.Fatal(err)
	}
	return value, attempts
}

func TestIntegrationPushHonorsSettingsAndDropsDeadDevices(t *testing.T) {
	pool, user := testUser(t)
	ctx := context.Background()
	sender := &senderStub{err: map[string]error{"dead": ErrTokenGone}}
	service := NewService(pool, sender)
	if err := service.RegisterDevice(ctx, user, "alive", ""); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterDevice(ctx, user, "dead", "android"); err != nil {
		t.Fatal(err)
	}

	add(t, pool, user, CategoryReview)
	if _, err := service.pushBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0].Token != "alive" || sender.sent[0].Link != "/keuangan/1" {
		t.Fatalf("sent=%+v", sender.sent)
	}
	if got, _ := state(t, pool, user); got != "sent" {
		t.Fatalf("state=%s", got)
	}
	tokens, err := service.tokens(ctx, user)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("tokens=%v err=%v", tokens, err)
	}

	// A muted category stays in the inbox but sends no push.
	if _, err := service.SetPreferences(ctx, user, map[string]bool{CategoryReview: false}); err != nil {
		t.Fatal(err)
	}
	add(t, pool, user, CategoryReview)
	if _, err := service.pushBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("muted push sent: %d", len(sender.sent))
	}
	if got, _ := state(t, pool, user); got != "skipped" {
		t.Fatalf("muted state=%s", got)
	}
	rows, unread, err := service.List(ctx, user, 10)
	if err != nil || len(rows) != 2 || unread != 2 {
		t.Fatalf("rows=%d unread=%d err=%v", len(rows), unread, err)
	}
	if _, err := service.SetPreferences(ctx, user, map[string]bool{"unknown": true}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown category: %v", err)
	}

	// Another user cannot touch this inbox.
	if err := service.MarkRead(ctx, uuid.NewString(), rows[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if err := service.MarkRead(ctx, user, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := service.MarkAllRead(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := service.List(ctx, user, 10); unread != 0 {
		t.Fatalf("unread=%d", unread)
	}
}

func TestIntegrationPushRetriesThenGivesUp(t *testing.T) {
	pool, user := testUser(t)
	ctx := context.Background()
	sender := &senderStub{err: map[string]error{"flaky": errors.New("503")}}
	service := NewService(pool, sender)
	if err := service.RegisterDevice(ctx, user, "flaky", ""); err != nil {
		t.Fatal(err)
	}
	add(t, pool, user, CategoryResult)
	for attempt := 1; attempt < maxPushAttempts; attempt++ {
		if _, err := service.pushBatch(ctx); err != nil {
			t.Fatal(err)
		}
		if got, attempts := state(t, pool, user); got != "pending" || attempts != attempt {
			t.Fatalf("attempt %d: %s %d", attempt, got, attempts)
		}
		if _, err := pool.Exec(ctx, `UPDATE notifications SET next_push_at = now() WHERE user_id = $1::uuid`, user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.pushBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := state(t, pool, user); got != "failed" {
		t.Fatalf("state=%s", got)
	}
}

func TestFCMRequestHidesContentOnTheLockScreen(t *testing.T) {
	body, err := fcmRequest(Push{Token: "t", Title: "Judul", Body: "Isi", Link: "/keuangan/1", ID: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	android := decoded["message"]["android"].(map[string]any)
	if android["priority"] != "HIGH" || android["notification"].(map[string]any)["visibility"] != "PRIVATE" {
		t.Fatalf("android=%+v", android)
	}
	if !strings.Contains(string(body), "konkit_finance") {
		t.Fatalf("no channel: %s", body)
	}
	if !fcmTokenGone(404, nil) || !fcmTokenGone(400, []byte(`{"errorCode":"UNREGISTERED"}`)) || fcmTokenGone(503, nil) {
		t.Fatal("token gone detection")
	}
}

func TestIntegrationTestPush(t *testing.T) {
	pool, user := testUser(t)
	ctx := context.Background()
	sender := &senderStub{err: map[string]error{"gone-" + user: ErrTokenGone, "bad-" + user: errors.New("down")}}
	service := NewService(pool, sender)
	for _, token := range []string{"good-" + user, "gone-" + user, "bad-" + user} {
		if err := service.RegisterDevice(ctx, user, token, "android"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.TestPush(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.Gone != 1 || result.Failed != 1 || result.Devices != 2 || !result.Configured {
		t.Fatalf("result=%+v", result)
	}
	status, err := service.Status(ctx, user)
	if err != nil || status.Devices != 2 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	var inbox int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1::uuid`, user).Scan(&inbox); err != nil || inbox != 0 {
		t.Fatalf("inbox=%d err=%v", inbox, err)
	}
	result, err = NewService(pool, nil).TestPush(ctx, user)
	if err != nil || result.Configured || result.Sent != 0 {
		t.Fatalf("no sender=%+v err=%v", result, err)
	}
}
