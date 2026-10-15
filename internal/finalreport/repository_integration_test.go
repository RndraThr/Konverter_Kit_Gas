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
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_settings WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
	})
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

func TestListPhotoPicksExcludesSoftDeletedPhoto(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}

	var mediaID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_media (regency_id, activity_type, storage_key, display_name, original_filename, media_type, mime_type, byte_size, checksum, source, status)
		VALUES ($1::uuid, 'rakor', $2, 'TEST-RAKOR-DELETED', 'test.jpg', 'image', 'image/jpeg', 100, repeat('a',64), 'gallery', 'deleted')
		RETURNING id::text
	`, regencyID, "finalreport-test-"+t.Name()).Scan(&mediaID); err != nil {
		t.Fatalf("seed activity_media: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM activity_media WHERE id=$1::uuid`, mediaID) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO final_report_photo_picks (program_id, regency_id, slide_key, activity_media_id, sort_order)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid, 0)
	`, programID, regencyID, SlideKeySosialisasi, mediaID); err != nil {
		t.Fatalf("seed photo pick: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE program_id=$1::uuid AND regency_id=$2::uuid AND slide_key=$3`, programID, regencyID, SlideKeySosialisasi)
	})

	picks, err := repo.ListPhotoPicks(ctx, programID, regencyID, SlideKeySosialisasi, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range picks {
		if p.ActivityMediaID == mediaID {
			t.Fatalf("expected soft-deleted photo %s to be excluded, got %+v", mediaID, p)
		}
	}
}

func TestInsertDocumentVersionIncrementsAndSupersedes(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: ""}

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID) })

	first, prevKey, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum1", "key1", unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Version != 1 || prevKey != "" {
		t.Fatalf("got version=%d prevKey=%q, want version=1 prevKey=\"\"", first.Version, prevKey)
	}

	second, prevKey, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum2", "key2", unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Version != 2 || prevKey != "key1" {
		t.Fatalf("got version=%d prevKey=%q, want version=2 prevKey=\"key1\"", second.Version, prevKey)
	}

	docs, err := repo.ListDocuments(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 2 || docs[0].Status != "active" || docs[1].Status != "superseded" {
		t.Fatalf("got %+v", docs)
	}
}
