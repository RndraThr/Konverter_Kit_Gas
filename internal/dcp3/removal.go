package dcp3

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// ErrImportNotFound: no imported batch with this id in the caller's kabupaten.
var ErrImportNotFound = errors.New("dcp3 import not found")

// ImportSummary is one imported DCP3 file of a schedule, with how far its
// recipients have gone.
type ImportSummary struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	TotalRows   int        `json:"total_rows"`
	ImportedAt  *time.Time `json:"imported_at"`
	ImportedBy  string     `json:"imported_by"`
	Recipients  int        `json:"recipients"`
	Linked      int        `json:"linked"`
	Distributed int        `json:"distributed"`
}

// RemovalRecipient is one recipient a removal unlinks or keeps.
type RemovalRecipient struct {
	FullName   string `json:"full_name"`
	NIK        string `json:"nik"`
	SlotNumber *int   `json:"slot_number,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Removal is what removing an import does (preview) or did.
type Removal struct {
	Import ImportSummary `json:"import"`
	// Removed recipients are deleted with their DCP3 rows and, when no other
	// data uses them, their person records.
	Removed int `json:"removed"`
	// Unlinked recipients were on a nomor bagi that is not completed: the
	// nomor bagi goes back to open, its photos stay.
	Unlinked []RemovalRecipient `json:"unlinked"`
	// Kept recipients are distributed, replaced or part of a replacement
	// history; they and their DCP3 rows stay.
	Kept []RemovalRecipient `json:"kept"`
	// ImportDeleted: nothing of the file is left, so the file itself is gone
	// and can be imported again.
	ImportDeleted bool `json:"import_deleted"`
}

// ListImports lists the imported DCP3 files of a schedule, newest first.
func (r *Repository) ListImports(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ImportSummary, error) {
	rows, err := r.pool.Query(ctx, importSummarySQL+`
		WHERE b.schedule_id::text = $1 AND b.status = 'imported' AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY b.id, u.id ORDER BY b.imported_at DESC NULLS LAST, b.created_at DESC`, strings.TrimSpace(scheduleID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list dcp3 imports: %w", err)
	}
	defer rows.Close()
	out := []ImportSummary{}
	for rows.Next() {
		var s ImportSummary
		if err := rows.Scan(&s.ID, &s.Filename, &s.TotalRows, &s.ImportedAt, &s.ImportedBy, &s.Recipients, &s.Linked, &s.Distributed); err != nil {
			return nil, fmt.Errorf("scan dcp3 import: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const importSummarySQL = `
	SELECT b.id::text, b.original_filename, b.total_rows, b.imported_at, COALESCE(NULLIF(u.full_name, ''), u.username, ''),
		count(pa.id) FILTER (WHERE pa.status <> 'cancelled'),
		count(ds.id) FILTER (WHERE ds.status <> 'completed'),
		count(pa.id) FILTER (WHERE pa.status IN ('distributed', 'replaced') OR ds.status = 'completed')
	FROM dcp3_import_batches b
	JOIN program_schedules ps ON ps.id = b.schedule_id
	LEFT JOIN users u ON u.id = b.imported_by
	LEFT JOIN candidate_nominations cn ON cn.batch_id = b.id
	LEFT JOIN package_allocations pa ON pa.nomination_id = cn.id
	LEFT JOIN distribution_slots ds ON ds.allocation_id = pa.id`

type batchAllocation struct {
	id, status, fullName, nik string
	slotID                    *string
	slotNumber                *int
	slotStatus                *string
	replaced                  bool
}

// RemoveImport removes an imported DCP3 file. With dryRun it only reports
// what would happen. Everything runs in one transaction.
func (r *Repository) RemoveImport(ctx context.Context, actor auth.Principal, batchID string, dryRun bool, meta auth.ClientMeta, scope auth.RegencyScope) (Removal, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Removal{}, fmt.Errorf("begin remove dcp3 import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID string
	err = tx.QueryRow(ctx, `SELECT b.schedule_id::text FROM dcp3_import_batches b JOIN program_schedules ps ON ps.id = b.schedule_id
		WHERE b.id::text = $1 AND b.status = 'imported' AND ($2 OR ps.regency_id::text = ANY($3)) FOR UPDATE OF b`,
		strings.TrimSpace(batchID), scope.Unrestricted, scope.RegencyIDs).Scan(&scheduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Removal{}, ErrImportNotFound
	}
	if err != nil {
		return Removal{}, fmt.Errorf("lock dcp3 import: %w", err)
	}
	var result Removal
	if err := tx.QueryRow(ctx, importSummarySQL+` WHERE b.id::text = $1 GROUP BY b.id, u.id`, batchID).Scan(&result.Import.ID, &result.Import.Filename,
		&result.Import.TotalRows, &result.Import.ImportedAt, &result.Import.ImportedBy, &result.Import.Recipients, &result.Import.Linked, &result.Import.Distributed); err != nil {
		return Removal{}, fmt.Errorf("summarize dcp3 import: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT pa.id::text, pa.status, COALESCE(p.full_name, ''), COALESCE(p.nik, ''), ds.id::text, ds.slot_number, ds.status,
			EXISTS (SELECT 1 FROM recipient_replacements rr WHERE rr.old_allocation_id = pa.id OR rr.new_allocation_id = pa.id)
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		LEFT JOIN people p ON p.id = pa.intended_person_id
		LEFT JOIN distribution_slots ds ON ds.allocation_id = pa.id
		WHERE cn.batch_id::text = $1
		ORDER BY ds.slot_number NULLS LAST, p.full_name
		FOR UPDATE OF pa`, batchID)
	if err != nil {
		return Removal{}, fmt.Errorf("list import recipients: %w", err)
	}
	var allocations []batchAllocation
	for rows.Next() {
		var a batchAllocation
		if err := rows.Scan(&a.id, &a.status, &a.fullName, &a.nik, &a.slotID, &a.slotNumber, &a.slotStatus, &a.replaced); err != nil {
			rows.Close()
			return Removal{}, fmt.Errorf("scan import recipient: %w", err)
		}
		allocations = append(allocations, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Removal{}, fmt.Errorf("list import recipients: %w", err)
	}

	var drop, unlinkSlots []string
	result.Unlinked, result.Kept = []RemovalRecipient{}, []RemovalRecipient{}
	for _, a := range allocations {
		who := RemovalRecipient{FullName: a.fullName, NIK: a.nik, SlotNumber: a.slotNumber}
		switch {
		case a.status == "distributed" || (a.slotStatus != nil && *a.slotStatus == "completed"):
			who.Reason = "sudah selesai didistribusikan"
			result.Kept = append(result.Kept, who)
		case a.status == "replaced" || a.replaced:
			who.Reason = "punya riwayat penggantian penerima"
			result.Kept = append(result.Kept, who)
		case a.slotID != nil:
			result.Unlinked = append(result.Unlinked, who)
			unlinkSlots = append(unlinkSlots, *a.slotID)
			drop = append(drop, a.id)
		default:
			drop = append(drop, a.id)
		}
	}
	result.Removed = len(drop)
	result.ImportDeleted = len(result.Kept) == 0
	if dryRun {
		return result, nil
	}

	if len(unlinkSlots) > 0 {
		// The wrong person's signature goes; the nomor bagi's photos stay.
		if _, err := tx.Exec(ctx, `DELETE FROM distribution_slot_signatures WHERE distribution_slot_id = ANY($1::uuid[])`, unlinkSlots); err != nil {
			return Removal{}, fmt.Errorf("remove signatures of unlinked slots: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id = NULL, recipient_person_id = NULL, status = 'open',
			needs_recompletion = false, reopened_at = NULL, reopened_by = NULL, reopened_stage = NULL, revision_reason = NULL, updated_at = now()
			WHERE id = ANY($1::uuid[])`, unlinkSlots); err != nil {
			return Removal{}, fmt.Errorf("unlink slots: %w", err)
		}
	}
	// People the removed rows brought in, deleted below when nothing else uses them.
	var people []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(DISTINCT x), '{}') FROM (
			SELECT cn.person_id::text AS x FROM candidate_nominations cn
			WHERE cn.batch_id::text = $1 AND NOT EXISTS (SELECT 1 FROM package_allocations pa WHERE pa.nomination_id = cn.id AND NOT (pa.id::text = ANY($2)))
			UNION SELECT intended_person_id::text FROM package_allocations WHERE id::text = ANY($2)
			UNION SELECT actual_recipient_person_id::text FROM package_allocations WHERE id::text = ANY($2)
		) s WHERE x IS NOT NULL`, batchID, drop).Scan(&people); err != nil {
		return Removal{}, fmt.Errorf("collect import people: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM package_allocations WHERE id::text = ANY($1)`, drop); err != nil {
		return Removal{}, fmt.Errorf("remove import allocations: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM candidate_nominations cn WHERE cn.batch_id::text = $1
		AND NOT EXISTS (SELECT 1 FROM package_allocations pa WHERE pa.nomination_id = cn.id)`, batchID); err != nil {
		return Removal{}, fmt.Errorf("remove import nominations: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dcp3_import_rows ir WHERE ir.batch_id::text = $1
		AND NOT EXISTS (SELECT 1 FROM candidate_nominations cn WHERE cn.import_row_id = ir.id)`, batchID); err != nil {
		return Removal{}, fmt.Errorf("remove import rows: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM people p WHERE p.id::text = ANY($1)
		AND NOT EXISTS (SELECT 1 FROM candidate_nominations WHERE person_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM package_allocations WHERE intended_person_id = p.id OR actual_recipient_person_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM distribution_slots WHERE recipient_person_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM distribution_slot_signatures WHERE person_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM recipient_replacements WHERE old_person_id = p.id OR new_person_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM eligibility_checks WHERE person_id = p.id)`, people); err != nil {
		return Removal{}, fmt.Errorf("remove import people: %w", err)
	}
	if result.ImportDeleted {
		if _, err := tx.Exec(ctx, `DELETE FROM dcp3_import_batches WHERE id::text = $1`, batchID); err != nil {
			return Removal{}, fmt.Errorf("remove import: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `UPDATE dcp3_import_batches SET updated_at = now() WHERE id::text = $1`, batchID); err != nil {
		return Removal{}, fmt.Errorf("touch import: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "dcp3.import_removed", ResourceType: "dcp3_import_batch", ResourceID: batchID,
		Metadata: map[string]any{"schedule_id": scheduleID, "filename": result.Import.Filename, "removed": result.Removed, "unlinked": len(result.Unlinked),
			"kept": len(result.Kept), "import_deleted": result.ImportDeleted}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Removal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Removal{}, fmt.Errorf("commit remove dcp3 import: %w", err)
	}
	return result, nil
}
