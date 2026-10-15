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
