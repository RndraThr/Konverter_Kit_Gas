package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// Status tanggal pada ringkasan BA.
const (
	OverviewFinal    = "final"    // dokumen final aktif
	OverviewStale    = "stale"    // data berubah setelah final, perlu dibuat ulang
	OverviewUploaded = "uploaded" // arsip hasil terisi sudah diunggah
)

// OverviewEntry meringkas satu jenis BA dalam satu jadwal.
type OverviewEntry struct {
	// Dates memetakan tanggal (YYYY-MM-DD) ke final, stale, atau uploaded.
	Dates map[string]string `json:"dates"`
	Final int               `json:"final"`
	Stale int               `json:"stale"`
	// WetSupported: BA ini punya mode tanda tangan basah.
	WetSupported bool `json:"wet_supported"`
	WetEnabled   bool `json:"wet_enabled"`
	// WetScans: tanggal yang sudah punya pindaian tanda tangan basah.
	WetScans []string `json:"wet_scans"`
}

// Overview adalah ringkasan seluruh BA satu jadwal, dikunci dengan kunci
// jenis BA yang sama dengan rute API (dp3, individual, daily-recap, ...).
type Overview struct {
	ScheduleID string                    `json:"schedule_id"`
	Types      map[string]*OverviewEntry `json:"types"`
}

// overviewAggregates memetakan jenis BA ke document_type pada
// bast_aggregate_documents (pemeriksaan memakai awalan "pemeriksaan:").
var overviewAggregates = map[string]string{
	"dp3":                 "dp3",
	"daily-recap":         "daily_recap",
	"closing-titik-serah": "closing_titik_serah",
	"closing-kabupaten":   "closing_kabupaten",
	"servis-berkala":      "servis_berkala",
	"tkdn":                "tkdn",
	"pemeriksaan":         "pemeriksaan",
}

// overviewArchives memetakan jenis BA arsip ke document_kind pada
// bast_rakorda_uploads.
var overviewArchives = map[string]string{
	"rakorda":      "rakorda",
	"sosialisasi":  "sosialisasi",
	"training-10":  "training_10",
	"training-100": "training_100",
}

// overviewWetKeys memetakan jenis BA ke kunci dokumen tanda tangan basah.
var overviewWetKeys = map[string]string{
	"dp3":                 "dp3",
	"daily-recap":         "rekap-harian",
	"closing-titik-serah": "closing-titik-serah",
	"closing-kabupaten":   "closing-kabupaten",
	"servis-berkala":      "servis-berkala",
	"tkdn":                "tkdn",
}

// Overview mengembalikan status final, perlu dibuat ulang, arsip, dan tanda
// tangan basah per jenis BA untuk satu jadwal.
func (r *Repository) Overview(ctx context.Context, scheduleID string, scope auth.RegencyScope) (Overview, error) {
	var programID, regencyID string
	err := r.pool.QueryRow(ctx, `
		SELECT ps.program_id::text, ps.regency_id::text FROM program_schedules ps
		WHERE ps.id::text=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&programID, &regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Overview{}, ErrNotFound
	}
	if err != nil {
		return Overview{}, fmt.Errorf("overview schedule: %w", err)
	}

	result := Overview{ScheduleID: scheduleID, Types: map[string]*OverviewEntry{}}
	entry := func(key string) *OverviewEntry {
		if result.Types[key] == nil {
			result.Types[key] = &OverviewEntry{Dates: map[string]string{}, WetScans: []string{}}
		}
		return result.Types[key]
	}
	for key := range overviewAggregates {
		entry(key)
	}
	for key := range overviewArchives {
		entry(key)
	}
	entry("individual")

	// Dokumen final: per tanggal, aktif menang atas stale.
	documentKeys := map[string]string{}
	for key, documentType := range overviewAggregates {
		documentKeys[documentType] = key
	}
	rows, err := r.pool.Query(ctx, `
		SELECT CASE WHEN document_type LIKE 'pemeriksaan:%' THEN 'pemeriksaan' ELSE document_type END,
			document_date::text, bool_or(status='active')
		FROM bast_aggregate_documents
		WHERE schedule_id::text=$1 AND status IN ('active','stale')
		GROUP BY 1, 2
	`, scheduleID)
	if err != nil {
		return Overview{}, fmt.Errorf("overview aggregates: %w", err)
	}
	if err := collectOverview(rows, func(documentType string) *OverviewEntry {
		if key, ok := documentKeys[documentType]; ok {
			return entry(key)
		}
		return nil
	}); err != nil {
		return Overview{}, err
	}

	rows, err = r.pool.Query(ctx, `
		SELECT 'individual', local_date::text, bool_or(status='active')
		FROM bast_daily_bundles
		WHERE program_id::text=$1 AND regency_id::text=$2 AND document_type='individual' AND status IN ('active','stale')
		GROUP BY 1, 2
	`, programID, regencyID)
	if err != nil {
		return Overview{}, fmt.Errorf("overview bundles: %w", err)
	}
	if err := collectOverview(rows, func(string) *OverviewEntry { return entry("individual") }); err != nil {
		return Overview{}, err
	}

	archiveKeys := map[string]string{}
	for key, kind := range overviewArchives {
		archiveKeys[kind] = key
	}
	rows, err = r.pool.Query(ctx, `
		SELECT DISTINCT document_kind, event_date::text FROM bast_rakorda_uploads WHERE schedule_id::text=$1
	`, scheduleID)
	if err != nil {
		return Overview{}, fmt.Errorf("overview archives: %w", err)
	}
	for rows.Next() {
		var kind, date string
		if err := rows.Scan(&kind, &date); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan overview archive: %w", err)
		}
		if key, ok := archiveKeys[kind]; ok {
			item := entry(key)
			item.Dates[date] = OverviewUploaded
			item.Final++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Overview{}, fmt.Errorf("overview archives: %w", err)
	}

	// Tanda tangan basah: mode efektif per BA dan tanggal yang sudah dipindai.
	wetTypes := map[string]string{}
	for key, documentKey := range overviewWetKeys {
		effective, err := r.effectiveSignatureRules(ctx, scheduleID, documentKey)
		if err != nil {
			return Overview{}, err
		}
		item := entry(key)
		item.WetSupported, item.WetEnabled = true, effective.wet
		wetTypes[documentKey] = key
	}
	rows, err = r.pool.Query(ctx, `
		SELECT document_key, document_date::text FROM bast_wet_signature_scans
		WHERE schedule_id::text=$1 ORDER BY document_date
	`, scheduleID)
	if err != nil {
		return Overview{}, fmt.Errorf("overview wet scans: %w", err)
	}
	for rows.Next() {
		var documentKey, date string
		if err := rows.Scan(&documentKey, &date); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan overview wet scan: %w", err)
		}
		if key, ok := wetTypes[documentKey]; ok {
			item := entry(key)
			item.WetScans = append(item.WetScans, date)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Overview{}, fmt.Errorf("overview wet scans: %w", err)
	}
	return result, nil
}

func collectOverview(rows pgx.Rows, entryOf func(string) *OverviewEntry) error {
	defer rows.Close()
	for rows.Next() {
		var documentType, date string
		var active bool
		if err := rows.Scan(&documentType, &date, &active); err != nil {
			return fmt.Errorf("scan overview: %w", err)
		}
		item := entryOf(documentType)
		if item == nil {
			continue
		}
		if active {
			item.Dates[date] = OverviewFinal
			item.Final++
		} else {
			item.Dates[date] = OverviewStale
			item.Stale++
		}
	}
	return rows.Err()
}
