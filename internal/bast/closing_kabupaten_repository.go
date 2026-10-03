package bast

import (
	"context"
	"encoding/json"
	"fmt"

	"konkit/internal/auth"
)

// ListClosingKabupatenRows mengumpulkan seluruh distribusi selesai pada
// jadwal dan mengelompokkannya per varian mesin. Lokasi/titik serah (kolom
// pertama tabel) diisi di layer service dari ScheduleSettings.HandoverLocation
// karena sistem saat ini hanya mendukung satu titik serah per jadwal.
func (r *Repository) ListClosingKabupatenRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingKabupatenRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.verification_snapshot_json
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		  AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY ds.id
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list closing kabupaten rows: %w", err)
	}
	defer rows.Close()
	var raw [][]byte
	for rows.Next() {
		var item []byte
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groupClosingKabupatenRows(raw), nil
}

func groupClosingKabupatenRows(raw [][]byte) []ClosingKabupatenRow {
	type key struct{ brand, typ string }
	order := []key{}
	counts := map[key]int{}
	for _, snapshot := range raw {
		brand, typ := "", ""
		if len(snapshot) > 0 {
			var envelope struct {
				Equipment *dp3MachineSnapshot `json:"equipment"`
			}
			if err := json.Unmarshal(snapshot, &envelope); err == nil && envelope.Equipment != nil {
				brand, typ = envelope.Equipment.MachineBrand, envelope.Equipment.MachineType
			}
		}
		k := key{brand: brand, typ: typ}
		if _, exists := counts[k]; !exists {
			order = append(order, k)
		}
		counts[k]++
	}
	result := make([]ClosingKabupatenRow, 0, len(order))
	for _, k := range order {
		result = append(result, ClosingKabupatenRow{MachineBrand: k.brand, MachineType: k.typ, Count: counts[k]})
	}
	return result
}
