// Package seeds holds idempotent data seeds that restore working data after a database reset.
package seeds

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed konkit_2026.sql
var konkit2026 string

// Result counts the KONKIT-2026 rows present after seeding.
type Result struct {
	Zones       int
	Assignments int
	Schedules   int
	Logos       int
}

// SeedKonkit2026 inserts whatever is missing of the KONKIT-2026 Persiapan data and reports what
// the program now has.
func SeedKonkit2026(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, konkit2026); err != nil {
		return Result{}, fmt.Errorf("seed KONKIT-2026: %w", err)
	}
	var result Result
	err = tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM program_zones z WHERE z.program_id = p.id),
			(SELECT count(*) FROM program_regency_assignments a WHERE a.program_id = p.id),
			(SELECT count(*) FROM program_schedules s WHERE s.program_id = p.id),
			(SELECT count(*) FROM program_ba_logo_assets l WHERE l.program_id = p.id)
		FROM programs p WHERE p.code = 'KONKIT-2026'`).
		Scan(&result.Zones, &result.Assignments, &result.Schedules, &result.Logos)
	if err != nil {
		return Result{}, fmt.Errorf("count KONKIT-2026 seed: %w", err)
	}
	return result, tx.Commit(ctx)
}
