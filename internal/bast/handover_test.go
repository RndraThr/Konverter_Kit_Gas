package bast

import (
	"context"
	"errors"
	"testing"
)

func handoverSnapshot(brand, typ string) []byte {
	return []byte(`{"equipment":{"machine_brand":"` + brand + `","machine_type":"` + typ + `"}}`)
}

func TestBuildHandoverViewSummarisesPerLocation(t *testing.T) {
	locations := []HandoverLocation{{ID: "a", Name: "Lokasi A"}, {ID: "b", Name: "Lokasi B"}, {ID: "c", Name: "Aula Kosong"}}
	recipients := []handoverRecipient{
		{SlotID: "1", LocationID: "a", Completed: true, LocalDate: "2026-10-02"},
		{SlotID: "2", LocationID: "a", Completed: true, LocalDate: "2026-10-01"},
		{SlotID: "3", LocationID: "b"},
		{SlotID: "4", SlotNumber: 4, Name: "Budi", Completed: true, LocalDate: "2026-10-01"},
		{SlotID: "5"},
		{SlotID: "6", LocationID: "gone", Completed: true},
	}
	view := buildHandoverView("s", locations, recipients, map[string]handoverDocumentStatus{"a": {Active: true, ActiveDate: "2026-10-03"}, "b": {Stale: true}})
	a, b, c := view.Locations[0], view.Locations[1], view.Locations[2]
	if a.ID != "a" || a.Recipients != 2 || a.Status != HandoverStatusFinal || a.DocumentDate != "2026-10-03" || len(a.Dates) != 2 || a.Dates[0] != "2026-10-01" {
		t.Fatalf("location a=%+v", a)
	}
	if b.ID != "b" || b.Recipients != 0 || b.Assigned != 1 || b.Status != HandoverStatusStale {
		t.Fatalf("location b=%+v", b)
	}
	// Locations without recipients on this schedule come last.
	if c.ID != "c" || c.Status != HandoverStatusEmpty {
		t.Fatalf("location c=%+v", c)
	}
	if len(view.Unassigned) != 2 || view.Unassigned[0].SlotNumber != 4 || view.Unassigned[0].Name != "Budi" {
		t.Fatalf("unassigned=%+v", view.Unassigned)
	}
}

func TestClosingKabupatenRowsComeFromFinalHandoverDocuments(t *testing.T) {
	locations := []HandoverLocation{{ID: "b", Name: "Lokasi B"}, {ID: "a", Name: "Lokasi A"}}
	recipients := []handoverRecipient{
		{LocationID: "a", Completed: true, LocalDate: "2026-10-01", Snapshot: handoverSnapshot("SHARK", "SPWP")},
		{LocationID: "a", Completed: true, LocalDate: "2026-10-02", Snapshot: handoverSnapshot("SHARK", "SPWP")},
		{LocationID: "b", Completed: true, LocalDate: "2026-10-01", Snapshot: handoverSnapshot("YANMAR", "TF")},
	}
	snapshots := map[string]ClosingTitikSerahSnapshot{
		"a": {HandoverLocation: "LOKASI A FINAL", GrandTotal: 2, Rows: []ClosingRow{
			{LocalDate: "2026-10-01", MachineBrand: "SHARK", MachineType: "SPWP", Count: 1},
			{LocalDate: "2026-10-02", MachineBrand: "SHARK", MachineType: "SPWP", Count: 1},
		}},
	}
	if _, err := closingKabupatenRowsFromHandover(locations, recipients, snapshots, true); !errors.Is(err, ErrHandoverClosingIncomplete) {
		t.Fatalf("strict without final b err=%v", err)
	}
	rows, err := closingKabupatenRowsFromHandover(locations, recipients, snapshots, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0] != (ClosingKabupatenRow{Location: "LOKASI A FINAL", MachineBrand: "SHARK", MachineType: "SPWP", Count: 2, Final: true}) || rows[1].Location != "Lokasi B" || rows[1].Final {
		t.Fatalf("rows=%+v", rows)
	}
	snapshots["b"] = ClosingTitikSerahSnapshot{HandoverLocation: "LOKASI B", GrandTotal: 1, Rows: []ClosingRow{{MachineBrand: "YANMAR", MachineType: "TF", Count: 1}}}
	rows, err = closingKabupatenRowsFromHandover(locations, recipients, snapshots, true)
	if err != nil || len(rows) != 2 || !rows[1].Final {
		t.Fatalf("strict rows=%+v err=%v", rows, err)
	}
	// Final document no longer covers every recipient of the location.
	snapshots["b"] = ClosingTitikSerahSnapshot{GrandTotal: 5}
	if _, err := closingKabupatenRowsFromHandover(locations, recipients, snapshots, true); !errors.Is(err, ErrHandoverClosingIncomplete) {
		t.Fatalf("outdated final err=%v", err)
	}
	recipients = append(recipients, handoverRecipient{Completed: true})
	if _, err := closingKabupatenRowsFromHandover(locations, recipients, snapshots, true); !errors.Is(err, ErrHandoverUnassigned) {
		t.Fatalf("unassigned err=%v", err)
	}
}

func TestClosingRawRowsForFiltersLocationAndSortsByDate(t *testing.T) {
	raw := closingRawRowsFor([]handoverRecipient{
		{LocationID: "a", Completed: true, LocalDate: "2026-10-02"},
		{LocationID: "b", Completed: true, LocalDate: "2026-10-01"},
		{LocationID: "a", Completed: true, LocalDate: "2026-10-01"},
		{LocationID: "a"},
	}, "a")
	if len(raw) != 2 || raw[0].LocalDate != "2026-10-01" {
		t.Fatalf("raw=%+v", raw)
	}
}

func TestHandoverKeysAndContext(t *testing.T) {
	if closingDocumentType("x") != "closing_titik_serah:x" || closingWetKey("x") != "closing-titik-serah@x" || wetBaseKey("closing-titik-serah@x") != "closing-titik-serah" || wetBaseKey("dp3") != "dp3" {
		t.Fatal("unexpected handover keys")
	}
	ctx := WithHandoverLocation(context.Background(), " b ")
	locations := []HandoverLocation{{ID: "a"}, {ID: "b"}}
	if location, err := pickHandoverLocation(locations, handoverLocationFrom(ctx)); err != nil || location.ID != "b" {
		t.Fatalf("pick=%+v err=%v", location, err)
	}
	if _, err := pickHandoverLocation(locations, ""); !errors.Is(err, ErrHandoverLocationRequired) {
		t.Fatalf("ambiguous err=%v", err)
	}
	if location, err := pickHandoverLocation(locations[:1], ""); err != nil || location.ID != "a" {
		t.Fatalf("single pick=%+v err=%v", location, err)
	}
}

func TestFormatClosingFilenameIncludesLocation(t *testing.T) {
	if got := formatClosingFilename("Kabupaten Wajo", "Balai Desa/Tempe", "2026-10-03", 1); got != "CLOSING TITIK SERAH - KABUPATEN WAJO - BALAI DESA TEMPE - 2026-10-03 - V1.pdf" {
		t.Fatalf("filename=%q", got)
	}
}
