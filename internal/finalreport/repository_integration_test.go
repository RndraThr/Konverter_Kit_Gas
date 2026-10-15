package finalreport

import (
	"context"
	"errors"
	"os"
	"testing"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func unrestrictedScope() auth.RegencyScope { return auth.RegencyScope{Unrestricted: true} }

func TestGetSettingsReturnsZeroValueWhenNoRowExists(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}

	settings, err := repo.GetSettings(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.LeaderCount != 0 {
		t.Fatalf("expected zero-value settings, got %+v", settings)
	}
}

func TestPutSettingsUpsertsAndRejectsOutOfScopeRegency(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	actor := auth.Principal{UserID: ""}

	saved, err := repo.PutSettings(ctx, actor, Settings{ProgramID: programID, RegencyID: regencyID, LeaderCount: 1, AdminCount: 2, TechnicianCount: 2, HelperCount: 2}, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.LeaderCount != 1 || saved.AdminCount != 2 {
		t.Fatalf("got %+v", saved)
	}

	scoped := auth.RegencyScope{Unrestricted: false, RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}
	if _, err := repo.PutSettings(ctx, actor, Settings{ProgramID: programID, RegencyID: regencyID, LeaderCount: 9}, scoped); err == nil {
		t.Fatal("expected out-of-scope error, got nil")
	}
}

func TestReplacePhotoPicksRejectsPhotoFromAnotherRegency(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyA, regencyB string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyA); err != nil {
		t.Fatalf("fixture regency a: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id OFFSET 1 LIMIT 1`).Scan(&regencyB); err != nil {
		t.Fatalf("fixture regency b: %v", err)
	}

	var mediaID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_media (regency_id, activity_type, storage_key, display_name, original_filename, media_type, mime_type, byte_size, checksum, source)
		VALUES ($1::uuid, 'rakor', $2, 'TEST-RAKOR', 'test.jpg', 'image', 'image/jpeg', 100, repeat('a',64), 'gallery')
		RETURNING id::text
	`, regencyB, "finalreport-test-"+t.Name()).Scan(&mediaID); err != nil {
		t.Fatalf("seed activity_media: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM activity_media WHERE id=$1::uuid`, mediaID) })

	_, err := repo.ReplacePhotoPicks(ctx, programID, regencyA, SlideKeyRapatKoordinasi, []string{mediaID}, unrestrictedScope())
	if !errors.Is(err, ErrPhotoOutOfScope) {
		t.Fatalf("expected ErrPhotoOutOfScope, got %v", err)
	}
}
