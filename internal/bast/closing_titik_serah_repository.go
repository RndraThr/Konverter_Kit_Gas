package bast

import (
	"context"
	"encoding/json"
	"fmt"
)

type closingRawRow struct {
	LocalDate            string
	VerificationSnapshot []byte
}

// StaleClosingKabupaten menandai Closing Kabupaten final perlu dibuat ulang
// setelah BA Serah Terima suatu titik serah berubah.
func (r *Repository) StaleClosingKabupaten(ctx context.Context, scheduleID string) error {
	if _, err := r.pool.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale',updated_at=now() WHERE schedule_id::text=$1 AND status='active' AND document_type='closing_kabupaten'`, scheduleID); err != nil {
		return fmt.Errorf("invalidate closing kabupaten: %w", err)
	}
	return nil
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
