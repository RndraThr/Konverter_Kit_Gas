package bast

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestGroupDailyRecapVariantsKeepsFirstAppearanceOrder(t *testing.T) {
	recipients := []DailyRecapRecipient{
		{MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "5.5 HP", MachineFuelType: "Bensin"},
		{MachineBrand: "YANMAR", MachineType: "TF", MachinePower: "6.5 HP", MachineFuelType: "Solar"},
		{MachineBrand: "SHARK", MachineType: "SPWP", MachinePower: "5.5 HP", MachineFuelType: "Bensin"},
	}
	variants := groupDailyRecapVariants(recipients)
	if len(variants) != 2 {
		t.Fatalf("variants=%+v", variants)
	}
	if variants[0].Label != "Varian 1" || variants[0].Count != 2 || variants[1].Label != "Varian 2" || variants[1].Count != 1 {
		t.Fatalf("variants=%+v", variants)
	}
}

func TestValidateDailyRecapRequiresVerificationSnapshot(t *testing.T) {
	valid := []DailyRecapRecipient{{FullName: "A", MachineBrand: "B", MachineType: "T", MachineSerial: "S", MachinePower: "P", MachineFuelType: "F"}}
	if err := validateDailyRecapRecipients(valid); err != nil {
		t.Fatalf("valid err=%v", err)
	}
	if err := validateDailyRecapRecipients(nil); !errors.Is(err, ErrAggregateNoRecipients) {
		t.Fatalf("empty err=%v", err)
	}
	missingSerial := []DailyRecapRecipient{{FullName: "A", MachineBrand: "B", MachineType: "T", MachineSerial: "", MachinePower: "P", MachineFuelType: "F"}}
	if err := validateDailyRecapRecipients(missingSerial); !errors.Is(err, ErrVerificationSnapshotIncomplete) {
		t.Fatalf("serial err=%v", err)
	}
}

func TestValidateDailyRecapSettingsRequiresPertaminaRep(t *testing.T) {
	complete := ScheduleSettings{HandoverLocation: "Lokasi", AgricultureOfficeName: "Dinas", AgricultureOfficeNIP: "NIP", InstallerName: "Pelaksana", SupervisorName: "Pengawas", PertaminaRepName: "Pertamina"}
	if err := validateDailyRecapSettings(complete); err != nil {
		t.Fatalf("complete err=%v", err)
	}
	missing := complete
	missing.PertaminaRepName = ""
	if err := validateDailyRecapSettings(missing); !errors.Is(err, ErrSignatoryRequired) {
		t.Fatalf("pertamina err=%v", err)
	}
}

func TestFormatDailyRecapFilename(t *testing.T) {
	got := formatDailyRecapFilename("Kabupaten Wajo", "2026-10-02", 1)
	if got != "REKAP HARIAN - KABUPATEN WAJO - 2026-10-02 - V1.pdf" {
		t.Fatalf("filename=%q", got)
	}
}

func TestRenderDailyRecapProducesPDF(t *testing.T) {
	snapshot := DailyRecapSnapshot{
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: dailyRecapSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas", PertaminaRepName: "Pertamina"},
		Recipients: []dailyRecapRecipientSnapshot{
			{SlotNumber: 1, FullName: "Siti Aminah", FarmerCardNumber: "KP-01", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachineSerial: "M-001", MachinePower: "5.5 HP", MachineFuelType: "Bensin"},
		},
		Variants:   []DailyRecapVariant{{Label: "Varian 1", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 1}},
		GrandTotal: 1,
	}
	rendered, err := RenderDailyRecap(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.PDF) < 5 || string(rendered.PDF[:4]) != "%PDF" {
		t.Fatalf("not a PDF")
	}
	if rendered.PageCount < 2 {
		t.Fatalf("page count=%d", rendered.PageCount)
	}
}

func TestRenderDailyRecapIncludesProcurementHeadingAndUnderlinedTitle(t *testing.T) {
	snapshot := DailyRecapSnapshot{
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: dailyRecapSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas", PertaminaRepName: "Pertamina"},
		Recipients: []dailyRecapRecipientSnapshot{
			{SlotNumber: 1, FullName: "Siti Aminah", FarmerCardNumber: "KP-01", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachineSerial: "M-001"},
		},
		Variants:   []DailyRecapVariant{{Label: "Varian 1", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 1}},
		GrandTotal: 1,
	}
	rendered, err := RenderDailyRecap(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"PENGADAAN BARANG", "LIQUEFIED", "PETROLEUM GAS", "TAHUN ANGGARAN 2026", "PT PERTAMINA PATRA NIAGA", dailyRecapTitle, "Grand Total" /* label pada baris varian pertama, sesuai kasing dokumen referensi */} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered daily recap is missing %q", want)
		}
	}
}

// TestRenderDailyRecapLongRowsNeverOverflowPageBottom guards against rows
// (including Grand Total rows) being drawn past the page margin: row height
// must be checked before the row is drawn, not after.
func TestRenderDailyRecapLongRowsNeverOverflowPageBottom(t *testing.T) {
	recipients := make([]dailyRecapRecipientSnapshot, 0, 40)
	variants := make([]DailyRecapVariant, 0, 6)
	for i := 0; i < 40; i++ {
		brand := fmt.Sprintf("MEREK PANJANG NOMOR %d", i%6)
		recipients = append(recipients, dailyRecapRecipientSnapshot{
			SlotNumber: i + 1, FullName: fmt.Sprintf("Penerima Dengan Nama Sangat Panjang Nomor %d", i+1),
			FarmerCardNumber: fmt.Sprintf("KP-%04d", i+1), MachineBrand: brand,
			MachineType: "TIPE MESIN DENGAN DESKRIPSI YANG CUKUP PANJANG", MachineSerial: fmt.Sprintf("SN-%06d", i+1),
		})
	}
	for i := 0; i < 6; i++ {
		variants = append(variants, DailyRecapVariant{Label: fmt.Sprintf("Varian %d", i+1), MachineBrand: fmt.Sprintf("MEREK PANJANG NOMOR %d", i), MachineType: "TIPE MESIN DENGAN DESKRIPSI YANG CUKUP PANJANG", Count: 6})
	}
	snapshot := DailyRecapSnapshot{
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: dailyRecapSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas", PertaminaRepName: "Pertamina"},
		Recipients:  recipients,
		Variants:    variants,
		GrandTotal:  40,
	}
	rendered, err := RenderDailyRecap(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.PageCount < 3 {
		t.Fatalf("expected many long rows to force pagination, got page count=%d", rendered.PageCount)
	}
}

// TestRenderDailyRecapSignatureNamesWrapInsteadOfOverlapping guards against
// long signatory names in the narrow 4-column signature grid overflowing
// into the neighboring column (dp3FitText must wrap to multiple lines once
// shrinking the font no longer makes the text fit).
func TestRenderDailyRecapSignatureNamesWrapInsteadOfOverlapping(t *testing.T) {
	snapshot := DailyRecapSnapshot{
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: dailyRecapSignatories{AgricultureOfficeName: "Dinas Pertanian Kabupaten Wajo", AgricultureOfficeNIP: "123", InstallerName: "Ahmad Fauzi", SupervisorName: "Andi Saputra Wijaya Kusuma", PertaminaRepName: "Rian Hidayat"},
		Recipients: []dailyRecapRecipientSnapshot{
			{SlotNumber: 1, FullName: "Siti Aminah", FarmerCardNumber: "KP-01", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachineSerial: "M-001"},
		},
		Variants:   []DailyRecapVariant{{Label: "Varian 1", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", Count: 1}},
		GrandTotal: 1,
	}
	rendered, err := RenderDailyRecap(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	// Kasus overlap sebelumnya menyambung nama kolom pertama langsung ke
	// label "Nama :" kolom berikutnya tanpa pemisah, mis. "WAJONama".
	for _, glued := range []string{"WAJONama", "KUSUMANama"} {
		if strings.Contains(content, glued) {
			t.Fatalf("signature names overlap into the next column: found %q in rendered text", glued)
		}
	}
}

func TestRenderDailyRecapUppercasesLegacyBusinessValues(t *testing.T) {
	snapshot := DailyRecapSnapshot{
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "Konsultan Jaya", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 16}},
		Signatories: dailyRecapSignatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Budi Santoso", SupervisorName: "Andi Saputra", PertaminaRepName: "Siti Rahma"},
		Recipients:  []dailyRecapRecipientSnapshot{{SlotNumber: 1, FullName: "Siti Aminah", FarmerCardNumber: "kp-01", MachineBrand: "Shark", MachineType: "Pompa Tani", MachineSerial: "m-001", MachinePower: "5.5 hp", MachineFuelType: "Bensin"}},
		Variants:    []DailyRecapVariant{{Label: "Varian Satu", MachineBrand: "Shark", MachineType: "Pompa Tani", Count: 1}},
		GrandTotal:  1,
	}
	rendered, err := RenderDailyRecap(snapshot, map[string][]byte{"logo-1": testPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	content := decodedPDFStreams(t, rendered.PDF)
	for _, want := range []string{"LAPANGAN DESA", "KONSULTAN JAYA", "SITI AMINAH", "POMPA TANI", "SATU", "BUDI SANTOSO", "ANDI SAPUTRA", "SITI RAHMA"} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered daily recap is missing uppercase legacy value %q", want)
		}
	}
	if strings.Contains(content, "Siti Aminah") || strings.Contains(content, "Pompa Tani") || strings.Contains(content, "Andi Saputra") {
		t.Fatal("rendered daily recap still contains mixed-case legacy business values")
	}
}
