package bast

import (
	"errors"
	"os"
	"testing"
)

func sampleGasElpijiSnapshot(date string, count int) GasElpijiSnapshot {
	return GasElpijiSnapshot{
		DocumentType: AggregateDocumentGasElpiji, DocumentDate: date, FiscalYear: 2026, RegencyName: "KAB. BANGKA",
		WarehouseLocation: "SPBE PT Bangka Gas Mandiri", WarehouseAddress: "Jl. Raya Sungailiat KM 12, Kec. Pemali, Kabupaten Bangka, Kepulauan Bangka Belitung",
		PIC: "Budi Santoso (0812 3456 7890)", CylinderCount: count, Remarks: "Diterima dalam kondisi baik",
		ReceiverName: "DWI SANTOSO", ReceiverCompany: "PT KIAN SANTANG MULIATAMA TBK.", PertaminaName: "RINA KARTIKA",
		Logos: []LogoSnapshot{
			{AssetID: "l1", StorageKey: "builtin:04-pertamina-transparan.png", MimeType: "image/png", SortOrder: 1, MaxWidthMM: 42, MaxHeightMM: 15},
			{AssetID: "l2", StorageKey: "builtin:01-logo-kuning-transparan.png", MimeType: "image/png", SortOrder: 2, MaxWidthMM: 42, MaxHeightMM: 15},
			{AssetID: "l3", StorageKey: "builtin:02-pertamina-patra-niaga-transparan.png", MimeType: "image/png", SortOrder: 3, MaxWidthMM: 42, MaxHeightMM: 15},
			{AssetID: "l4", StorageKey: "builtin:03-pt-kian-santang-muliatama-transparan.png", MimeType: "image/png", SortOrder: 4, MaxWidthMM: 42, MaxHeightMM: 15},
		},
	}
}

func TestGasElpijiInputNormalizeAndComplete(t *testing.T) {
	input := GasElpijiInput{ScheduleID: " s1 ", DocumentDate: "2026-12-02", WarehouseLocation: "  SPBE   Bangka ", WarehouseAddress: "Jl. Raya", PIC: "Budi", CylinderCount: 120, ReceiverName: "Dwi", PertaminaName: "Rina"}
	if err := input.normalize(); err != nil || input.ScheduleID != "s1" || input.WarehouseLocation != "SPBE Bangka" {
		t.Fatalf("input=%+v err=%v", input, err)
	}
	if !input.complete() {
		t.Fatal("expected complete input")
	}
	input.ReceiverName, input.PertaminaName = "", ""
	if !input.complete() {
		t.Fatal("blank signer names fall back to the schedule signatories")
	}
	snapshot := buildGasElpijiSnapshot(DP3Context{}, ScheduleSettings{InstallerName: "ANDI", PertaminaRepName: "  RINA  "}, nil, input)
	if snapshot.ReceiverName != "ANDI" || snapshot.PertaminaName != "RINA" || !snapshot.signersComplete() {
		t.Fatalf("fallback names=%q/%q", snapshot.ReceiverName, snapshot.PertaminaName)
	}
	if buildGasElpijiSnapshot(DP3Context{}, ScheduleSettings{}, nil, input).signersComplete() {
		t.Fatal("no names anywhere must be incomplete")
	}
	input.CylinderCount = 0
	if input.complete() {
		t.Fatal("zero cylinders must be incomplete")
	}
	for _, bad := range []GasElpijiInput{
		{ScheduleID: "s1", DocumentDate: "02-12-2026"},
		{ScheduleID: "s1", DocumentDate: "2026-12-02", CylinderCount: -1},
		{DocumentDate: "2026-12-02"},
	} {
		if err := bad.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("input=%+v err=%v", bad, err)
		}
	}
}

func TestGasElpijiStatementAndThousands(t *testing.T) {
	if got := gasElpijiStatement("2026-12-02"); got != "Pada hari ini, Rabu, tanggal 2 Desember tahun 2026, telah dilakukan pengantaran tabung gas elpiji dan diterima, dengan rincian sebagai berikut:" {
		t.Fatalf("statement=%q", got)
	}
	for value, want := range map[int]string{0: "0", 999: "999", 1250: "1.250", 1000000: "1.000.000"} {
		if got := formatThousands(value); got != want {
			t.Fatalf("formatThousands(%d)=%q", value, got)
		}
	}
}

func TestRenderGasElpijiOnePagePerReceipt(t *testing.T) {
	logoBytes, err := loadLogoBytes(t.Context(), nil, sampleGasElpijiSnapshot("2026-12-02", 1).Logos)
	if err != nil {
		t.Fatal(err)
	}
	single, err := RenderGasElpiji([]GasElpijiSnapshot{sampleGasElpijiSnapshot("2026-12-02", 1250)}, logoBytes)
	if err != nil || single.PageCount != 1 || string(single.PDF[:4]) != "%PDF" {
		t.Fatalf("single page_count=%d err=%v", single.PageCount, err)
	}
	blank := GasElpijiSnapshot{DocumentType: AggregateDocumentGasElpiji, DocumentDate: "2026-12-12", FiscalYear: 2026, Logos: sampleGasElpijiSnapshot("", 0).Logos}
	merged, err := RenderGasElpiji([]GasElpijiSnapshot{sampleGasElpijiSnapshot("2026-12-02", 1250), blank}, logoBytes)
	if err != nil || merged.PageCount != 2 {
		t.Fatalf("merged page_count=%d err=%v", merged.PageCount, err)
	}
	if path := os.Getenv("GAS_ELPIJI_SAMPLE_PDF"); path != "" {
		if err := os.WriteFile(path, merged.PDF, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RenderGasElpiji(nil, logoBytes); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty err=%v", err)
	}
}

func TestMergedMatchesTracksReceiptChecksums(t *testing.T) {
	receipts := []GasElpijiReceipt{{Document: AggregateDocument{ID: "a", Checksum: "1"}}, {Document: AggregateDocument{ID: "b", Checksum: "2"}}}
	snapshot := GasElpijiMergedSnapshot{Receipts: []GasElpijiMergedReceipt{{DocumentID: "a", Checksum: "1"}, {DocumentID: "b", Checksum: "2"}}}
	if !mergedMatches(snapshot, receipts) {
		t.Fatal("expected merged to match")
	}
	receipts[1].Document.Checksum = "3"
	if mergedMatches(snapshot, receipts) {
		t.Fatal("changed receipt must make merged outdated")
	}
	if mergedMatches(snapshot, receipts[:1]) {
		t.Fatal("cancelled receipt must make merged outdated")
	}
}

func TestRenderGasElpijiUsesSignaturesAndWetScans(t *testing.T) {
	logoBytes, err := loadLogoBytes(t.Context(), nil, sampleGasElpijiSnapshot("2026-12-02", 1).Logos)
	if err != nil {
		t.Fatal(err)
	}
	logoBytes[signatoryImageKey(SignatoryInstaller, SignatoryKindSignature)] = testPNG(t)
	logoBytes[signatoryImageKey(SignatoryPertamina, SignatoryKindStamp)] = testPNG(t)
	digital := sampleGasElpijiSnapshot("2026-12-02", 120)
	digital.SignatureMarks = SignatureMarks{SignatoryInstaller: {Signature: true}, SignatoryPertamina: {Stamp: true}}
	plain, err := RenderGasElpiji([]GasElpijiSnapshot{sampleGasElpijiSnapshot("2026-12-02", 120)}, logoBytes)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := RenderGasElpiji([]GasElpijiSnapshot{digital}, logoBytes)
	if err != nil || signed.PageCount != 1 || len(signed.PDF) <= len(plain.PDF) {
		t.Fatalf("signed page_count=%d size=%d plain=%d err=%v", signed.PageCount, len(signed.PDF), len(plain.PDF), err)
	}

	// Penerimaan bertanda tangan basah memakai pindaiannya sebagai halaman,
	// sedangkan penerimaan lain di BA gabungan tetap dirender sistem.
	scanned := sampleGasElpijiSnapshot("2026-12-12", 80)
	scanned.Wet = WetSignature{Enabled: true, Code: "ABCD1234", Scanned: true}
	logoBytes[gasElpijiScanKey("2026-12-12")] = testPNG(t)
	merged, err := RenderGasElpiji([]GasElpijiSnapshot{digital, scanned}, logoBytes)
	if err != nil || merged.PageCount != 2 {
		t.Fatalf("merged page_count=%d err=%v", merged.PageCount, err)
	}
	if gasElpijiScanKey("2026-12-12") == gasElpijiScanKey("2026-12-02") {
		t.Fatal("scan keys must differ per date")
	}
}

func TestGasElpijiSignatureParties(t *testing.T) {
	roles := signatureDocuments[gasElpijiDocumentKey]
	if len(roles) != 2 || roles[0] != SignatoryInstaller || roles[1] != SignatoryPertamina || !wetSignatureDocuments[gasElpijiDocumentKey] {
		t.Fatalf("roles=%v wet=%v", roles, wetSignatureDocuments[gasElpijiDocumentKey])
	}
}
