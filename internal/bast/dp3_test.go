package bast

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDP3RecipientsRequiresMachineData(t *testing.T) {
	recipients := []DP3Recipient{
		{FullName: "Siti", NIK: "7312345678901234", Address: "Jalan Sawah", MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "5.5 HP", MachineFuelType: "Bensin"},
	}
	if err := validateDP3Recipients(recipients); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := validateDP3Recipients(nil); !errors.Is(err, ErrAggregateNoRecipients) {
		t.Fatalf("empty err=%v", err)
	}
	if err := validateDP3Recipients([]DP3Recipient{{FullName: "", NIK: "7312345678901234", Address: "X", MachineBrand: "B", MachineType: "T", MachinePower: "P", MachineFuelType: "F"}}); !errors.Is(err, ErrRecipientIdentityIncomplete) {
		t.Fatalf("identity err=%v", err)
	}
	if err := validateDP3Recipients([]DP3Recipient{{FullName: "A", NIK: "7312345678901234", Address: "X", MachineBrand: "", MachineType: "T", MachinePower: "P", MachineFuelType: "F"}}); !errors.Is(err, ErrAllocationSnapshotIncomplete) {
		t.Fatalf("machine err=%v", err)
	}
	if err := validateDP3Recipients([]DP3Recipient{{FullName: "A", NIK: "7312345678901234", Address: "X", MachineBrand: "B", MachineType: "T", MachinePower: "", MachineFuelType: "F"}}); !errors.Is(err, ErrMachinePowerRequired) {
		t.Fatalf("power err=%v", err)
	}
}

func TestDP3ValidationStatusMapping(t *testing.T) {
	cases := map[error]string{
		ErrZoneNotConfigured:              "zone_not_configured",
		ErrBrandingNotConfigured:          "ba_logo_required",
		ErrHandoverLocationRequired:       "handover_location_required",
		ErrSignatoryRequired:              "signatory_required",
		ErrAggregateNoRecipients:          "no_recipients",
		ErrRecipientIdentityIncomplete:    "recipient_identity_incomplete",
		ErrAllocationSnapshotIncomplete:   "allocation_snapshot_incomplete",
		ErrVerificationSnapshotIncomplete: "verification_snapshot_incomplete",
		ErrMachinePowerRequired:           "machine_power_required",
		ErrMachineFuelRequired:            "machine_fuel_required",
	}
	for err, want := range cases {
		if got := dp3ValidationStatus(err); got != want {
			t.Fatalf("status(%v)=%q want %q", err, got, want)
		}
	}
	if got := dp3ValidationStatus(nil); got != "ready" {
		t.Fatalf("nil status=%q", got)
	}
}

func TestFormatDP3Filename(t *testing.T) {
	got := formatDP3Filename("  Kabupaten   Wajo ", "2026-10-02", 2)
	if got != "DP3 - KABUPATEN WAJO - 2026-10-02 - V2.pdf" {
		t.Fatalf("filename=%q", got)
	}
}

func TestRenderDP3ProducesPDF(t *testing.T) {
	snapshot := DP3Snapshot{
		DocumentType: AggregateDocumentDP3, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}},
		Signatories: dp3Signatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas"},
		Recipients: []dp3RecipientSnapshot{
			{FullName: "Siti Aminah", NIK: "7312345678901234", Address: "Jalan Sawah", Village: "Tempe", District: "Sabbangparu", Regency: "Wajo", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachinePower: "5.5 HP", MachineFuelType: "Bensin", Source: "allocation_snapshot"},
		},
	}
	rendered, err := RenderDP3(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.PDF) < 5 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("not a PDF: %q", rendered.PDF[:min(20, len(rendered.PDF))])
	}
	if rendered.PageCount < 2 {
		t.Fatalf("page count=%d, expected table + signature page", rendered.PageCount)
	}
}

func TestRenderDP3IncludesProcurementHeadingAndUnderlinedTitle(t *testing.T) {
	snapshot := DP3Snapshot{
		DocumentType: AggregateDocumentDP3, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}},
		Signatories: dp3Signatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas"},
		Recipients: []dp3RecipientSnapshot{
			{FullName: "Siti Aminah", NIK: "7312345678901234", Address: "Jalan Sawah", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachinePower: "5.5 HP", MachineFuelType: "Bensin", Source: "allocation_snapshot"},
		},
	}
	rendered, err := RenderDP3(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"PENGADAAN BARANG", "LIQUEFIED", "PETROLEUM GAS", "TAHUN ANGGARAN 2026", "PT PERTAMINA PATRA NIAGA", dp3Title} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered DP3 is missing %q", want)
		}
	}
}

// TestRenderDP3LongRowsNeverOverflowPageBottom guards against rendering a
// table row whose bottom edge falls past the page margin: dp3RowHeight must
// be checked before the row is drawn, not after.
func TestRenderDP3LongRowsNeverOverflowPageBottom(t *testing.T) {
	longAddress := strings.Repeat("Jalan Sangat Panjang Sekali Nomor Sembilan Puluh Sembilan ", 3)
	recipients := make([]dp3RecipientSnapshot, 0, 12)
	for i := 0; i < 12; i++ {
		recipients = append(recipients, dp3RecipientSnapshot{
			FullName: fmt.Sprintf("Penerima Dengan Nama Sangat Panjang Nomor %d", i+1),
			NIK:      "7312345678901234", Address: longAddress,
			MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachinePower: "5.5 HP", MachineFuelType: "Bensin",
			Source: "allocation_snapshot",
		})
	}
	snapshot := DP3Snapshot{
		DocumentType: AggregateDocumentDP3, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}},
		Signatories: dp3Signatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas"},
		Recipients:  recipients,
	}
	rendered, err := RenderDP3(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	// Baris panjang wajib memaksa halaman tambahan (bukti tinggi dihitung
	// sebelum digambar): minimal 1 halaman tabel tambahan + 1 halaman TTD.
	if rendered.PageCount < 3 {
		t.Fatalf("expected long rows to force pagination, got page count=%d", rendered.PageCount)
	}
}

func TestRenderDP3UppercasesLegacyBusinessValues(t *testing.T) {
	snapshot := DP3Snapshot{
		DocumentType: AggregateDocumentDP3, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "Konsultan Jaya", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}},
		Signatories: dp3Signatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Budi Santoso", SupervisorName: "Andi Saputra"},
		Recipients: []dp3RecipientSnapshot{{
			FullName: "Siti Aminah", NIK: "7312345678901234", Address: "Jalan Sawah", Village: "Desa Baru", District: "Sabbangparu", Regency: "Wajo",
			MachineBrand: "Shark", MachineType: "Pompa Tani", MachinePower: "5.5 hp", MachineFuelType: "Bensin", Source: "allocation_snapshot",
		}},
	}
	rendered, err := RenderDP3(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"KABUPATEN WAJO", "LAPANGAN DESA", "KONSULTAN JAYA", "SITI AMINAH", "JALAN SAWAH", "DESA BARU", "POMPA TANI", "BUDI SANTOSO", "ANDI SAPUTRA"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered DP3 is missing uppercase legacy value %q", want)
		}
	}
	if strings.Contains(content, "Siti Aminah") || strings.Contains(content, "Jalan Sawah") || strings.Contains(content, "Andi Saputra") {
		t.Fatal("rendered DP3 still contains mixed-case legacy business values")
	}
}
