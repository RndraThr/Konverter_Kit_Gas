package seeds

import (
	"context"
	"os"
	"testing"

	"konkit/internal/database"
)

// The test database already carries KONKIT-2026 from migrations 00013-00022, which is exactly the
// state the seed restores, so seeding it again must change nothing.
func TestSeedKonkit2026IsIdempotent(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for run := 1; run <= 2; run++ {
		result, err := SeedKonkit2026(ctx, pool)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if result.Zones != 3 || result.Assignments != 46 || result.Schedules != 46 {
			t.Fatalf("run %d: got %+v, want 3 zones, 46 assignments, 46 schedules", run, result)
		}
	}

	var zone1, zone2, quota int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE z.code = 'ZONA-1'), count(*) FILTER (WHERE z.code = 'ZONA-2'),
			(SELECT sum(s.slot_quota) FROM program_schedules s WHERE s.program_id = p.id)
		FROM programs p
		JOIN program_regency_assignments a ON a.program_id = p.id
		JOIN program_zones z ON z.id = a.zone_id
		WHERE p.code = 'KONKIT-2026'
		GROUP BY p.id`).Scan(&zone1, &zone2, &quota)
	if err != nil {
		t.Fatal(err)
	}
	if zone1 != 23 || zone2 != 23 || quota != 9240 {
		t.Fatalf("zona 1 = %d, zona 2 = %d, quota = %d; want 23, 23, 9240", zone1, zone2, quota)
	}
}
