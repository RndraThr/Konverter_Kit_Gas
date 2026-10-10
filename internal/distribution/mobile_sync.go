package distribution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"konkit/internal/auth"
)

// Mobile delta sync. The field app keeps a local copy of a schedule's slots
// and candidates and pulls only what changed since its last cursor, so it can
// work offline and stay current when online (local-first).

// SyncCandidate is a DCP3 candidate of a schedule with the fields the app
// needs to link it offline.
type SyncCandidate struct {
	CandidateMatch
	// Linked is true once the allocation got a distribution number; the app
	// hides it from POS Dokumen search.
	Linked    bool      `json:"linked"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncResult carries changed items plus the server time to use as the next cursor.
type SyncResult[T any] struct {
	Items      []T       `json:"items"`
	ServerTime time.Time `json:"server_time"`
}

// SyncCandidates returns candidates of the schedule changed since since (all when nil).
func (r *Repository) SyncCandidates(ctx context.Context, scheduleID string, since *time.Time, scope auth.RegencyScope) (SyncResult[SyncCandidate], error) {
	result := SyncResult[SyncCandidate]{Items: []SyncCandidate{}}
	if err := r.pool.QueryRow(ctx, `SELECT now()`).Scan(&result.ServerTime); err != nil {
		return result, fmt.Errorf("sync candidates clock: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT pa.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
			COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''), cn.program_type,
			pa.distribution_number IS NOT NULL,
			GREATEST(pa.updated_at, cn.updated_at, p.updated_at)
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		LEFT JOIN LATERAL (SELECT identifier_type, normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE pa.schedule_id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
			AND ($4::timestamptz IS NULL OR GREATEST(pa.updated_at, cn.updated_at, p.updated_at) >= $4)
		ORDER BY p.nik
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs, since)
	if err != nil {
		return result, fmt.Errorf("sync candidates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item SyncCandidate
		if err := rows.Scan(&item.AllocationID, &item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier,
			&item.Address, &item.Village, &item.District, &item.PhoneNumber, &item.ProgramType, &item.Linked, &item.UpdatedAt); err != nil {
			return result, fmt.Errorf("scan sync candidate: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

// SyncSlots returns full slots (documentation and media included) whose slot
// row, documentation or media changed since since (all when nil).
func (r *Repository) SyncSlots(ctx context.Context, scheduleID string, since *time.Time, scope auth.RegencyScope) (SyncResult[DistributionSlot], error) {
	result := SyncResult[DistributionSlot]{Items: []DistributionSlot{}}
	if err := r.pool.QueryRow(ctx, `SELECT now()`).Scan(&result.ServerTime); err != nil {
		return result, fmt.Errorf("sync slots clock: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
			AND ($4::timestamptz IS NULL OR GREATEST(
				ds.updated_at,
				(SELECT max(dcs.updated_at) FROM documentation_slots dcs WHERE dcs.distribution_slot_id = ds.id),
				(SELECT max(mf.updated_at) FROM media_files mf
					JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
					WHERE dcs.distribution_slot_id = ds.id)
			) >= $4)
		ORDER BY ds.slot_number
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs, since)
	if err != nil {
		return result, fmt.Errorf("sync slots: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan sync slot: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("sync slots: %w", err)
	}
	for _, id := range ids {
		slot, err := r.getSlotByID(ctx, id)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, slot)
	}
	return result, nil
}

type syncRepository interface {
	SyncCandidates(ctx context.Context, scheduleID string, since *time.Time, scope auth.RegencyScope) (SyncResult[SyncCandidate], error)
	SyncSlots(ctx context.Context, scheduleID string, since *time.Time, scope auth.RegencyScope) (SyncResult[DistributionSlot], error)
}

// parseSince accepts an RFC3339 cursor; empty means a full sync.
func parseSince(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, ErrSyncCursorInvalid
	}
	return &parsed, nil
}

func (s *Service) SyncCandidates(ctx context.Context, scheduleID, since string, scope auth.RegencyScope) (SyncResult[SyncCandidate], error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return SyncResult[SyncCandidate]{}, ErrScheduleRequired
	}
	cursor, err := parseSince(since)
	if err != nil {
		return SyncResult[SyncCandidate]{}, err
	}
	if s.syncRepository == nil {
		return SyncResult[SyncCandidate]{}, fmt.Errorf("distribution sync is unavailable")
	}
	return s.syncRepository.SyncCandidates(ctx, scheduleID, cursor, scope)
}

func (s *Service) SyncSlots(ctx context.Context, scheduleID, since string, scope auth.RegencyScope) (SyncResult[DistributionSlot], error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return SyncResult[DistributionSlot]{}, ErrScheduleRequired
	}
	cursor, err := parseSince(since)
	if err != nil {
		return SyncResult[DistributionSlot]{}, err
	}
	if s.syncRepository == nil {
		return SyncResult[DistributionSlot]{}, fmt.Errorf("distribution sync is unavailable")
	}
	return s.syncRepository.SyncSlots(ctx, scheduleID, cursor, scope)
}
