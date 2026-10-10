package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"konkit/internal/auth"
)

// Mobile delta sync of activity documentation: the app keeps a local copy of
// a program+regency's media (counts per activity type come from it) and pulls
// what changed since its cursor, deletions included (status "deleted").

var ErrSyncCursorInvalid = fmt.Errorf("since must be an RFC 3339 time")

// SyncResult carries changed media plus the server time to use as the next cursor.
type SyncResult struct {
	Items      []ActivityMedia `json:"items"`
	ServerTime time.Time       `json:"server_time"`
}

// Sync returns media of the program and regency changed since since (all
// active ones when nil). Deleted media are included after a cursor so the
// app can drop them.
func (r *Repository) Sync(ctx context.Context, programID, regencyID string, since *time.Time, scope auth.RegencyScope) (SyncResult, error) {
	result := SyncResult{Items: []ActivityMedia{}}
	if err := r.pool.QueryRow(ctx, `SELECT now()`).Scan(&result.ServerTime); err != nil {
		return result, fmt.Errorf("activity sync clock: %w", err)
	}
	rows, err := r.pool.Query(ctx, activityMediaSelect+`
		WHERE m.regency_id = $1
		  AND ($2 = '' OR m.program_id = NULLIF($2,'')::uuid)
		  AND ($3 OR r.id::text = ANY($4))
		  AND (($5::timestamptz IS NULL AND m.status = 'active') OR m.updated_at >= $5)
		ORDER BY m.updated_at, m.id
	`, regencyID, programID, scope.Unrestricted, scope.RegencyIDs, since)
	if err != nil {
		return result, fmt.Errorf("activity sync: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanActivityMedia(rows)
		if err != nil {
			return result, err
		}
		item.ContentURL = "/api/v1/activities/media/" + item.ID + "/content"
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

type syncRepository interface {
	Sync(ctx context.Context, programID, regencyID string, since *time.Time, scope auth.RegencyScope) (SyncResult, error)
}

// Sync validates the request and returns the delta.
func (s *Service) Sync(ctx context.Context, programID, regencyID, since string, scope auth.RegencyScope) (SyncResult, error) {
	regencyID = strings.TrimSpace(regencyID)
	if regencyID == "" {
		return SyncResult{}, ErrRegencyRequired
	}
	var cursor *time.Time
	if since = strings.TrimSpace(since); since != "" {
		parsed, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			return SyncResult{}, ErrSyncCursorInvalid
		}
		cursor = &parsed
	}
	repo, ok := s.repository.(syncRepository)
	if !ok {
		return SyncResult{}, fmt.Errorf("activity sync is unavailable")
	}
	return repo.Sync(ctx, strings.TrimSpace(programID), regencyID, cursor, scope)
}
