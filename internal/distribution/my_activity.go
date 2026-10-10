package distribution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"konkit/internal/auth"
)

// "Lanjutkan pekerjaan" per account: the slots of a schedule the signed-in
// user changed, newest first. Read from the audit log, which already records
// the actor of every slot change and photo upload/delete, so no extra
// bookkeeping is needed.

// MyActivity is one slot the user worked on and when they last touched it.
type MyActivity struct {
	SlotNumber     int       `json:"slot_number"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// myActivityActions are the audit actions that count as working on a slot.
var myActivityActions = []string{
	"distribution.slot_created",
	"distribution.date_updated",
	"distribution.equipment_updated",
	"distribution.slot_linked",
	"distribution.slot_completed",
	"documentation.media_uploaded",
	"documentation.media_deleted",
}

// MyActivity lists the slots of scheduleID changed by userID (at most 100).
// Media events point at a documentation slot; it is mapped to its
// distribution slot.
func (r *Repository) MyActivity(ctx context.Context, userID, scheduleID string, scope auth.RegencyScope) ([]MyActivity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.slot_number, max(al.created_at) AS last_activity_at
		FROM audit_logs al
		JOIN distribution_slots ds ON ds.id::text = CASE
			WHEN al.resource_type = 'distribution_slot' THEN al.resource_id
			ELSE (SELECT dcs.distribution_slot_id::text FROM documentation_slots dcs WHERE dcs.id::text = al.metadata->>'slot_id')
		END
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE al.actor_user_id = $1
		  AND al.action = ANY($2)
		  AND ds.schedule_id = $3
		  AND ($4 OR ps.regency_id::text = ANY($5))
		GROUP BY ds.slot_number
		ORDER BY last_activity_at DESC, ds.slot_number DESC
		LIMIT 100
	`, userID, myActivityActions, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("my distribution activity: %w", err)
	}
	defer rows.Close()
	result := []MyActivity{}
	for rows.Next() {
		var item MyActivity
		if err := rows.Scan(&item.SlotNumber, &item.LastActivityAt); err != nil {
			return nil, fmt.Errorf("scan my distribution activity: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type myActivityRepository interface {
	MyActivity(ctx context.Context, userID, scheduleID string, scope auth.RegencyScope) ([]MyActivity, error)
}

// MyActivity returns the signed-in user's recent slots of a schedule.
func (s *Service) MyActivity(ctx context.Context, actor auth.Principal, scheduleID string, scope auth.RegencyScope) ([]MyActivity, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if s.myActivityRepository == nil {
		return nil, fmt.Errorf("distribution activity is unavailable")
	}
	return s.myActivityRepository.MyActivity(ctx, actor.UserID, scheduleID, scope)
}
