package bast

import (
	"context"
	"strings"
	"testing"
)

type wetLoaderStub struct {
	enabled  bool
	scan     []byte
	scanCode string
}

func (s wetLoaderStub) ResolveWetSignature(context.Context, string, string, string) (bool, []byte, string, error) {
	return s.enabled, s.scan, s.scanCode, nil
}

// Kode dokumen tidak berubah oleh tanda tangan digital, status tanda tangan
// basah, maupun nomor versi; berubah bila isi dokumen berubah.
func TestContentCodeIgnoresSignatureAndVersion(t *testing.T) {
	base := sampleClosingSnapshot()
	code := contentCode("closing-titik-serah", base.DocumentDate, base)
	if len(code) != 8 || strings.ToUpper(code) != code {
		t.Fatalf("code=%q", code)
	}
	same := sampleClosingSnapshot()
	same.DocumentNumber = "BA-OTHER/7"
	same.SignatureMarks = SignatureMarks{SignatoryInstaller: {Signature: true}}
	same.Wet = WetSignature{Enabled: true, Code: "X", Scanned: true}
	if got := contentCode("closing-titik-serah", same.DocumentDate, same); got != code {
		t.Fatalf("code changed by non-content fields: %s vs %s", got, code)
	}
	changed := sampleClosingSnapshot()
	changed.Rows[0].Count++
	if got := contentCode("closing-titik-serah", changed.DocumentDate, changed); got == code {
		t.Fatal("code must change when content changes")
	}
}

func TestLoadWetSignatureMatchesScanByCode(t *testing.T) {
	snapshot := sampleClosingSnapshot()
	code := contentCode("closing-titik-serah", snapshot.DocumentDate, snapshot)
	ctx := context.Background()

	wet, images, err := loadWetSignature(ctx, wetLoaderStub{enabled: true, scan: testPNG(t), scanCode: code}, "s", "closing-titik-serah", snapshot.DocumentDate, snapshot)
	if err != nil || !wet.Enabled || !wet.Scanned || images[wetScanImageKey] == nil {
		t.Fatalf("matching scan wet=%+v err=%v", wet, err)
	}
	wet, images, _ = loadWetSignature(ctx, wetLoaderStub{enabled: true, scan: testPNG(t), scanCode: "STALE000"}, "s", "closing-titik-serah", snapshot.DocumentDate, snapshot)
	if !wet.Enabled || wet.Scanned || images != nil {
		t.Fatalf("stale scan must not be used: wet=%+v", wet)
	}
	wet, _, _ = loadWetSignature(ctx, wetLoaderStub{}, "s", "closing-titik-serah", snapshot.DocumentDate, snapshot)
	if wet.Enabled {
		t.Fatal("disabled mode must stay off")
	}
	wet, _, _ = loadWetSignature(ctx, wetLoaderStub{enabled: true}, "s", "pemeriksaan", snapshot.DocumentDate, snapshot)
	if wet.Enabled {
		t.Fatal("unsupported document must stay off")
	}
	pageCtx, request := withWetPage(ctx)
	wet, _, _ = loadWetSignature(pageCtx, nil, "s", "closing-titik-serah", snapshot.DocumentDate, snapshot)
	if !wet.PageOnly || request.code != code {
		t.Fatalf("page request wet=%+v code=%q", wet, request.code)
	}
}

// Cetak halaman tanda tangan hanya menghasilkan halaman terakhir dengan kode
// dokumen; pindaian menggantikan halaman tanda tangan buatan sistem.
func TestRenderClosingWetSignaturePages(t *testing.T) {
	images := map[string][]byte{"logo-1": testPNG(t)}
	full, err := RenderClosingTitikSerah(sampleClosingSnapshot(), images)
	if err != nil {
		t.Fatal(err)
	}

	pageOnly := sampleClosingSnapshot()
	pageOnly.Wet = WetSignature{Enabled: true, Code: "ABCD1234", PageOnly: true}
	page, err := RenderClosingTitikSerah(pageOnly, images)
	if err != nil {
		t.Fatal(err)
	}
	if page.PageCount != 1 {
		t.Fatalf("signature page pages=%d, want 1", page.PageCount)
	}
	if !strings.Contains(decodedPDFStreams(t, page.PDF), "ABCD1234") {
		t.Fatal("printed signature page must carry the document code")
	}

	scanned := sampleClosingSnapshot()
	scanned.Wet = WetSignature{Enabled: true, Code: "ABCD1234", Scanned: true}
	withScan := map[string][]byte{"logo-1": testPNG(t), wetScanImageKey: testPNG(t)}
	result, err := RenderClosingTitikSerah(scanned, withScan)
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount != full.PageCount {
		t.Fatalf("scanned pages=%d, want %d", result.PageCount, full.PageCount)
	}
	if strings.Contains(decodedPDFStreams(t, result.PDF), "ABCD1234") {
		t.Fatal("scanned document must not print the code footer")
	}
}
