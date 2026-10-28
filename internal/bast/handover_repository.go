package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// ListHandoverLocations mengembalikan titik serah jadwal. Jadwal yang belum
// punya daftar lokasi memakai titik serah pada pengaturan BA sebagai lokasi
// pertamanya.
func (r *Repository) ListHandoverLocations(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]HandoverLocation, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM program_schedules WHERE id::text=$1 AND ($2 OR regency_id::text = ANY($3))`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("handover schedule: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO bast_handover_locations (schedule_id, name)
		SELECT st.schedule_id, btrim(st.handover_location) FROM bast_schedule_settings st
		WHERE st.schedule_id::text=$1 AND btrim(COALESCE(st.handover_location,'')) <> ''
		  AND NOT EXISTS (SELECT 1 FROM bast_handover_locations l WHERE l.schedule_id = st.schedule_id)
		ON CONFLICT DO NOTHING
	`, scheduleID); err != nil {
		return nil, fmt.Errorf("seed handover location: %w", err)
	}
	return r.queryHandoverLocations(ctx, r.pool, scheduleID)
}

type handoverQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (r *Repository) queryHandoverLocations(ctx context.Context, db handoverQuerier, scheduleID string) ([]HandoverLocation, error) {
	rows, err := db.Query(ctx, `SELECT id::text, name, areas, sort_order FROM bast_handover_locations WHERE schedule_id::text=$1 ORDER BY sort_order, created_at, id`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list handover locations: %w", err)
	}
	defer rows.Close()
	locations := []HandoverLocation{}
	for rows.Next() {
		var location HandoverLocation
		var areas []byte
		if err := rows.Scan(&location.ID, &location.Name, &areas, &location.SortOrder); err != nil {
			return nil, err
		}
		location.Areas = []HandoverArea{}
		if len(areas) > 0 {
			if err := json.Unmarshal(areas, &location.Areas); err != nil {
				return nil, fmt.Errorf("decode handover areas: %w", err)
			}
		}
		locations = append(locations, location)
	}
	return locations, rows.Err()
}

// listHandoverRecipients mengembalikan seluruh nomor bagi berpenerima pada
// jadwal beserta desa/kecamatan penerima dan pindah manualnya.
func (r *Repository) listHandoverRecipients(ctx context.Context, scheduleID string) ([]handoverRecipient, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_number, COALESCE(p.full_name,''), COALESCE(p.district,''), COALESCE(p.village,''),
			COALESCE(o.location_id::text,''),
			(ds.status='completed' AND ds.distributed_at IS NOT NULL),
			COALESCE(COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date)::text,''),
			ds.verification_snapshot_json
		FROM distribution_slots ds
		JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN bast_handover_overrides o ON o.distribution_slot_id = ds.id
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
		if err := rows.Scan(&item.SlotID, &item.SlotNumber, &item.Name, &item.District, &item.Village, &item.Override, &item.Completed, &item.LocalDate, &item.Snapshot); err != nil {
			return nil, err
		}
		if !item.Completed {
			item.LocalDate, item.Snapshot = "", nil
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListHandoverRecipients mengembalikan penerima jadwal dengan cakupan
// kabupaten pengguna.
func (r *Repository) ListHandoverRecipients(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]handoverRecipient, error) {
	if _, err := r.GetScheduleSettings(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	return r.listHandoverRecipients(ctx, scheduleID)
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

// staleClosingDocuments menandai BA Serah Terima dan Closing Kabupaten final
// perlu dibuat ulang karena pembagian titik serah berubah.
func staleClosingDocuments(ctx context.Context, tx pgx.Tx, scheduleID string) error {
	if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale',updated_at=now()
		WHERE schedule_id::text=$1 AND status='active' AND (document_type LIKE 'closing_titik_serah:%' OR document_type='closing_kabupaten')`, scheduleID); err != nil {
		return fmt.Errorf("invalidate closing documents: %w", err)
	}
	return nil
}

// SaveHandoverLocations mengganti daftar titik serah jadwal. Lokasi yang
// dihapus ikut menghapus pindah manualnya; dokumennya disimpan sebagai riwayat.
func (r *Repository) SaveHandoverLocations(ctx context.Context, actor auth.Principal, scheduleID string, inputs []HandoverLocationInput, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin handover locations: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	current, err := r.queryHandoverLocations(ctx, tx, scheduleID)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, location := range current {
		existing[location.ID] = true
	}
	keep := []string{}
	for _, input := range inputs {
		if input.ID != "" {
			if !existing[input.ID] {
				return ErrHandoverLocationInvalid
			}
			keep = append(keep, input.ID)
		}
	}
	removed := []string{}
	for _, location := range current {
		if !containsString(keep, location.ID) {
			removed = append(removed, location.ID)
		}
	}
	if len(removed) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM bast_handover_locations WHERE id::text = ANY($1)`, removed); err != nil {
			return fmt.Errorf("delete handover locations: %w", err)
		}
		documentTypes := make([]string, len(removed))
		for i, id := range removed {
			documentTypes[i] = closingDocumentType(id)
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='superseded',updated_at=now() WHERE schedule_id::text=$1 AND status IN ('active','stale') AND document_type = ANY($2)`, scheduleID, documentTypes); err != nil {
			return fmt.Errorf("retire handover documents: %w", err)
		}
	}
	// Nama unik per jadwal: kosongkan dulu agar pertukaran nama antarlokasi
	// tidak bentrok dengan indeks unik.
	if _, err := tx.Exec(ctx, `UPDATE bast_handover_locations SET name = id::text WHERE schedule_id::text=$1`, scheduleID); err != nil {
		return fmt.Errorf("reset handover names: %w", err)
	}
	for order, input := range inputs {
		areas, err := json.Marshal(input.Areas)
		if err != nil {
			return err
		}
		if input.ID == "" {
			if _, err := tx.Exec(ctx, `INSERT INTO bast_handover_locations (schedule_id, name, areas, sort_order) VALUES ($1::uuid, $2, $3::jsonb, $4)`, scheduleID, input.Name, string(areas), order); err != nil {
				return fmt.Errorf("insert handover location: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_handover_locations SET name=$2, areas=$3::jsonb, sort_order=$4, updated_at=now() WHERE id::text=$1`, input.ID, input.Name, string(areas), order); err != nil {
			return fmt.Errorf("update handover location: %w", err)
		}
	}
	if err := staleClosingDocuments(ctx, tx, scheduleID); err != nil {
		return err
	}
	names := make([]string, len(inputs))
	for i, input := range inputs {
		names[i] = input.Name
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_locations_updated", ResourceType: "program_schedule", ResourceID: scheduleID, Metadata: map[string]any{"locations": names, "removed": len(removed)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetHandoverOverride memindahkan satu penerima ke titik serah tertentu;
// locationID kosong mengembalikannya ke pembagian otomatis.
func (r *Repository) SetHandoverOverride(ctx context.Context, actor auth.Principal, scheduleID, slotID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin handover override: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	var found bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM distribution_slots WHERE id::text=$1 AND schedule_id::text=$2 AND recipient_person_id IS NOT NULL)`, slotID, scheduleID).Scan(&found); err != nil {
		return fmt.Errorf("check handover slot: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	if strings.TrimSpace(locationID) == "" {
		if _, err := tx.Exec(ctx, `DELETE FROM bast_handover_overrides WHERE distribution_slot_id::text=$1`, slotID); err != nil {
			return fmt.Errorf("clear handover override: %w", err)
		}
	} else {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bast_handover_locations WHERE id::text=$1 AND schedule_id::text=$2)`, locationID, scheduleID).Scan(&found); err != nil {
			return fmt.Errorf("check handover location: %w", err)
		}
		if !found {
			return ErrHandoverLocationInvalid
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO bast_handover_overrides (distribution_slot_id, location_id, updated_by) VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid)
			ON CONFLICT (distribution_slot_id) DO UPDATE SET location_id=EXCLUDED.location_id, updated_by=EXCLUDED.updated_by, updated_at=now()
		`, slotID, locationID, actor.UserID); err != nil {
			return fmt.Errorf("save handover override: %w", err)
		}
	}
	if err := staleClosingDocuments(ctx, tx, scheduleID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.handover_override_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"schedule_id": scheduleID, "location_id": locationID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
