package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context) ([]Setting, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT key, value, value_type, COALESCE(description, ''), COALESCE(updated_by::text, ''), updated_at
		FROM system_settings ORDER BY key
	`)
	if err != nil {
		return nil, fmt.Errorf("list system settings: %w", err)
	}
	defer rows.Close()
	result := make([]Setting, 0, len(Definitions))
	for rows.Next() {
		var setting Setting
		var value []byte
		if err := rows.Scan(&setting.Key, &value, &setting.Type, &setting.Description, &setting.UpdatedBy, &setting.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan system setting: %w", err)
		}
		if err := json.Unmarshal(value, &setting.Value); err != nil {
			return nil, fmt.Errorf("decode system setting %s: %w", setting.Key, err)
		}
		result = append(result, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system settings: %w", err)
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, actor auth.Principal, values map[string]string, meta auth.ClientMeta) ([]Setting, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin settings update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows, err := tx.Query(ctx, `SELECT key, value FROM system_settings WHERE key = ANY($1) ORDER BY key FOR UPDATE`, keys)
	if err != nil {
		return nil, fmt.Errorf("lock system settings: %w", err)
	}
	previous := make(map[string]string, len(keys))
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan locked setting: %w", err)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode locked setting %s: %w", key, err)
		}
		previous[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate locked settings: %w", err)
	}
	rows.Close()
	if len(previous) != len(values) {
		return nil, ErrInvalidSetting
	}

	for _, key := range keys {
		encoded, err := json.Marshal(values[key])
		if err != nil {
			return nil, fmt.Errorf("encode setting %s: %w", key, err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE system_settings
			SET value = $2::jsonb, updated_by = $3, updated_at = now()
			WHERE key = $1
		`, key, encoded, actor.UserID); err != nil {
			return nil, fmt.Errorf("update system setting %s: %w", key, err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{
		ActorUserID:  actor.UserID,
		Action:       "settings.updated",
		ResourceType: "system_settings",
		Metadata:     map[string]any{"previous": previous, "current": values},
		IPAddress:    meta.IPAddress,
		UserAgent:    meta.UserAgent,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit settings update: %w", err)
	}
	return r.List(ctx)
}
