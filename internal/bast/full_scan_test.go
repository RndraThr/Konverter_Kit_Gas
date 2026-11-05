package bast

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"

	"github.com/go-pdf/fpdf"
)

func pdfWithPages(t *testing.T, pages int) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	for i := 0; i < pages; i++ {
		doc.AddPage()
	}
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

type fullScanRepositoryStub struct {
	saved []byte
	code  string
	pages int
}

func (s *fullScanRepositoryStub) GetFullScan(context.Context, string, string, string, auth.RegencyScope) (*FullScanMeta, error) {
	if s.saved == nil {
		return nil, nil
	}
	return &FullScanMeta{ContentCode: s.code, PageCount: s.pages}, nil
}
func (s *fullScanRepositoryStub) OpenFullScan(context.Context, string, string, string, auth.RegencyScope) ([]byte, error) {
	return s.saved, nil
}
func (s *fullScanRepositoryStub) PutFullScan(_ context.Context, _ auth.Principal, _, _, _, code string, pages int, pdf []byte, _ auth.RegencyScope, _ auth.ClientMeta) error {
	s.saved, s.code, s.pages = pdf, code, pages
	return nil
}
func (s *fullScanRepositoryStub) DeleteFullScan(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error {
	s.saved = nil
	return nil
}

func TestFullScanUploadNeedsTheSamePageCount(t *testing.T) {
	code := "AAAA1111"
	repo := &fullScanRepositoryStub{}
	service := NewFullScanService(repo, map[string]FullScanDocument{"dp3": {
		Preview: func(context.Context, string, string, auth.RegencyScope) (AggregatePreview, error) {
			return AggregatePreview{PageCount: 3}, nil
		},
		Code: func(context.Context, string, string, auth.RegencyScope) (string, error) { return code, nil },
	}})
	ctx, scope := context.Background(), auth.RegencyScope{Unrestricted: true}

	var pages *FullScanPageError
	if err := service.Upload(ctx, auth.Principal{}, "dp3", "schedule-1", "2026-11-05", pdfWithPages(t, 2), scope, auth.ClientMeta{}); !errors.As(err, &pages) || pages.Want != 3 || pages.Got != 2 {
		t.Fatalf("err=%v", err)
	}
	if err := service.Upload(ctx, auth.Principal{}, "dp3", "schedule-1", "2026-11-05", []byte("not a pdf"), scope, auth.ClientMeta{}); !errors.Is(err, ErrFullScanInvalid) {
		t.Fatalf("err=%v", err)
	}
	if err := service.Upload(ctx, auth.Principal{}, "dp3", "schedule-1", "2026-11-05", pdfWithPages(t, 3), scope, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(ctx, "dp3", "schedule-1", "2026-11-05", scope)
	if err != nil || !status.Supported || status.PageCount != 3 || status.Scan == nil || !status.Scan.Valid || status.Scan.PageCount != 3 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	// The document changed after the scan: it has to be scanned again.
	code = "BBBB2222"
	if status, _ := service.Status(ctx, "dp3", "schedule-1", "2026-11-05", scope); status.Scan == nil || status.Scan.Valid {
		t.Fatalf("status after change=%+v", status.Scan)
	}
	// A BA without full scan support is refused.
	if err := service.Upload(ctx, auth.Principal{}, "gas-elpiji", "schedule-1", "2026-11-05", pdfWithPages(t, 3), scope, auth.ClientMeta{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("gas-elpiji err=%v", err)
	}
}

func TestFullScanFinalPDFUsesTheMatchingScan(t *testing.T) {
	rendered := []byte("%PDF-rendered")
	if got, err := fullScanFinalPDF(WetSignature{}, rendered); err != nil || !bytes.Equal(got, rendered) {
		t.Fatalf("digital: %q %v", got, err)
	}
	if _, err := fullScanFinalPDF(WetSignature{Full: true, Code: "C"}, rendered); !errors.Is(err, ErrFullScanRequired) {
		t.Fatalf("missing scan: %v", err)
	}
	scan := []byte("%PDF-scan")
	if got, err := fullScanFinalPDF(WetSignature{Full: true, Code: "C", FullPDF: scan}, rendered); err != nil || !bytes.Equal(got, scan) {
		t.Fatalf("scan: %q %v", got, err)
	}
}

type fullScanLoaderStub struct {
	exists bool
	pdf    []byte
	code   string
}

func (s fullScanLoaderStub) ResolveFullScan(context.Context, string, string, string) (bool, []byte, string, error) {
	return s.exists, s.pdf, s.code, nil
}

func TestLoadWetSignatureReportsFullScan(t *testing.T) {
	snapshot := map[string]string{"recipients": "x"}
	code := contentCode("dp3", "2026-11-05", snapshot)
	wet, _, err := loadWetSignature(context.Background(), fullScanLoaderStub{exists: true, pdf: []byte("%PDF"), code: code}, "s", "dp3", "2026-11-05", snapshot)
	if err != nil || !wet.Full || wet.Enabled || string(wet.FullPDF) != "%PDF" {
		t.Fatalf("wet=%+v err=%v", wet, err)
	}
	// A scan of other content is not used.
	wet, _, _ = loadWetSignature(context.Background(), fullScanLoaderStub{exists: true, pdf: []byte("%PDF"), code: "OTHER"}, "s", "dp3", "2026-11-05", snapshot)
	if !wet.Full || wet.FullPDF != nil {
		t.Fatalf("stale scan used: %+v", wet)
	}
	// No scan: falls through to the wet signature (none here).
	if wet, _, _ := loadWetSignature(context.Background(), fullScanLoaderStub{}, "s", "dp3", "2026-11-05", snapshot); wet.Full {
		t.Fatalf("wet=%+v", wet)
	}
}

func TestFullScanPreviewShowsTheMatchingScan(t *testing.T) {
	scan := pdfWithPages(t, 2)
	if pdf, pages := fullScanPreview(WetSignature{Full: true, FullPDF: scan}, []byte("%PDF-system"), 3); !bytes.Equal(pdf, scan) || pages != 2 {
		t.Fatalf("pages=%d", pages)
	}
	if pdf, pages := fullScanPreview(WetSignature{Full: true}, []byte("%PDF-system"), 3); string(pdf) != "%PDF-system" || pages != 3 {
		t.Fatalf("without scan pages=%d", pages)
	}
	// Printing renders the system BA even with a scan.
	snapshot := map[string]string{"recipients": "x"}
	code := contentCode("dp3", "2026-11-05", snapshot)
	wet, _, _ := loadWetSignature(withSystemRender(context.Background()), fullScanLoaderStub{exists: true, pdf: scan, code: code}, "s", "dp3", "2026-11-05", snapshot)
	if !wet.Full || wet.FullPDF != nil {
		t.Fatalf("system render used the scan: %+v", wet.Full)
	}
}

func TestMergePDFsJoinsThePagesInOrder(t *testing.T) {
	merged, err := mergePDFs([][]byte{pdfWithPages(t, 1), pdfWithPages(t, 2), pdfWithPages(t, 1)})
	if err != nil || merged.PageCount != 4 {
		t.Fatalf("pages=%d err=%v", merged.PageCount, err)
	}
	if count, err := pdfPageCount(merged.PDF); err != nil || count != 4 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	single := pdfWithPages(t, 2)
	if one, err := mergePDFs([][]byte{single}); err != nil || one.PageCount != 2 || !bytes.Equal(one.PDF, single) {
		t.Fatalf("single pages=%d err=%v", one.PageCount, err)
	}
}

func TestGasElpijiSupportsFullScan(t *testing.T) {
	if !fullScanDocuments["gas-elpiji"] || overviewFullScanKeys["gas-elpiji"] != "gas-elpiji" {
		t.Fatal("gas elpiji should support pindai lengkap")
	}
}
