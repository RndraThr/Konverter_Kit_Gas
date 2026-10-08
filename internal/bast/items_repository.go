package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// GetScheduleTemplateEntries mengembalikan barang template paket yang dipasang
// pada jadwal.
func (r *Repository) GetScheduleTemplateEntries(ctx context.Context, scheduleID string) ([]TemplateEntry, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT pt.values_json FROM program_schedules s JOIN package_template_versions pt ON pt.id=s.package_template_version_id WHERE s.id=$1`, scheduleID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule template items: %w", err)
	}
	return TemplateEntriesFromValues(raw)
}

// GetZonePONumbers mengembalikan No. PO zona dengan kunci poKey(jenis, kode).
func (r *Repository) GetZonePONumbers(ctx context.Context, zoneID string) (map[string]string, error) {
	numbers := map[string]string{}
	if zoneID == "" {
		return numbers, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT item_kind,item_code,po_number FROM bast_zone_po_numbers WHERE zone_id=$1`, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list zone PO numbers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, code, number string
		if err := rows.Scan(&kind, &code, &number); err != nil {
			return nil, err
		}
		numbers[poKey(kind, code)] = number
	}
	return numbers, rows.Err()
}

// ZoneTemplateValues adalah values_json template paket yang dipakai jadwal di
// satu zona program.
type ZoneTemplateValues struct {
	ZoneID   string
	ZoneName string
	Values   [][]byte
}

// ListProgramZoneTemplates mengembalikan zona non-placeholder program beserta
// template paket jadwal kabupaten di zona tersebut. Zona tanpa jadwal memakai
// semua template jadwal program agar PO tetap bisa disiapkan lebih dulu.
func (r *Repository) ListProgramZoneTemplates(ctx context.Context, programID string) ([]ZoneTemplateValues, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM programs WHERE id=$1)`, programID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check program: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT z.id::text, z.name,
			COALESCE((SELECT jsonb_agg(DISTINCT pt.values_json) FROM program_schedules s
				JOIN program_regency_assignments a ON a.program_id=s.program_id AND a.regency_id=s.regency_id
				JOIN package_template_versions pt ON pt.id=s.package_template_version_id
				WHERE s.program_id=z.program_id AND a.zone_id=z.id), '[]'::jsonb),
			COALESCE((SELECT jsonb_agg(DISTINCT pt.values_json) FROM program_schedules s
				JOIN package_template_versions pt ON pt.id=s.package_template_version_id
				WHERE s.program_id=z.program_id), '[]'::jsonb)
		FROM program_zones z
		WHERE z.program_id=$1 AND NOT z.is_placeholder
		ORDER BY z.sort_order, z.name`, programID)
	if err != nil {
		return nil, fmt.Errorf("list program zone templates: %w", err)
	}
	defer rows.Close()
	result := []ZoneTemplateValues{}
	for rows.Next() {
		var zone ZoneTemplateValues
		var own, all []byte
		if err := rows.Scan(&zone.ZoneID, &zone.ZoneName, &own, &all); err != nil {
			return nil, err
		}
		var values []json.RawMessage
		if err := json.Unmarshal(own, &values); err != nil {
			return nil, err
		}
		if len(values) == 0 {
			if err := json.Unmarshal(all, &values); err != nil {
				return nil, err
			}
		}
		for _, value := range values {
			zone.Values = append(zone.Values, []byte(value))
		}
		result = append(result, zone)
	}
	return result, rows.Err()
}

// ZonePOInput adalah satu No. PO zona untuk satu barang+merk template
// (Kind = jenis barang, Code = kode opsi atau komponen).
type ZonePOInput struct {
	Kind     string `json:"kind"`
	Code     string `json:"code"`
	PONumber string `json:"po_number"`
}

func (r *Repository) ReplaceZonePONumbers(ctx context.Context, actor auth.Principal, programID, zoneID string, numbers []ZonePOInput, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM program_zones WHERE id=$1 AND program_id=$2 AND NOT is_placeholder)`, zoneID, programID).Scan(&exists); err != nil {
		return fmt.Errorf("check zone: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_zone_po_numbers WHERE zone_id=$1`, zoneID); err != nil {
		return fmt.Errorf("clear zone PO numbers: %w", err)
	}
	for _, number := range numbers {
		if _, err := tx.Exec(ctx, `INSERT INTO bast_zone_po_numbers(zone_id,item_kind,item_code,po_number,updated_by) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid)`, zoneID, number.Kind, number.Code, number.PONumber, actor.UserID); err != nil {
			return fmt.Errorf("insert zone PO number: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.zone_po_saved", ResourceType: "program_zone", ResourceID: zoneID, Metadata: map[string]any{"po_numbers": len(numbers)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetScheduleItemSelection mengembalikan pilihan merk manual per barang pada
// satu jadwal (kosong bila belum pernah diatur).
func (r *Repository) GetScheduleItemSelection(ctx context.Context, scheduleID string) (map[string]string, error) {
	var payload []byte
	err := r.pool.QueryRow(ctx, `SELECT selection_json FROM bast_schedule_item_variants WHERE schedule_id=$1`, scheduleID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule item selection: %w", err)
	}
	selection := map[string]string{}
	if err := json.Unmarshal(payload, &selection); err != nil {
		return nil, fmt.Errorf("decode schedule item selection: %w", err)
	}
	return selection, nil
}

func (r *Repository) UpsertScheduleItemSelection(ctx context.Context, actor auth.Principal, scheduleID string, selection map[string]string, meta auth.ClientMeta) error {
	payload, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO bast_schedule_item_variants(schedule_id,selection_json,updated_by)
		VALUES($1,$2,NULLIF($3,'')::uuid)
		ON CONFLICT(schedule_id) DO UPDATE SET selection_json=EXCLUDED.selection_json,updated_by=EXCLUDED.updated_by,updated_at=now()
	`, scheduleID, payload, actor.UserID); err != nil {
		return fmt.Errorf("upsert schedule item selection: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.schedule_items_saved", ResourceType: "bast_schedule_item_variants", ResourceID: scheduleID, Metadata: map[string]any{"selected": len(selection)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
