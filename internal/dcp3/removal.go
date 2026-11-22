package dcp3

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrImportNotFound: no imported batch with this id in the caller's kabupaten.
	ErrImportNotFound = errors.New("dcp3 import not found")
	// ErrRemovalSelection: a chosen recipient is not part of the import.
	ErrRemovalSelection = errors.New("chosen recipient is not part of the import")
)

// Recipient states in an import removal, from the safest to the most
// consequential. Only "free" recipients are chosen by default.
const (
	RemovalFree      = "free"      // no nomor bagi yet
	RemovalLinked    = "linked"    // on a nomor bagi that is not completed
	RemovalCompleted = "completed" // distributed: its nomor bagi is completed
	RemovalReplaced  = "replaced"  // part of a recipient replacement history
)

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

// RemovalRecipient is one recipient of the import, with what removing it
// does and whether it is chosen by default.
type RemovalRecipient struct {
	AllocationID string `json:"allocation_id"`
	FullName     string `json:"full_name"`
	NIK          string `json:"nik"`
	SlotNumber   *int   `json:"slot_number,omitempty"`
	State        string `json:"state"`
	// Warning says what else changes when the recipient is removed ("" for free ones).
	Warning string `json:"warning,omitempty"`
	// Selected is the default choice: only recipients without a warning.
	Selected bool `json:"selected"`
}

// Removal is what removing an import does (preview) or did.
type Removal struct {
	Import     ImportSummary      `json:"import"`
	Recipients []RemovalRecipient `json:"recipients"`
	// Removed recipients were deleted; Unlinked of them were on a nomor bagi,
	// which went back to open with its photos; Kept were not chosen.
	Removed  int `json:"removed"`
	Unlinked int `json:"unlinked"`
	Kept     int `json:"kept"`
	// ImportDeleted: nothing of the file is left, so it can be imported again.
	ImportDeleted bool `json:"import_deleted"`
}

// RemovalInput chooses the recipients to remove; nil means the default
// choice (recipients without a warning).
type RemovalInput struct {
	AllocationIDs *[]string `json:"allocation_ids"`
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
	RemovalRecipient
	slotID string
}

// removalState classifies a recipient and says what removing it changes.
func removalState(status string, slotStatus *string, replaced bool) (string, string) {
	switch {
	case status == "distributed" || (slotStatus != nil && *slotStatus == "completed"):
		return RemovalCompleted, "Sudah selesai didistribusikan: nomor bagi dikosongkan lagi (foto tetap ada) dan BA terkait perlu dibuat ulang"
	case status == "replaced" || replaced:
		return RemovalReplaced, "Punya riwayat penggantian penerima: riwayat penggantiannya ikut dihapus"
	case slotStatus != nil:
		return RemovalLinked, "Tersambung ke nomor bagi: nomor bagi dikosongkan lagi (foto tetap ada)"
	default:
		return RemovalFree, ""
	}
}

// RemoveImport removes recipients of an imported DCP3 file. With dryRun it
// only lists them with the default choice. Everything runs in one transaction.
func (r *Repository) RemoveImport(ctx context.Context, actor auth.Principal, batchID string, input RemovalInput, dryRun bool, meta auth.ClientMeta, scope auth.RegencyScope) (Removal, error) {
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
		SELECT pa.id::text, pa.status, COALESCE(p.full_name, ''), COALESCE(p.nik, ''), COALESCE(ds.id::text, ''), ds.slot_number, ds.status,
			EXISTS (SELECT 1 FROM recipient_replacements rr WHERE rr.old_allocation_id = pa.id OR rr.new_allocation_id = pa.id)
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		LEFT JOIN people p ON p.id = pa.intended_person_id
		LEFT JOIN distribution_slots ds ON ds.allocation_id = pa.id
		WHERE cn.batch_id::text = $1
		ORDER BY ds.slot_number NULLS FIRST, p.full_name
		FOR UPDATE OF pa`, batchID)
	if err != nil {
		return Removal{}, fmt.Errorf("list import recipients: %w", err)
	}
	var allocations []batchAllocation
	for rows.Next() {
		var a batchAllocation
		var status string
		var slotStatus *string
		var replaced bool
		if err := rows.Scan(&a.AllocationID, &status, &a.FullName, &a.NIK, &a.slotID, &a.SlotNumber, &slotStatus, &replaced); err != nil {
			rows.Close()
			return Removal{}, fmt.Errorf("scan import recipient: %w", err)
		}
		a.State, a.Warning = removalState(status, slotStatus, replaced)
		a.Selected = a.State == RemovalFree
		allocations = append(allocations, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Removal{}, fmt.Errorf("list import recipients: %w", err)
	}

	// The chosen recipients: the default, or exactly the ones sent.
	if input.AllocationIDs != nil {
		chosen := *input.AllocationIDs
		for _, id := range chosen {
			if !slices.ContainsFunc(allocations, func(a batchAllocation) bool { return a.AllocationID == strings.TrimSpace(id) }) {
				return Removal{}, ErrRemovalSelection
			}
		}
		for i := range allocations {
			allocations[i].Selected = slices.ContainsFunc(chosen, func(id string) bool { return strings.TrimSpace(id) == allocations[i].AllocationID })
		}
	}
	var drop, slots, completedSlots, replacedAllocations []string
	result.Recipients = make([]RemovalRecipient, 0, len(allocations))
	for _, a := range allocations {
		result.Recipients = append(result.Recipients, a.RemovalRecipient)
		if !a.Selected {
			result.Kept++
			continue
		}
		drop = append(drop, a.AllocationID)
		if a.slotID != "" {
			slots = append(slots, a.slotID)
			result.Unlinked++
			if a.State == RemovalCompleted {
				completedSlots = append(completedSlots, a.slotID)
			}
		}
		if a.State == RemovalReplaced {
			replacedAllocations = append(replacedAllocations, a.AllocationID)
		}
	}
	result.Removed = len(drop)
	result.ImportDeleted = result.Kept == 0
	if dryRun {
		return result, nil
	}

	if len(replacedAllocations) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM recipient_replacements WHERE old_allocation_id::text = ANY($1) OR new_allocation_id::text = ANY($1)`, replacedAllocations); err != nil {
			return Removal{}, fmt.Errorf("remove replacement history: %w", err)
		}
	}
	if len(completedSlots) > 0 {
		// Same invalidation as reopening a completed slot: its BA documents
		// are made again when opened.
		if _, err := tx.Exec(ctx, `UPDATE bast_daily_bundles b SET status = 'stale', updated_at = now()
			WHERE b.status = 'active' AND EXISTS (SELECT 1 FROM bast_daily_bundle_items bi JOIN bast_individual_documents d ON d.id = bi.individual_document_id
				WHERE bi.bundle_id = b.id AND d.distribution_slot_id::text = ANY($1) AND d.status = 'final')`, completedSlots); err != nil {
			return Removal{}, fmt.Errorf("invalidate daily BA bundles: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_individual_documents SET status = 'stale' WHERE distribution_slot_id::text = ANY($1) AND status = 'final'`, completedSlots); err != nil {
			return Removal{}, fmt.Errorf("invalidate individual BA: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status = 'stale', updated_at = now() WHERE schedule_id::text = $1 AND status = 'active'`, scheduleID); err != nil {
			return Removal{}, fmt.Errorf("invalidate aggregate BA: %w", err)
		}
	}
	if len(slots) > 0 {
		// The wrong person's signature goes; the nomor bagi's photos stay.
		if _, err := tx.Exec(ctx, `DELETE FROM distribution_slot_signatures WHERE distribution_slot_id::text = ANY($1)`, slots); err != nil {
			return Removal{}, fmt.Errorf("remove signatures of unlinked slots: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id = NULL, recipient_person_id = NULL, status = 'open',
			verification_snapshot_json = '{}'::jsonb, distributed_at = NULL, distributed_by = NULL, completed_at = NULL,
			needs_recompletion = false, reopened_at = NULL, reopened_by = NULL, reopened_stage = NULL, revision_reason = NULL, updated_at = now()
			WHERE id::text = ANY($1)`, slots); err != nil {
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
		Metadata: map[string]any{"schedule_id": scheduleID, "filename": result.Import.Filename, "removed": result.Removed, "unlinked": result.Unlinked,
			"completed_removed": len(completedSlots), "kept": result.Kept, "import_deleted": result.ImportDeleted}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Removal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Removal{}, fmt.Errorf("commit remove dcp3 import: %w", err)
	}
	return result, nil
}
