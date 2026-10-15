package finalreport

import (
	"context"
	"errors"
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
	svc := NewService(repo, stubProgramContexts{})
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
