package bast

import (
	"errors"
	"testing"
)

func TestBuildRakordaSnapshotValidatesInput(t *testing.T) {
	ctx := DP3Context{ScheduleID: "schedule-1", ProgramID: "program-1", ProgramType: "farmer", RegencyName: "Wajo", ProvinceName: "Sulawesi Selatan", ZoneName: "Zona 4", FiscalYear: 2026, HasActiveLogo: true}
	settings := ScheduleSettings{RakordaLocation: "Aula Kantor Bupati", RakordaRowCount: 45}

	snapshot, err := buildRakordaSnapshot(ctx, settings, nil, "2026-10-03")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Location != "AULA KANTOR BUPATI" || snapshot.RowCount != 45 || snapshot.RegencyName != "WAJO" || snapshot.ZoneName != "ZONA 4" {
		t.Fatalf("snapshot=%+v", snapshot)
	}

	settings.RakordaRowCount = 4
	if _, err := buildRakordaSnapshot(ctx, settings, nil, "2026-10-03"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}

	settings.RakordaRowCount = 45
	placeholderCtx := ctx
	placeholderCtx.ZonePlaceholder = true
	if _, err := buildRakordaSnapshot(placeholderCtx, settings, nil, "2026-10-03"); !errors.Is(err, ErrZoneNotConfigured) {
		t.Fatalf("err=%v", err)
	}
}

func TestRenderRakordaProducesNumberedMultiPageAttendanceSheet(t *testing.T) {
	snapshot := RakordaSnapshot{
		DocumentDate: "2026-10-03", Location: "AULA KANTOR BUPATI",
		RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026,
		RowCount: 45, Logos: []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
	}
	rendered, err := RenderRakorda(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount != 2 {
		t.Fatalf("page_count=%d, want 2", rendered.PageCount)
	}
	if len(rendered.PDF) < 1000 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("invalid PDF, len=%d", len(rendered.PDF))
	}
}
