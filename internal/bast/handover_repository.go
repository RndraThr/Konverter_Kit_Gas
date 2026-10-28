package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/textnorm"

	"github.com/jackc/pgx/v5"
)

// scheduleRegency mengembalikan kabupaten jadwal dalam cakupan pengguna.
func (r *Repository) scheduleRegency(ctx context.Context, scheduleID string, scope auth.RegencyScope) (string, error) {
	var regencyID string
	err := r.pool.QueryRow(ctx, `SELECT regency_id::text FROM program_schedules WHERE id::text=$1 AND ($2 OR regency_id::text = ANY($3))`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("handover schedule: %w", err)
	}
	return regencyID, nil
}

// ListHandoverLocations mengembalikan titik serah kabupaten jadwal, urut
// nama. Kabupaten yang belum punya daftar memakai titik serah pada
// pengaturan BA jadwal sebagai lokasi pertamanya.
func (r *Repository) ListHandoverLocations(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]HandoverLocation, error) {
	regencyID, err := r.scheduleRegency(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	var seed string
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(handover_location,'') FROM bast_schedule_settings WHERE schedule_id::text=$1`, scheduleID).Scan(&seed); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("handover seed: %w", err)
	}
	if name := textnorm.PlaceName(seed); textnorm.PlaceKey(name) != "" {
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO bast_handover_locations (regency_id, name, name_key)
			SELECT $1::uuid, $2, $3 WHERE NOT EXISTS (SELECT 1 FROM bast_handover_locations WHERE regency_id::text=$1)
			ON CONFLICT DO NOTHING
		`, regencyID, name, textnorm.PlaceKey(name)); err != nil {
			return nil, fmt.Errorf("seed handover location: %w", err)
		}
	}
	return queryHandoverLocations(ctx, r.pool, regencyID)
}

type handoverQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func queryHandoverLocations(ctx context.Context, db handoverQuerier, regencyID string) ([]HandoverLocation, error) {
	rows, err := db.Query(ctx, `SELECT id::text, name FROM bast_handover_locations WHERE regency_id::text=$1 ORDER BY upper(name), id`, regencyID)
	if err != nil {
		return nil, fmt.Errorf("list handover locations: %w", err)
	}
	defer rows.Close()
	locations := []HandoverLocation{}
	for rows.Next() {
		var location HandoverLocation
		if err := rows.Scan(&location.ID, &location.Name); err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}
	return locations, rows.Err()
}

// ListHandoverRecipients mengembalikan seluruh nomor bagi berpenerima pada
// jadwal beserta titik serahnya.
func (r *Repository) ListHandoverRecipients(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]handoverRecipient, error) {
	if _, err := r.scheduleRegency(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_number, COALESCE(p.full_name,''), COALESCE(p.district,''), COALESCE(p.village,''),
			COALESCE(ds.handover_location_id::text,''),
			(ds.status='completed' AND ds.distributed_at IS NOT NULL),
			COALESCE(COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date)::text,''),
			ds.verification_snapshot_json
		FROM distribution_slots ds
		JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id::text=$1
		ORDER BY ds.slot_number, ds.id
	`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list handover recipients: %w", err)
	}
	defer rows.Close()
	items := []handoverRecipient{}
	for rows.Next() {
		var item handoverRecipient
		if err := rows.Scan(&item.SlotID, &item.SlotNumber, &item.Name, &item.District, &item.Village, &item.LocationID, &item.Completed, &item.LocalDate, &item.Snapshot); err != nil {
			return nil, err
		}
		if !item.Completed {
			item.LocalDate, item.Snapshot = "", nil
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// HandoverDocumentStatuses mengembalikan status BA Serah Terima per lokasi.
func (r *Repository) HandoverDocumentStatuses(ctx context.Context, scheduleID string) (map[string]handoverDocumentStatus, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT substring(document_type FROM 21), status, document_date::text
		FROM bast_aggregate_documents
		WHERE schedule_id::text=$1 AND document_type LIKE 'closing_titik_serah:%' AND status IN ('active','stale')
		ORDER BY finalized_at
	`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("handover document statuses: %w", err)
	}
	defer rows.Close()
	result := map[string]handoverDocumentStatus{}
	for rows.Next() {
		var locationID, status, date string
		if err := rows.Scan(&locationID, &status, &date); err != nil {
			return nil, err
		}
		item := result[locationID]
		if status == "active" {
			item.Active, item.ActiveDate = true, date
		} else {
			item.Stale = true
		}
		result[locationID] = item
	}
	return result, rows.Err()
}

// ListActiveClosingSnapshots mengembalikan snapshot BA Serah Terima final
// terbaru per lokasi titik serah.
func (r *Repository) ListActiveClosingSnapshots(ctx context.Context, scheduleID string) (map[string]ClosingTitikSerahSnapshot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (document_type) substring(document_type FROM 21), snapshot_json
		FROM bast_aggregate_documents
		WHERE schedule_id::text=$1 AND document_type LIKE 'closing_titik_serah:%' AND status='active'
		ORDER BY document_type, finalized_at DESC
	`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list closing snapshots: %w", err)
	}
	defer rows.Close()
	result := map[string]ClosingTitikSerahSnapshot{}
	for rows.Next() {
		var locationID string
		var raw []byte
		if err := rows.Scan(&locationID, &raw); err != nil {
			return nil, err
		}
		var snapshot ClosingTitikSerahSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return nil, fmt.Errorf("decode closing snapshot: %w", err)
		}
		result[locationID] = snapshot
	}
	return result, rows.Err()
}

// staleRegencyClosings menandai BA Serah Terima dan Closing Kabupaten final
// seluruh jadwal kabupaten perlu dibuat ulang (nama atau isi titik serah
// berubah).
func staleRegencyClosings(ctx context.Context, tx pgx.Tx, regencyID string) error {
	if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale',updated_at=now()
		WHERE regency_id::text=$1 AND status='active' AND (document_type LIKE 'closing_titik_serah:%' OR document_type='closing_kabupaten')`, regencyID); err != nil {
		return fmt.Errorf("invalidate closing documents: %w", err)
	}
	return nil
}

// lockHandoverLocation mengunci jadwal dan memastikan lokasi milik
// kabupatennya; mengembalikan kabupaten.
func lockHandoverLocation(ctx context.Context, tx pgx.Tx, scheduleID, locationID string, scope auth.RegencyScope) (string, error) {
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return "", err
	}
	var regencyID string
	err := tx.QueryRow(ctx, `
		SELECT l.regency_id::text FROM bast_handover_locations l
		JOIN program_schedules ps ON ps.regency_id = l.regency_id
		WHERE l.id::text=$1 AND ps.id::text=$2 FOR UPDATE OF l
	`, locationID, scheduleID).Scan(&regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrHandoverLocationInvalid
	}
	if err != nil {
		return "", fmt.Errorf("lock handover location: %w", err)
	}
	return regencyID, nil
}

// RenameHandoverLocation memperbaiki nama titik serah; semua penerimanya ikut
// karena yang tersimpan adalah ID lokasi.
func (r *Repository) RenameHandoverLocation(ctx context.Context, actor auth.Principal, scheduleID, locationID, name string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rename handover location: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	regencyID, err := lockHandoverLocation(ctx, tx, scheduleID, locationID, scope)
	if err != nil {
		return err
	}
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bast_handover_locations WHERE regency_id::text=$1 AND name_key=$2 AND id::text<>$3)`, regencyID, textnorm.PlaceKey(name), locationID).Scan(&taken); err != nil {
		return fmt.Errorf("check handover name: %w", err)
	}
	if taken {
		return ErrHandoverLocationDuplicate
	}
	if _, err := tx.Exec(ctx, `UPDATE bast_handover_locations SET name=$2, name_key=$3, updated_at=now() WHERE id::text=$1`, locationID, name, textnorm.PlaceKey(name)); err != nil {
		return fmt.Errorf("rename handover location: %w", err)
	}
	if err := staleRegencyClosings(ctx, tx, regencyID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_location_renamed", ResourceType: "bast_handover_location", ResourceID: locationID, Metadata: map[string]any{"name": name, "schedule_id": scheduleID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MergeHandoverLocations memindahkan semua penerima lokasi sumber ke lokasi
// tujuan lalu menghapus lokasi sumber (varian nama dobel).
func (r *Repository) MergeHandoverLocations(ctx context.Context, actor auth.Principal, scheduleID, sourceID, targetID string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if sourceID == targetID {
		return ErrHandoverLocationInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin merge handover location: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	regencyID, err := lockHandoverLocation(ctx, tx, scheduleID, sourceID, scope)
	if err != nil {
		return err
	}
	targetRegency, err := lockHandoverLocation(ctx, tx, scheduleID, targetID, scope)
	if err != nil {
		return err
	}
	if targetRegency != regencyID {
		return ErrHandoverLocationInvalid
	}
	tag, err := tx.Exec(ctx, `UPDATE distribution_slots SET handover_location_id=$2::uuid, updated_at=now() WHERE handover_location_id::text=$1`, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("move handover recipients: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='superseded',updated_at=now() WHERE document_type=$1 AND status IN ('active','stale')`, closingDocumentType(sourceID)); err != nil {
		return fmt.Errorf("retire merged documents: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_handover_locations WHERE id::text=$1`, sourceID); err != nil {
		return fmt.Errorf("delete merged location: %w", err)
	}
	if err := staleRegencyClosings(ctx, tx, regencyID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_location_merged", ResourceType: "bast_handover_location", ResourceID: targetID, Metadata: map[string]any{"source_id": sourceID, "moved": tag.RowsAffected(), "schedule_id": scheduleID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteHandoverLocation menghapus titik serah yang tidak dipakai penerima.
func (r *Repository) DeleteHandoverLocation(ctx context.Context, actor auth.Principal, scheduleID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete handover location: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := lockHandoverLocation(ctx, tx, scheduleID, locationID, scope); err != nil {
		return err
	}
	var used bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM distribution_slots WHERE handover_location_id::text=$1)`, locationID).Scan(&used); err != nil {
		return fmt.Errorf("check handover usage: %w", err)
	}
	if used {
		return ErrHandoverLocationInUse
	}
	if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='superseded',updated_at=now() WHERE document_type=$1 AND status IN ('active','stale')`, closingDocumentType(locationID)); err != nil {
		return fmt.Errorf("retire location documents: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_handover_locations WHERE id::text=$1`, locationID); err != nil {
		return fmt.Errorf("delete handover location: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_location_deleted", ResourceType: "bast_handover_location", ResourceID: locationID, Metadata: map[string]any{"schedule_id": scheduleID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AssignHandoverLocation mengoreksi titik serah satu penerima jadwal (juga
// setelah distribusi selesai).
func (r *Repository) AssignHandoverLocation(ctx context.Context, actor auth.Principal, scheduleID, slotID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assign handover location: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	regencyID, err := lockHandoverLocation(ctx, tx, scheduleID, locationID, scope)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE distribution_slots SET handover_location_id=$3::uuid, updated_at=now() WHERE id::text=$1 AND schedule_id::text=$2 AND recipient_person_id IS NOT NULL`, slotID, scheduleID, locationID)
	if err != nil {
		return fmt.Errorf("assign handover location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := staleRegencyClosings(ctx, tx, regencyID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_location_assigned", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"schedule_id": scheduleID, "location_id": locationID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
