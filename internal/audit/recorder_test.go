package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRecordRejectsMissingActionOrResource(t *testing.T) {
	tests := []Event{
		{ResourceType: "user"},
		{Action: "user.updated"},
	}
	for _, event := range tests {
		err := Record(context.Background(), &recordingQuerier{}, event)
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got %v", err)
		}
	}
}

func TestRecordRemovesSensitiveMetadataRecursively(t *testing.T) {
	querier := &recordingQuerier{}
	event := Event{
		ActorUserID:  "actor-1",
		Action:       "user.updated",
		ResourceType: "user",
		ResourceID:   "user-2",
		Metadata: map[string]any{
			"email":    "new@konkit.test",
			"password": "plain-text",
			"nested": map[string]any{
				"database_url": "postgres://secret",
				"status":       "active",
			},
			"items": []any{map[string]any{"session_secret": "hidden", "kept": true}},
		},
	}

	if err := Record(context.Background(), querier, event); err != nil {
		t.Fatal(err)
	}
	if len(querier.args) != 7 {
		t.Fatalf("expected 7 query arguments, got %d", len(querier.args))
	}

	raw, ok := querier.args[4].([]byte)
	if !ok {
		t.Fatalf("expected encoded metadata bytes, got %T", querier.args[4])
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, secret := range []string{"password", "plain-text", "database_url", "postgres://secret", "session_secret", "hidden"} {
		if strings.Contains(strings.ToLower(encoded), strings.ToLower(secret)) {
			t.Fatalf("sensitive metadata %q was retained: %s", secret, encoded)
		}
	}
	if metadata["email"] != "new@konkit.test" {
		t.Fatalf("safe metadata was removed: %v", metadata)
	}
}

type recordingQuerier struct {
	args []any
	err  error
}

func (q *recordingQuerier) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	q.args = append([]any(nil), args...)
	return pgconn.NewCommandTag("INSERT 0 1"), q.err
}
