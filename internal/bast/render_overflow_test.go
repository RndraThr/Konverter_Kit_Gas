package bast

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Long-text stress renders: every free-text field of each BA sample is lengthened, once with
// long words and once with a long unbroken token (serial numbers, codes), and the PDF is written
// to BAST_STRESS_DIR so scripts/check_pdf_overflow.py can verify no text crosses a table border
// or the page edge. Without BAST_STRESS_DIR the renders still run, so a crash on long input fails.

const stressWords = "PANJANG SEKALI UNTUK MENGUJI PEMBUNGKUSAN TEKS DI DALAM TABEL AGAR TIDAK MELEWATI GARIS"

var stressTokenText = strings.Repeat("ABCDEFGHIJ", 6)

type stressMode string

const (
	stressPlain stressMode = "plain"
	stressLong  stressMode = "words"
	stressToken stressMode = "token"
)

func stressValue(original string, mode stressMode) string {
	switch mode {
	case stressToken:
		return strings.TrimSpace(original + " " + stressTokenText)
	case stressLong:
		return strings.TrimSpace(original + " " + stressWords)
	}
	return original
}

// structuralField reports string fields that are identifiers or formats rather than text a
// user types, so lengthening them would only break the render instead of testing it.
func structuralField(name string) bool {
	for _, marker := range []string{"Date", "ID", "Key", "Mime", "Code", "Status", "Type", "Source", "Checksum", "Number", "NIP", "Phone", "Code", "Prefix"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func lengthenStrings(value reflect.Value, mode stressMode, fieldName string) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			lengthenStrings(value.Elem(), mode, fieldName)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			if !field.IsExported() && value.Field(i).Kind() != reflect.Struct && value.Field(i).Kind() != reflect.Slice {
				continue
			}
			if !value.Field(i).CanSet() {
				continue
			}
			lengthenStrings(value.Field(i), mode, field.Name)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			lengthenStrings(value.Index(i), mode, fieldName)
		}
	case reflect.String:
		if value.CanSet() && !structuralField(fieldName) && value.String() != "" {
			value.SetString(stressValue(value.String(), mode))
		}
	}
}

func stressed[T any](sample T, mode stressMode) T {
	value := reflect.ValueOf(&sample).Elem()
	lengthenStrings(value, mode, "")
	return sample
}

func writeStressPDF(t *testing.T, name string, pdf []byte) {
	t.Helper()
	if len(pdf) == 0 {
		t.Fatalf("%s rendered an empty PDF", name)
	}
	dir := os.Getenv("BAST_STRESS_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".pdf"), pdf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStressRenderLongTextInEveryBA(t *testing.T) {
	logos := map[string][]byte{"logo": testPNG(t), "l1": testPNG(t), "l2": testPNG(t), "l3": testPNG(t), "l4": testPNG(t), "l5": testPNG(t), "logo-1": testPNG(t), "logo-key": testPNG(t)}
	for _, token := range []stressMode{stressPlain, stressLong, stressToken} {
		suffix := string(token)
		render := func(name string, fn func() ([]byte, error)) {
			t.Helper()
			pdf, err := fn()
			if err != nil {
				t.Fatalf("%s/%s: %v", name, suffix, err)
			}
			writeStressPDF(t, name+"-"+suffix, pdf)
		}
		render("ba-perorangan", func() ([]byte, error) {
			document := stressed(makeRenderDocument(1, 1), token)
			result, err := RenderPetaniBundle(BundleRenderInput{LocalDate: "2024-12-10", Documents: []RecipientDocument{document}, LogoBytes: logos})
			return result.PDF, err
		})
		render("dp3", func() ([]byte, error) {
			result, err := RenderDP3(stressed(sampleDP3Snapshot(), token), logos)
			return result.PDF, err
		})
		render("rekap-harian", func() ([]byte, error) {
			result, err := RenderDailyRecap(stressed(sampleDailyRecapSnapshot(), token), logos)
			return result.PDF, err
		})
		render("closing-titik-serah", func() ([]byte, error) {
			result, err := RenderClosingTitikSerah(stressed(sampleClosingSnapshot(), token), logos)
			return result.PDF, err
		})
		render("closing-kabupaten", func() ([]byte, error) {
			result, err := RenderClosingKabupaten(stressed(sampleClosingKabupatenSnapshot(), token), logos)
			return result.PDF, err
		})
		render("servis-berkala", func() ([]byte, error) {
			result, err := RenderServisBerkala(stressed(sampleServisBerkalaSnapshot(), token), logos)
			return result.PDF, err
		})
		render("tkdn", func() ([]byte, error) {
			result, err := RenderTKDN(stressed(sampleTKDNSnapshot(), token), logos)
			return result.PDF, err
		})
		render("pemeriksaan", func() ([]byte, error) {
			result, err := RenderPemeriksaan(stressed(samplePemeriksaanSnapshots()[:1], token), logos)
			return result.PDF, err
		})
		render("gas-lpg", func() ([]byte, error) {
			result, err := RenderGasElpiji([]GasElpijiSnapshot{stressed(sampleGasElpijiSnapshot("2026-12-02", 1250), token)}, logos)
			return result.PDF, err
		})
		activity := stressed(sampleActivitySnapshot(), token)
		render("rakorda", func() ([]byte, error) {
			result, err := RenderRakorda(activity, logos)
			return result.PDF, err
		})
		render("sosialisasi", func() ([]byte, error) {
			result, err := RenderSosialisasi(activity, logos)
			return result.PDF, err
		})
		render("training-10", func() ([]byte, error) {
			result, err := RenderTraining10(activity, logos)
			return result.PDF, err
		})
		render("training-100", func() ([]byte, error) {
			result, err := RenderTraining100(activity, logos)
			return result.PDF, err
		})
	}
}

func sampleDP3Snapshot() DP3Snapshot {
	return DP3Snapshot{
		DocumentType: AggregateDocumentDP3, DocumentDate: "2026-10-02", RegencyName: "Kabupaten Wajo",
		HandoverLocation: "Lapangan Desa", ConsultantCompanyName: "PT KSM", FiscalYear: 2026,
		Logos:       []LogoSnapshot{{AssetID: "logo-1", StorageKey: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 13}},
		Signatories: dp3Signatories{AgricultureOfficeName: "Dinas Pertanian", AgricultureOfficeNIP: "123", InstallerName: "Pelaksana", SupervisorName: "Pengawas"},
		Recipients: []dp3RecipientSnapshot{
			{FullName: "Siti Aminah", NIK: "7312345678901234", Address: "Jalan Sawah", Village: "Tempe", District: "Sabbangparu", Regency: "Wajo", MachineBrand: "SHARK", MachineType: "SPWP 80-30/3\"", MachinePower: "5.5 HP", MachineFuelType: "Bensin", Source: "allocation_snapshot"},
		},
	}
}

func sampleDailyRecapSnapshot() DailyRecapSnapshot {
	return DailyRecapSnapshot{
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
}

func sampleActivitySnapshot() RakordaSnapshot {
	return RakordaSnapshot{
		DocumentDate: "2026-10-03", Location: "BALAI DESA TEMPE",
		RegencyName: "WAJO", ProvinceName: "SULAWESI SELATAN", ZoneName: "ZONA 4", FiscalYear: 2026,
		RowCount: 12, Logos: []LogoSnapshot{{AssetID: "logo-1", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 35, MaxHeightMM: 14}},
		Signatories: closingSignatories{AgricultureOfficeName: "IR. HARYANTO", AgricultureOfficeNIP: "197203151998031004", InstallerName: "Pelaksana", SupervisorName: "Pengawas"},
	}
}
