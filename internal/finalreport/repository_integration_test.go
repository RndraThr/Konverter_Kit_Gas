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
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
	})

	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "finalreport-test"}
	first, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum1", "key1", unrestrictedScope(), meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("got version=%d, want 1", first.Version)
	}

	second, err := repo.InsertDocumentVersion(ctx, actor, programID, regencyID, "LAPORAN AKHIR - A.pptx", "checksum2", "key2", unrestrictedScope(), meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("got version=%d, want 2", second.Version)
	}

	docs, err := repo.ListDocuments(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 2 || docs[0].Status != "active" || docs[1].Status != "superseded" {
		t.Fatalf("got %+v", docs)
	}
	if docs[1].StorageKey != "key1" {
		t.Fatalf("superseded row must keep its storage_key, got %q", docs[1].StorageKey)
	}

	var auditRows int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE action='final_report.generated' AND resource_type='final_report_document' AND resource_id=$1
		  AND metadata->>'checksum'='checksum2' AND metadata->>'version'='2' AND user_agent='finalreport-test' AND host(ip_address)='127.0.0.1'
	`, second.ID).Scan(&auditRows); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("expected 1 audit row for version 2, got %d", auditRows)
	}
}

func TestGetDocumentRespectsScope(t *testing.T) {
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
		pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
	})
	doc, err := repo.InsertDocumentVersion(ctx, auth.Principal{}, programID, regencyID, "LAPORAN AKHIR - B.pptx", "checksum", "key-get", unrestrictedScope(), auth.ClientMeta{})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := repo.GetDocument(ctx, doc.ID, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	if err != nil {
		t.Fatalf("in-scope get: %v", err)
	}
	if got.StorageKey != "key-get" || got.Filename != "LAPORAN AKHIR - B.pptx" {
		t.Fatalf("got %+v", got)
	}
	if _, err := repo.GetDocument(ctx, doc.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-scope: expected ErrNotFound, got %v", err)
	}
	if _, err := repo.GetDocument(ctx, "00000000-0000-0000-0000-000000000000", unrestrictedScope()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: expected ErrNotFound, got %v", err)
	}
	if _, err := repo.GetDocument(ctx, "not-a-uuid", unrestrictedScope()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id: expected ErrNotFound, got %v", err)
	}
}

func seedActivityMedia(t *testing.T, pool *pgxpool.Pool, programID *string, regencyID, mediaType, mimeType string) string {
	t.Helper()
	ctx := context.Background()
	var mediaID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_media (program_id, regency_id, activity_type, storage_key, display_name, original_filename, media_type, mime_type, byte_size, checksum, source)
		VALUES ($1::uuid, $2::uuid, 'rakor', 'finalreport-test-'||gen_random_uuid()::text, 'TEST-RAKOR', 'test', $3, $4, 100, repeat('a',64), 'gallery')
		RETURNING id::text
	`, programID, regencyID, mediaType, mimeType).Scan(&mediaID); err != nil {
		t.Fatalf("seed activity_media: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE activity_media_id=$1::uuid`, mediaID)
		pool.Exec(ctx, `DELETE FROM activity_media WHERE id=$1::uuid`, mediaID)
	})
	return mediaID
}

func TestReplacePhotoPicksRejectsPhotoFromAnotherProgram(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programA, programB, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs ORDER BY id LIMIT 1`).Scan(&programA); err != nil {
		t.Fatalf("fixture program a: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs ORDER BY id OFFSET 1 LIMIT 1`).Scan(&programB); err != nil {
		t.Skip("test DB has only one program")
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programA, regencyID)
	})

	otherProgram := seedActivityMedia(t, pool, &programB, regencyID, "image", "image/jpeg")
	if _, err := repo.ReplacePhotoPicks(ctx, programA, regencyID, SlideKeyRapatKoordinasi, []string{otherProgram}, unrestrictedScope()); !errors.Is(err, ErrPhotoOutOfScope) {
		t.Fatalf("photo from another program: expected ErrPhotoOutOfScope, got %v", err)
	}

	sameProgram := seedActivityMedia(t, pool, &programA, regencyID, "image", "image/jpeg")
	noProgram := seedActivityMedia(t, pool, nil, regencyID, "image", "image/png")
	picks, err := repo.ReplacePhotoPicks(ctx, programA, regencyID, SlideKeyRapatKoordinasi, []string{sameProgram, noProgram}, unrestrictedScope())
	if err != nil {
		t.Fatalf("same-program and program-less photos must be accepted: %v", err)
	}
	if len(picks) != 2 {
		t.Fatalf("expected 2 picks, got %+v", picks)
	}
}

func TestReplacePhotoPicksRejectsNonJPEGPNGMedia(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs ORDER BY id LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	for _, kind := range [][2]string{{"image", "image/webp"}, {"video", "video/mp4"}} {
		mediaID := seedActivityMedia(t, pool, &programID, regencyID, kind[0], kind[1])
		if _, err := repo.ReplacePhotoPicks(ctx, programID, regencyID, SlideKeyRapatKoordinasi, []string{mediaID}, unrestrictedScope()); !errors.Is(err, ErrPhotoTypeInvalid) {
			t.Fatalf("%s: expected ErrPhotoTypeInvalid, got %v", kind[1], err)
		}
	}
}

func TestListPhotoPicksExcludesNonJPEGPNGMedia(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs ORDER BY id LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	video := seedActivityMedia(t, pool, &programID, regencyID, "video", "video/mp4")
	if _, err := pool.Exec(ctx, `
		INSERT INTO final_report_photo_picks (program_id, regency_id, slide_key, activity_media_id, sort_order)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid, 0)
	`, programID, regencyID, SlideKeySosialisasi, video); err != nil {
		t.Fatalf("seed photo pick: %v", err)
	}
	picks, err := repo.ListPhotoPicks(ctx, programID, regencyID, SlideKeySosialisasi, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range picks {
		if p.ActivityMediaID == video {
			t.Fatalf("video pick must be excluded, got %+v", p)
		}
	}
}

func TestGetPreviewSourceDataFillsBlanksWhenDataMissing(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `
		SELECT ps.program_id::text, ps.regency_id::text FROM program_schedules ps
		LEFT JOIN bast_schedule_settings bs ON bs.schedule_id = ps.id
		WHERE bs.handover_location IS NULL OR bs.handover_location = ''
		LIMIT 1
	`).Scan(&programID, &regencyID); err != nil {
		t.Skip("no schedule without handover_location in test DB; skipping blank-field assertion")
	}

	data, err := repo.GetPreviewSourceData(ctx, programID, regencyID, unrestrictedScope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.handoverLocation != "" {
		t.Fatalf("expected empty handoverLocation from source data, got %q", data.handoverLocation)
	}
}
