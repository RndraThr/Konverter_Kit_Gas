package bast

import (
	"context"
	"encoding/json"
	"fmt"

	"konkit/internal/auth"
)

type closingRawRow struct {
	LocalDate            string
	VerificationSnapshot []byte
}

// ListClosingRows mengumpulkan seluruh distribusi selesai pada jadwal (tidak
// dibatasi satu tanggal) dan mengelompokkannya per (tanggal, varian mesin).
func (r *Repository) ListClosingRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date::text AS local_date, ds.verification_snapshot_json
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		  AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY local_date, ds.id
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list closing rows: %w", err)
	}
	defer rows.Close()
	raw := []closingRawRow{}
	for rows.Next() {
		var item closingRawRow
		if err := rows.Scan(&item.LocalDate, &item.VerificationSnapshot); err != nil {
			return nil, err
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groupClosingRows(raw), nil
}

// groupClosingRows menjaga urutan kemunculan pertama (tanggal, varian) agar
// hasilnya deterministik, dengan baris tanpa snapshot mesin lengkap dibiarkan
// kosong (dideteksi oleh validateClosingRows).
func groupClosingRows(raw []closingRawRow) []ClosingRow {
	type key struct{ date, brand, typ string }
	order := []key{}
	counts := map[key]int{}
	for _, item := range raw {
		brand, typ := "", ""
		if len(item.VerificationSnapshot) > 0 {
			var envelope struct {
				Equipment *dp3MachineSnapshot `json:"equipment"`
			}
			if err := json.Unmarshal(item.VerificationSnapshot, &envelope); err == nil && envelope.Equipment != nil {
				brand, typ = envelope.Equipment.MachineBrand, envelope.Equipment.MachineType
			}
		}
		k := key{date: item.LocalDate, brand: brand, typ: typ}
		if _, exists := counts[k]; !exists {
			order = append(order, k)
		}
		counts[k]++
	}
	result := make([]ClosingRow, 0, len(order))
	for _, k := range order {
		result = append(result, ClosingRow{LocalDate: k.date, MachineBrand: k.brand, MachineType: k.typ, Count: counts[k]})
	}
	return result
}
