package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context, filter Filter) (Page, error) {
	filter = normalizeFilter(filter)
	action := strings.TrimSpace(filter.Action)
	resourceType := strings.TrimSpace(filter.ResourceType)
	actorUserID := strings.TrimSpace(filter.ActorUserID)

	const where = `
		WHERE ($1 = '' OR audit_logs.action = $1)
		  AND ($2 = '' OR audit_logs.resource_type = $2)
		  AND ($3 = '' OR audit_logs.actor_user_id = NULLIF($3, '')::uuid)
	`

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs "+where,
		action, resourceType, actorUserID).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count audit events: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	rows, err := r.pool.Query(ctx, `
		SELECT
			audit_logs.id::text,
			COALESCE(audit_logs.actor_user_id::text, ''),
			COALESCE(NULLIF(users.full_name, ''), users.username, ''),
			audit_logs.action,
			audit_logs.resource_type,
			COALESCE(audit_logs.resource_id, ''),
			audit_logs.metadata,
			COALESCE(audit_logs.ip_address::text, ''),
			COALESCE(audit_logs.user_agent, ''),
			audit_logs.created_at
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.actor_user_id
	`+where+`
		ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
		LIMIT $4 OFFSET $5
	`, action, resourceType, actorUserID, filter.PageSize, offset)
	if err != nil {
		return Page{}, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	items := make([]Entry, 0, filter.PageSize)
	for rows.Next() {
		var entry Entry
		var metadata []byte
		if err := rows.Scan(
			&entry.ID,
			&entry.ActorUserID,
			&entry.ActorName,
			&entry.Action,
			&entry.ResourceType,
			&entry.ResourceID,
			&metadata,
			&entry.IPAddress,
			&entry.UserAgent,
			&entry.CreatedAt,
		); err != nil {
			return Page{}, fmt.Errorf("scan audit event: %w", err)
		}
		if err := json.Unmarshal(metadata, &entry.Metadata); err != nil {
			return Page{}, fmt.Errorf("decode audit metadata: %w", err)
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate audit events: %w", err)
	}

	return Page{
		Items:    items,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
	}, nil
}

func normalizeFilter(filter Filter) Filter {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	return filter
}
