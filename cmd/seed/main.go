// Command seed restores the KONKIT-2026 Persiapan data (program, zones, regency assignments,
// schedules) after the database has been emptied. It only inserts missing rows.
package main

import (
	"context"
	"fmt"
	"log"

	"konkit/internal/config"
	"konkit/internal/database"
	"konkit/internal/database/seeds"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	result, err := seeds.SeedKonkit2026(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("KONKIT-2026 siap: %d zona, %d kabupaten, %d jadwal, %d logo\n", result.Zones, result.Assignments, result.Schedules, result.Logos)
}
