// Package notifications keeps each user's inbox, their push settings and
// devices, and sends pushes through FCM so they arrive with the app closed.
// A row in notifications also signals the user's open clients (realtime).
package notifications

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Categories a user can mute for pushes. The inbox always keeps every message.
const (
	CategoryReview   = "finance_review"
	CategoryResult   = "finance_result"
	CategoryPayment  = "finance_payment"
	CategoryReminder = "finance_reminder"
)

var Categories = []string{CategoryReview, CategoryResult, CategoryPayment, CategoryReminder}

const (
	maxPushAttempts = 5
	pushBatch       = 50
	pushIdleWait    = 30 * time.Second
)

var (
	ErrNotFound     = errors.New("notification not found")
	ErrInvalidInput = errors.New("notification input is invalid")
	// ErrTokenGone: FCM no longer knows the device; its token is dropped.
	ErrTokenGone = errors.New("device token is no longer registered")
)

type Notification struct {
	ID        string     `json:"id"`
	Category  string     `json:"category"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Message is one notification for one or more users.
type Message struct {
	Category string
	Title    string
	Body     string
	Link     string
}

// Push is what a Sender delivers to one device.
type Push struct {
	Token string
	Title string
	Body  string
	Link  string
	ID    string
}

// PushStatus tells the app whether this server can push and how many phones
// the user has registered.
type PushStatus struct {
	Configured bool `json:"configured"`
	Devices    int  `json:"devices"`
}

// TestResult is how FCM answered a test push. Sent means FCM accepted it, not
// that the phone showed it.
type TestResult struct {
	PushStatus
	Sent   int `json:"sent"`
	Gone   int `json:"gone"`
	Failed int `json:"failed"`
}

// Sender delivers one push; ErrTokenGone drops the device.
type Sender interface {
	Send(ctx context.Context, push Push) error
}

type Service struct {
	pool   *pgxpool.Pool
	sender Sender
	wake   chan struct{}
}

// NewService: a nil sender keeps the inbox and realtime, without pushes.
func NewService(pool *pgxpool.Pool, sender Sender) *Service {
	return &Service{pool: pool, sender: sender, wake: make(chan struct{}, 1)}
}

// Enqueue adds message for users inside the caller's transaction, so it
// exists only if the change it reports was committed. Call Wake afterwards.
func Enqueue(ctx context.Context, tx pgx.Tx, userIDs []string, message Message) error {
	seen := map[string]bool{}
	for _, id := range userIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.Exec(ctx, `INSERT INTO notifications (user_id, category, title, body, link) VALUES ($1::uuid, $2, $3, $4, $5)`,
			id, message.Category, message.Title, message.Body, message.Link); err != nil {
			return fmt.Errorf("add notification: %w", err)
		}
	}
	return nil
}

// Wake makes the push worker look for new work now.
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) List(ctx context.Context, userID string, limit int) ([]Notification, int, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, category, title, body, link, read_at, created_at
		FROM notifications WHERE user_id = $1::uuid ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	result := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Category, &n.Title, &n.Body, &n.Link, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		result = append(result, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	var unread int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND read_at IS NULL`, userID).Scan(&unread); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}
	return result, unread, nil
}

func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1::uuid AND user_id = $2::uuid`, id, userID)
	if err != nil {
		return fmt.Errorf("read notification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1::uuid AND read_at IS NULL`, userID); err != nil {
		return fmt.Errorf("read notifications: %w", err)
	}
	return nil
}

// Preferences returns the push setting of every category.
func (s *Service) Preferences(ctx context.Context, userID string) (map[string]bool, error) {
	result := map[string]bool{}
	for _, category := range Categories {
		result[category] = true
	}
	rows, err := s.pool.Query(ctx, `SELECT category, push_enabled FROM notification_preferences WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, fmt.Errorf("list notification preferences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var category string
		var enabled bool
		if err := rows.Scan(&category, &enabled); err != nil {
			return nil, fmt.Errorf("scan notification preference: %w", err)
		}
		if _, known := result[category]; known {
			result[category] = enabled
		}
	}
	return result, rows.Err()
}

func (s *Service) SetPreferences(ctx context.Context, userID string, values map[string]bool) (map[string]bool, error) {
	known := map[string]bool{}
	for _, category := range Categories {
		known[category] = true
	}
	for category, enabled := range values {
		if !known[category] {
			return nil, ErrInvalidInput
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO notification_preferences (user_id, category, push_enabled) VALUES ($1::uuid, $2, $3)
			ON CONFLICT (user_id, category) DO UPDATE SET push_enabled = EXCLUDED.push_enabled`, userID, category, enabled); err != nil {
			return nil, fmt.Errorf("save notification preference: %w", err)
		}
	}
	return s.Preferences(ctx, userID)
}

// RegisterDevice ties token to the signed-in user; a phone that changes
// hands moves to its new user.
func (s *Service) RegisterDevice(ctx context.Context, userID, token, platform string) error {
	token, platform = strings.TrimSpace(token), strings.TrimSpace(platform)
	if platform == "" {
		platform = "android"
	}
	if token == "" || len(token) > 4096 || (platform != "android" && platform != "ios") {
		return ErrInvalidInput
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO device_tokens (token, user_id, platform) VALUES ($1, $2::uuid, $3)
		ON CONFLICT (token) DO UPDATE SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now()`, token, userID, platform); err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	return nil
}

// RemoveDevice stops pushes to the phone at sign-out.
func (s *Service) RemoveDevice(ctx context.Context, userID, token string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1 AND user_id = $2::uuid`, strings.TrimSpace(token), userID); err != nil {
		return fmt.Errorf("remove device: %w", err)
	}
	return nil
}

// Status reports the user's push setup.
func (s *Service) Status(ctx context.Context, userID string) (PushStatus, error) {
	tokens, err := s.tokens(ctx, userID)
	if err != nil {
		return PushStatus{}, err
	}
	return PushStatus{Configured: s.sender != nil, Devices: len(tokens)}, nil
}

// TestPush sends to each of the user's phones without touching the inbox, so
// a person can see where a push stops.
func (s *Service) TestPush(ctx context.Context, userID string) (TestResult, error) {
	status, err := s.Status(ctx, userID)
	if err != nil || !status.Configured {
		return TestResult{PushStatus: status}, err
	}
	tokens, err := s.tokens(ctx, userID)
	if err != nil {
		return TestResult{}, err
	}
	result := TestResult{PushStatus: status}
	for _, token := range tokens {
		sendErr := s.sender.Send(ctx, Push{Token: token, Title: "Tes notifikasi Konkit", Body: "Notifikasi ke HP ini berjalan.", Link: "/notifikasi", ID: "test"})
		switch {
		case sendErr == nil:
			result.Sent++
		case errors.Is(sendErr, ErrTokenGone):
			result.Gone++
			if _, err := s.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1`, token); err != nil {
				return TestResult{}, fmt.Errorf("drop device: %w", err)
			}
		default:
			result.Failed++
			log.Printf("notifications: test push: %v", sendErr)
		}
	}
	result.Devices -= result.Gone
	return result, nil
}

// RunPush sends pending pushes until ctx ends.
func (s *Service) RunPush(ctx context.Context) {
	for {
		more, err := s.pushBatch(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("notifications: push: %v", err)
		}
		if more && err == nil {
			continue
		}
		timer := time.NewTimer(pushIdleWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

type pendingPush struct {
	id, userID, category, title, body, link string
	attempts                                int
}

// pushBatch sends one batch; more is true when the batch was full.
func (s *Service) pushBatch(ctx context.Context) (bool, error) {
	if s.sender == nil {
		_, err := s.pool.Exec(ctx, `UPDATE notifications SET push_state = 'skipped' WHERE push_state = 'pending'`)
		return false, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, user_id::text, category, title, body, link, push_attempts
		FROM notifications WHERE push_state = 'pending' AND next_push_at <= now() ORDER BY created_at LIMIT $1`, pushBatch)
	if err != nil {
		return false, fmt.Errorf("list pending pushes: %w", err)
	}
	batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pendingPush, error) {
		var p pendingPush
		return p, row.Scan(&p.id, &p.userID, &p.category, &p.title, &p.body, &p.link, &p.attempts)
	})
	if err != nil {
		return false, fmt.Errorf("list pending pushes: %w", err)
	}
	for _, item := range batch {
		if err := s.deliver(ctx, item); err != nil {
			return false, err
		}
	}
	return len(batch) == pushBatch, nil
}

func (s *Service) deliver(ctx context.Context, item pendingPush) error {
	preferences, err := s.Preferences(ctx, item.userID)
	if err != nil {
		return err
	}
	if enabled, known := preferences[item.category]; known && !enabled {
		return s.finish(ctx, item.id, "skipped")
	}
	tokens, err := s.tokens(ctx, item.userID)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		log.Printf("notifications: push %s skipped, no device registered", item.id)
		return s.finish(ctx, item.id, "skipped")
	}
	delivered, failed := 0, 0
	for _, token := range tokens {
		sendErr := s.sender.Send(ctx, Push{Token: token, Title: item.title, Body: item.body, Link: item.link, ID: item.id})
		switch {
		case sendErr == nil:
			delivered++
		case errors.Is(sendErr, ErrTokenGone):
			if _, err := s.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1`, token); err != nil {
				return fmt.Errorf("drop device: %w", err)
			}
		default:
			failed++
			log.Printf("notifications: push %s: %v", item.id, sendErr)
		}
	}
	log.Printf("notifications: push %s accepted for %d of %d devices", item.id, delivered, len(tokens))
	return s.settle(ctx, item, delivered, failed)
}

// settle: one device reached is enough; only a total failure is retried, so
// a retry never repeats the push on another phone.
func (s *Service) settle(ctx context.Context, item pendingPush, delivered, failed int) error {
	switch {
	case delivered > 0:
		return s.finish(ctx, item.id, "sent")
	case failed == 0:
		return s.finish(ctx, item.id, "skipped")
	case item.attempts+1 >= maxPushAttempts:
		return s.finish(ctx, item.id, "failed")
	}
	delay := time.Duration(1<<item.attempts) * 30 * time.Second
	if _, err := s.pool.Exec(ctx, `UPDATE notifications SET push_attempts = push_attempts + 1, next_push_at = now() + $2::interval WHERE id = $1::uuid`,
		item.id, delay.String()); err != nil {
		return fmt.Errorf("delay push: %w", err)
	}
	return nil
}

func (s *Service) finish(ctx context.Context, id, state string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE notifications SET push_state = $2 WHERE id = $1::uuid`, id, state); err != nil {
		return fmt.Errorf("finish push: %w", err)
	}
	return nil
}

func (s *Service) tokens(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT token FROM device_tokens WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	result, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	return result, nil
}
