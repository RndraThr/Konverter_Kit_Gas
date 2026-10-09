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

const auditTimezoneJoin = `
	CROSS JOIN (
		SELECT COALESCE(
			max(value #>> '{}') FILTER (WHERE key = 'timezone'),
			'Asia/Jakarta'
		) AS name
		FROM system_settings
	) AS audit_timezone
`

const auditWhere = `
	WHERE ($1 = '' OR audit_logs.action = $1)
	  AND ($2 = '' OR audit_logs.resource_type = $2)
	  AND ($3 = '' OR audit_logs.actor_user_id = NULLIF($3, '')::uuid)
	  AND ($4 = '' OR
		audit_logs.action ILIKE '%' || $4 || '%' OR
		audit_logs.resource_type ILIKE '%' || $4 || '%' OR
		COALESCE(audit_logs.resource_id, '') ILIKE '%' || $4 || '%' OR
		COALESCE(users.full_name, '') ILIKE '%' || $4 || '%' OR
		COALESCE(users.username, '') ILIKE '%' || $4 || '%' OR
		COALESCE(users.email, '') ILIKE '%' || $4 || '%')
	  AND ($5 = '' OR
		COALESCE(users.full_name, '') ILIKE '%' || $5 || '%' OR
		COALESCE(users.username, '') ILIKE '%' || $5 || '%' OR
		COALESCE(users.email, '') ILIKE '%' || $5 || '%')
	  AND ($6 = '' OR audit_logs.created_at >= (NULLIF($6, '')::date::timestamp AT TIME ZONE audit_timezone.name))
	  AND ($7 = '' OR audit_logs.created_at < ((NULLIF($7, '')::date + 1)::timestamp AT TIME ZONE audit_timezone.name))
`

func (r *Repository) List(ctx context.Context, filter Filter) (Page, error) {
	filter = normalizeFilter(filter)
	args := []any{
		filter.Action,
		filter.ResourceType,
		filter.ActorUserID,
		filter.Query,
		filter.Actor,
		filter.DateFrom,
		filter.DateTo,
	}

	var total int64
	var summary Summary
	if err := r.pool.QueryRow(ctx, `
		WITH filtered AS (
			SELECT audit_logs.actor_user_id, audit_logs.created_at, audit_timezone.name AS timezone_name
			FROM audit_logs
			LEFT JOIN users ON users.id = audit_logs.actor_user_id
		`+auditTimezoneJoin+auditWhere+`
		)
		SELECT
			count(*),
			count(*) FILTER (
				WHERE created_at >= (date_trunc('day', now() AT TIME ZONE timezone_name) AT TIME ZONE timezone_name)
				  AND created_at < ((date_trunc('day', now() AT TIME ZONE timezone_name) + interval '1 day') AT TIME ZONE timezone_name)
			),
			count(*) FILTER (WHERE actor_user_id IS NULL)
		FROM filtered
	`, args...).Scan(&total, &summary.Today, &summary.System); err != nil {
		return Page{}, fmt.Errorf("summarize audit events: %w", err)
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
	`+auditTimezoneJoin+auditWhere+`
		ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
		LIMIT $8 OFFSET $9
	`, append(args, filter.PageSize, offset)...)
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
		Summary:  summary,
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
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Action = strings.TrimSpace(filter.Action)
	filter.ResourceType = strings.TrimSpace(filter.ResourceType)
	filter.ActorUserID = strings.TrimSpace(filter.ActorUserID)
	filter.Actor = strings.TrimSpace(filter.Actor)
	filter.DateFrom = strings.TrimSpace(filter.DateFrom)
	filter.DateTo = strings.TrimSpace(filter.DateTo)
	return filter
}
