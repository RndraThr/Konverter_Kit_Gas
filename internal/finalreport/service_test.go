package finalreport

import (
	"context"
	"errors"
	"io"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/programs"
)

type stubProgramContexts struct{ ctx programs.StorageContext }

func (s stubProgramContexts) ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error) {
	return s.ctx, nil
}

func TestPutSettingsRejectsNegativeCounts(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	_, err := svc.PutSettings(context.Background(), auth.Principal{}, Settings{ProgramID: "p", RegencyID: "r", LeaderCount: -1}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrCountNegative) {
		t.Fatalf("expected ErrCountNegative, got %v", err)
	}
}

func TestReplacePhotoPicksRejectsMoreThanSix(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	ids := []string{"1", "2", "3", "4", "5", "6", "7"}
	_, err := svc.ReplacePhotoPicks(context.Background(), "p", "r", SlideKeyRapatKoordinasi, ids, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrTooManyPicks) {
		t.Fatalf("expected ErrTooManyPicks, got %v", err)
	}
}

func TestPreviewFillsBlankWhenNoDataExists(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	svc := NewService(repo, stubProgramContexts{}, nil, "")
	ctx := context.Background()

	var programID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	// A regency with no program_schedules row at all for this program:
	var regencyID string
	if err := pool.QueryRow(ctx, `
		SELECT reg.id::text FROM regencies reg
		WHERE NOT EXISTS (SELECT 1 FROM program_schedules ps WHERE ps.program_id=$1::uuid AND ps.regency_id=reg.id)
		LIMIT 1
	`, programID).Scan(&regencyID); err != nil {
		t.Skip("every regency already has a schedule for this program in the test DB")
	}

	preview, err := svc.Preview(ctx, programID, regencyID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.RakordaDate != "Belum diisi" || preview.HandoverLocation != "Belum diisi" {
		t.Fatalf("expected blank placeholders, got %+v", preview)
	}
}

type fakeStorage struct {
	putCount int
	deleted  []string
}

func (f *fakeStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	f.putCount++
	data, _ := io.ReadAll(source)
	return key, int64(len(data)), "fakechecksum", nil
}
func (f *fakeStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func (f *fakeStorage) EnsureFolders(context.Context, [][]string) error { return nil }

func TestGenerateProducesSequentialVersions(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	storage := &fakeStorage{}
	svc := NewService(repo, stubProgramContexts{ctx: programs.StorageContext{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Test", ProgramID: "p", RegencyID: "r"}}, storage, "template/final_report_template.pptx")
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

	doc1, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	doc2, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if doc1.Version != 1 || doc2.Version != 2 {
		t.Fatalf("got versions %d, %d; want 1, 2", doc1.Version, doc2.Version)
	}
	if storage.putCount != 2 {
		t.Fatalf("expected 2 Put calls, got %d", storage.putCount)
	}
	if doc1.StorageKey == doc2.StorageKey {
		t.Fatalf("storage keys must differ across generations, both %q", doc1.StorageKey)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != doc1.StorageKey {
		t.Fatalf("expected only first key %q deleted, got %v", doc1.StorageKey, storage.deleted)
	}
}
