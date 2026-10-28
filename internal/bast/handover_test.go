package bast

import (
	"context"
	"errors"
	"testing"
)

func handoverSnapshot(brand, typ string) []byte {
	return []byte(`{"equipment":{"machine_brand":"` + brand + `","machine_type":"` + typ + `"}}`)
}

func TestAssignHandoverPrefersOverrideThenVillageThenDistrict(t *testing.T) {
	locations := []HandoverLocation{
		{ID: "a", Name: "Balai Desa Tempe", Areas: []HandoverArea{{District: "TEMPE"}}},
		{ID: "b", Name: "Lapangan Sabbangparu", Areas: []HandoverArea{{District: "Tempe", Village: " Maddukelleng "}}},
	}
	cases := []struct {
		name      string
		recipient handoverRecipient
		want      string
		source    string
	}{
		{"village beats district", handoverRecipient{District: "tempe", Village: "maddukelleng"}, "b", HandoverByVillage},
		{"district", handoverRecipient{District: "Tempe", Village: "Siengkang"}, "a", HandoverByDistrict},
		{"override", handoverRecipient{District: "Tempe", Village: "Maddukelleng", Override: "a"}, "a", HandoverByOverride},
		{"unknown override falls back", handoverRecipient{District: "Tempe", Override: "gone"}, "a", HandoverByDistrict},
		{"unassigned", handoverRecipient{District: "Sabbangparu"}, "", ""},
	}
	for _, tc := range cases {
		got, source := assignHandover(locations, tc.recipient)
		if got != tc.want || source != tc.source {
			t.Fatalf("%s: got %q/%q want %q/%q", tc.name, got, source, tc.want, tc.source)
		}
	}
	if got, source := assignHandover(locations[:1], handoverRecipient{District: "Lain"}); got != "a" || source != HandoverBySingle {
		t.Fatalf("single location should take everyone, got %q/%q", got, source)
	}
}

func TestNormalizeHandoverInputsRejectsDuplicateNames(t *testing.T) {
	if _, err := normalizeHandoverInputs([]HandoverLocationInput{{Name: "Balai  Desa"}, {Name: "balai desa"}}); !errors.Is(err, ErrHandoverLocationInvalid) {
		t.Fatalf("duplicate err=%v", err)
	}
	if _, err := normalizeHandoverInputs([]HandoverLocationInput{{Name: "  "}}); !errors.Is(err, ErrHandoverLocationInvalid) {
		t.Fatalf("blank err=%v", err)
	}
	got, err := normalizeHandoverInputs([]HandoverLocationInput{{Name: " Balai   Desa ", Areas: []HandoverArea{{District: "tempe"}, {District: " Tempe "}, {District: ""}, {District: "Tempe", Village: "siengkang"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "Balai Desa" || len(got[0].Areas) != 2 || got[0].Areas[1] != (HandoverArea{District: "TEMPE", Village: "SIENGKANG"}) {
		t.Fatalf("normalized=%+v", got)
	}
}

func TestBuildHandoverViewSummarisesPerLocation(t *testing.T) {
	locations := []HandoverLocation{
		{ID: "a", Name: "Lokasi A", Areas: []HandoverArea{{District: "TEMPE"}}},
		{ID: "b", Name: "Lokasi B", Areas: []HandoverArea{{District: "SABBANGPARU"}}},
		{ID: "c", Name: "Lokasi C"},
	}
	recipients := []handoverRecipient{
		{SlotID: "1", District: "Tempe", Village: "X", Completed: true, LocalDate: "2026-10-02"},
		{SlotID: "2", District: "Tempe", Village: "Y", Completed: true, LocalDate: "2026-10-01"},
		{SlotID: "3", District: "Sabbangparu", Village: "Z"},
		{SlotID: "4", District: "Belawa", Village: "W", Completed: true, LocalDate: "2026-10-01"},
		{SlotID: "5", District: "Belawa", Village: "W"},
	}
	view := buildHandoverView("s", locations, recipients, map[string]handoverDocumentStatus{"a": {Active: true, ActiveDate: "2026-10-03"}, "b": {Stale: true}})
	a, b, c := view.Locations[0], view.Locations[1], view.Locations[2]
	if a.Recipients != 2 || a.Status != HandoverStatusFinal || a.DocumentDate != "2026-10-03" || len(a.Dates) != 2 || a.Dates[0] != "2026-10-01" {
		t.Fatalf("location a=%+v", a)
	}
	if b.Recipients != 0 || b.Assigned != 1 || b.Status != HandoverStatusStale {
		t.Fatalf("location b=%+v", b)
	}
	if c.Status != HandoverStatusEmpty {
		t.Fatalf("location c=%+v", c)
	}
	if view.Unassigned != 1 || view.UnassignedTotal != 2 {
		t.Fatalf("unassigned=%d total=%d", view.Unassigned, view.UnassignedTotal)
	}
	if len(view.Areas) != 4 || view.Areas[0].District != "BELAWA" || view.Areas[0].Recipients != 2 {
		t.Fatalf("areas=%+v", view.Areas)
	}
}

func TestClosingKabupatenRowsComeFromFinalHandoverDocuments(t *testing.T) {
	locations := []HandoverLocation{
		{ID: "a", Name: "Lokasi A", Areas: []HandoverArea{{District: "TEMPE"}}},
		{ID: "b", Name: "Lokasi B", Areas: []HandoverArea{{District: "SABBANGPARU"}}},
	}
	recipients := []handoverRecipient{
		{District: "Tempe", Completed: true, LocalDate: "2026-10-01", Snapshot: handoverSnapshot("SHARK", "SPWP")},
		{District: "Tempe", Completed: true, LocalDate: "2026-10-02", Snapshot: handoverSnapshot("SHARK", "SPWP")},
		{District: "Sabbangparu", Completed: true, LocalDate: "2026-10-01", Snapshot: handoverSnapshot("YANMAR", "TF")},
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
	recipients = append(recipients, handoverRecipient{District: "Belawa", Completed: true})
	if _, err := closingKabupatenRowsFromHandover(locations, recipients, snapshots, true); !errors.Is(err, ErrHandoverUnassigned) {
		t.Fatalf("unassigned err=%v", err)
	}
}

func TestClosingRawRowsForFiltersLocationAndSortsByDate(t *testing.T) {
	locations := []HandoverLocation{{ID: "a", Areas: []HandoverArea{{District: "TEMPE"}}}, {ID: "b"}}
	raw := closingRawRowsFor(locations, []handoverRecipient{
		{District: "Tempe", Completed: true, LocalDate: "2026-10-02"},
		{District: "Lain", Completed: true, LocalDate: "2026-10-01"},
		{District: "Tempe", Completed: true, LocalDate: "2026-10-01"},
		{District: "Tempe"},
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
