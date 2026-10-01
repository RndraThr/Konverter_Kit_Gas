package bast

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
)

func TestBrandingRepositoryStoresAndOrdersLogos(t *testing.T) {
	pool := bastIntegrationPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	repo := NewRepository(pool)
	var programID, otherProgramID string
	mustQuery := func(query string, args []any, dest *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, query, args...).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Branding BA Test','farmer',2026,'active') RETURNING id::text`, []any{"BRAND-" + suffix}, &programID)
	mustQuery(`INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Branding BA Other','farmer',2026,'active') RETURNING id::text`, []any{"BROTH-" + suffix}, &otherProgramID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM program_ba_logo_assets WHERE program_id=ANY($1)`, []string{programID, otherProgramID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM programs WHERE id=ANY($1)`, []string{programID, otherProgramID})
	})

	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "branding-test"}

	// GetProgramCode validates existence.
	if code, err := repo.GetProgramCode(ctx, programID); err != nil || code != "BRAND-"+suffix {
		t.Fatalf("GetProgramCode = %q, %v", code, err)
	}
	if _, err := repo.GetProgramCode(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing program, got %v", err)
	}

	// Insert two logos out of order; another program gets its own logo.
	second, _, err := repo.SaveLogo(ctx, actor, LogoAsset{ProgramID: programID, SlotCode: "right", StorageKey: "key-right-" + suffix, OriginalFilename: "right.png", MimeType: "image/png", ByteSize: 10, Checksum: strings.Repeat("a", 64), SortOrder: 2, MaxWidthMM: 35, MaxHeightMM: 18}, meta)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := repo.SaveLogo(ctx, actor, LogoAsset{ProgramID: programID, SlotCode: "left", StorageKey: "key-left-" + suffix, OriginalFilename: "left.png", MimeType: "image/png", ByteSize: 10, Checksum: strings.Repeat("b", 64), SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 18}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.SaveLogo(ctx, actor, LogoAsset{ProgramID: otherProgramID, SlotCode: "left", StorageKey: "key-other-" + suffix, OriginalFilename: "o.png", MimeType: "image/png", ByteSize: 10, Checksum: strings.Repeat("c", 64), SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 18}, meta); err != nil {
		t.Fatal(err)
	}

	// ListLogos is scoped to the program and ordered by sort_order.
	logos, err := repo.ListLogos(ctx, programID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logos) != 2 || logos[0].ID != first.ID || logos[1].ID != second.ID {
		t.Fatalf("expected [left,right] for program, got %+v", logos)
	}

	// Upsert replaces the blob for the same slot and reports the old key.
	replaced, oldKey, err := repo.SaveLogo(ctx, actor, LogoAsset{ProgramID: programID, SlotCode: "left", StorageKey: "key-left2-" + suffix, OriginalFilename: "left2.png", MimeType: "image/jpeg", ByteSize: 20, Checksum: strings.Repeat("d", 64), SortOrder: 1, MaxWidthMM: 40, MaxHeightMM: 20}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if oldKey != "key-left-"+suffix {
		t.Fatalf("expected old key returned, got %q", oldKey)
	}
	if replaced.ID != first.ID || replaced.MimeType != "image/jpeg" {
		t.Fatalf("expected in-place upsert, got %+v", replaced)
	}

	// UpdateLogo flips visibility and ordering.
	updated, err := repo.UpdateLogo(ctx, actor, LogoPatchInput{ID: second.ID, ProgramID: programID, SortOrder: 5, MaxWidthMM: 30, MaxHeightMM: 15, IsVisible: false}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if updated.IsVisible || updated.SortOrder != 5 {
		t.Fatalf("expected hidden sort=5, got %+v", updated)
	}

	// UpdateLogo across programs is rejected.
	if _, err := repo.UpdateLogo(ctx, actor, LogoPatchInput{ID: second.ID, ProgramID: otherProgramID, SortOrder: 1, MaxWidthMM: 30, MaxHeightMM: 15, IsVisible: true}, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-program update, got %v", err)
	}
}
