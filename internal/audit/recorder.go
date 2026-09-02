package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type Querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func Record(ctx context.Context, db Querier, event Event) error {
	event.Action = strings.TrimSpace(event.Action)
	event.ResourceType = strings.TrimSpace(event.ResourceType)
	if event.Action == "" || event.ResourceType == "" {
		return ErrInvalidEvent
	}

	metadata, err := json.Marshal(sanitizeMap(event.Metadata))
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id, action, resource_type, resource_id,
			metadata, ip_address, user_agent
		)
		VALUES (
			NULLIF($1, '')::uuid, $2, $3, NULLIF($4, ''),
			$5::jsonb, NULLIF($6, '')::inet, NULLIF($7, '')
		)
	`, strings.TrimSpace(event.ActorUserID), event.Action, event.ResourceType,
		strings.TrimSpace(event.ResourceID), metadata, strings.TrimSpace(event.IPAddress),
		strings.TrimSpace(event.UserAgent))
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func sanitizeMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		if sensitiveKey(key) {
			continue
		}
		result[key] = sanitizeValue(value)
	}
	return result
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizeValue(item)
		}
		return result
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "password_hash", "session_secret", "database_url":
		return true
	default:
		return false
	}
}
