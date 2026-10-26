package finalreport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/finalreport/template"
	"konkit/internal/programs"
)

type stubProgramContexts struct{ ctx programs.StorageContext }

func (s stubProgramContexts) ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error) {
	return s.ctx, nil
}

func TestPutSettingsRejectsNegativeCounts(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	_, err := svc.PutSettings(context.Background(), auth.Principal{}, Settings{ProgramID: "p", RegencyID: "r", LeaderCount: -1}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrCountNegative) {
		t.Fatalf("expected ErrCountNegative, got %v", err)
	}
}

func TestReplacePhotoPicksEnforcesPerSlideSlotLimit(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	limits := map[string]int{SlideKeyPengirimanPenerimaan: 3, SlideKeyRapatKoordinasi: 2, SlideKeySosialisasi: 4}
	for slideKey, limit := range limits {
		ids := make([]string, limit+1)
		for i := range ids {
			ids[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		}
		_, err := svc.ReplacePhotoPicks(context.Background(), "p", "r", slideKey, ids, auth.RegencyScope{Unrestricted: true})
		if !errors.Is(err, ErrTooManyPicks) {
			t.Fatalf("%s with %d picks: expected ErrTooManyPicks, got %v", slideKey, limit+1, err)
		}
	}
}

func TestReplacePhotoPicksRejectsDuplicateIDs(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	id := "00000000-0000-0000-0000-000000000001"
	_, err := svc.ReplacePhotoPicks(context.Background(), "p", "r", SlideKeySosialisasi, []string{id, id}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrDuplicatePick) {
		t.Fatalf("expected ErrDuplicatePick, got %v", err)
	}
}

func TestReadAndGenerateRequireProgramAndRegency(t *testing.T) {
	svc := NewServiceForTest(nil, stubProgramContexts{})
	ctx := context.Background()
	scope := auth.RegencyScope{Unrestricted: true}
	calls := map[string]func(programID, regencyID string) error{
		"Preview": func(p, r string) error { _, err := svc.Preview(ctx, p, r, scope); return err },
		"ListPhotoPicks": func(p, r string) error {
			_, err := svc.ListPhotoPicks(ctx, p, r, SlideKeySosialisasi, scope)
			return err
		},
		"ReplacePhotoPicks": func(p, r string) error {
			_, err := svc.ReplacePhotoPicks(ctx, p, r, SlideKeySosialisasi, nil, scope)
			return err
		},
		"ListDocuments": func(p, r string) error { _, err := svc.ListDocuments(ctx, p, r, scope); return err },
		"Generate": func(p, r string) error {
			_, err := svc.Generate(ctx, auth.Principal{}, p, r, scope, auth.ClientMeta{})
			return err
		},
	}
	for name, call := range calls {
		if err := call(" ", "r"); !errors.Is(err, ErrProgramRequired) {
			t.Errorf("%s without program: expected ErrProgramRequired, got %v", name, err)
		}
		if err := call("p", ""); !errors.Is(err, ErrRegencyRequired) {
			t.Errorf("%s without regency: expected ErrRegencyRequired, got %v", name, err)
		}
	}
}

func TestFormatIndonesianDate(t *testing.T) {
	cases := map[string]string{
		"2026-05-07": "7 Mei 2026", "2026-01-01": "1 Januari 2026", "2026-02-28": "28 Februari 2026",
		"2026-03-10": "10 Maret 2026", "2026-04-02": "2 April 2026", "2026-06-15": "15 Juni 2026",
		"2026-07-04": "4 Juli 2026", "2026-08-17": "17 Agustus 2026", "2026-09-09": "9 September 2026",
		"2026-10-15": "15 Oktober 2026", "2026-11-30": "30 November 2026", "2026-12-31": "31 Desember 2026",
		"": "", "  ": "", "garbage": "", "2026-13-01": "", "07-05-2026": "", "2026-05-07T10:00:00Z": "",
	}
	for input, want := range cases {
		if got := formatIndonesianDate(input); got != want {
			t.Errorf("formatIndonesianDate(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRegencyLabel(t *testing.T) {
	cases := map[string]string{
		"KAB. BIREUEN": "KABUPATEN BIREUEN", "Kab. Wajo": "KABUPATEN WAJO", "KOTA LANGSA": "KOTA LANGSA",
		"kota langsa": "KOTA LANGSA", "": "",
	}
	for input, want := range cases {
		if got := regencyLabel(input); got != want {
			t.Errorf("regencyLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDocumentFilename(t *testing.T) {
	cases := []struct {
		programType programs.ProgramType
		regency     string
		want        string
	}{
		{programs.ProgramFarmer, "KAB. BIREUEN", "LAPORAN AKHIR - PETANI KAB. BIREUEN.pptx"},
		{programs.ProgramFisherman, "Kota Langsa", "LAPORAN AKHIR - NELAYAN KOTA LANGSA.pptx"},
	}
	for _, tc := range cases {
		if got := documentFilename(tc.programType, tc.regency); got != tc.want {
			t.Errorf("documentFilename(%q, %q) = %q, want %q", tc.programType, tc.regency, got, tc.want)
		}
	}
}

func TestSlotJPEGTranscodesPNGAndPassesJPEGThrough(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	out, err := slotJPEG(pngBuf.Bytes())
	if err != nil {
		t.Fatalf("transcode png: %v", err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("transcoded png is not a JPEG: %v", err)
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	same, err := slotJPEG(jpegBuf.Bytes())
	if err != nil || !bytes.Equal(same, jpegBuf.Bytes()) {
		t.Fatalf("jpeg must pass through unchanged (err=%v)", err)
	}
	if _, err := slotJPEG([]byte("not an image")); err == nil {
		t.Fatal("expected error for non-image bytes")
	}
}

func TestPreviewFillsBlankWhenNoDataExists(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	svc := NewService(repo, stubProgramContexts{}, nil, nil)
	ctx := context.Background()

	var programID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	// A regency with no program_schedules row at all for this program:
	var regencyID string
	if err := pool.QueryRow(ctx, `
		SELECT reg.id::text FROM regencies reg
		WHERE NOT EXISTS (SELECT 1 FROM program_schedules ps WHERE ps.program_id=$1::uuid AND ps.regency_id=reg.id)
		LIMIT 1
	`, programID).Scan(&regencyID); err != nil {
		t.Skip("every regency already has a schedule for this program in the test DB")
	}

	preview, err := svc.Preview(ctx, programID, regencyID, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.RakordaDate != "Belum diisi" || preview.HandoverLocation != "Belum diisi" {
		t.Fatalf("expected blank placeholders, got %+v", preview)
	}
}

type fakeStorage struct {
	putCount int
	lastPut  []byte
	deleted  []string
	files    map[string][]byte
}

func (f *fakeStorage) Put(ctx context.Context, key string, folderPath []string, source io.Reader) (string, int64, string, error) {
	f.putCount++
	data, _ := io.ReadAll(source)
	f.lastPut = data
	if f.files == nil {
		f.files = map[string][]byte{}
	}
	f.files[key] = data
	return key, int64(len(data)), "fakechecksum", nil
}
func (f *fakeStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if data, ok := f.files[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, errors.New("not found")
}
func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func (f *fakeStorage) EnsureFolders(context.Context, [][]string) error { return nil }

func TestGenerateProducesSequentialVersions(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	storage := &fakeStorage{}
	svc := NewService(repo, stubProgramContexts{ctx: programs.StorageContext{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "Kabupaten Test", ProgramID: "p", RegencyID: "r"}}, storage, template.PPTX)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
	})

	doc1, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	doc2, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if doc1.Version != 1 || doc2.Version != 2 {
		t.Fatalf("got versions %d, %d; want 1, 2", doc1.Version, doc2.Version)
	}
	if storage.putCount != 2 {
		t.Fatalf("expected 2 Put calls, got %d", storage.putCount)
	}
	if doc1.StorageKey == doc2.StorageKey {
		t.Fatalf("storage keys must differ across generations, both %q", doc1.StorageKey)
	}
	// Superseded versions keep their file (spec §3.3): nothing is deleted on success.
	if len(storage.deleted) != 0 {
		t.Fatalf("expected no deletions on successful generate, got %v", storage.deleted)
	}
	if doc2.Filename != "LAPORAN AKHIR - PETANI KABUPATEN TEST.pptx" {
		t.Fatalf("unexpected filename %q", doc2.Filename)
	}
	var auditRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='final_report.generated' AND resource_type='final_report_document' AND resource_id=$1 AND metadata->>'version'='2' AND metadata->>'program_id'=$2 AND metadata->>'regency_id'=$3 AND metadata->>'checksum'='fakechecksum'`, doc2.ID, programID, regencyID).Scan(&auditRows); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("expected 1 audit row for generate, got %d", auditRows)
	}

	// The superseded first version stays downloadable.
	content, err := svc.OpenDocument(ctx, doc1.ID, auth.RegencyScope{RegencyIDs: []string{regencyID}})
	if err != nil {
		t.Fatalf("open superseded document: %v", err)
	}
	defer content.Reader.Close()
	data, _ := io.ReadAll(content.Reader)
	if content.Filename != doc1.Filename || !bytes.Equal(data, storage.files[doc1.StorageKey]) {
		t.Fatalf("opened %q (%d bytes), want %q", content.Filename, len(data), doc1.Filename)
	}
	if _, err := svc.OpenDocument(ctx, doc1.ID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-scope open: expected ErrNotFound, got %v", err)
	}
}

func TestGenerateTranscodesPNGPickIntoJPEGSlot(t *testing.T) {
	pool := testPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	var programID, regencyID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM programs LIMIT 1`).Scan(&programID); err != nil {
		t.Fatalf("fixture program: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM regencies ORDER BY id LIMIT 1`).Scan(&regencyID); err != nil {
		t.Fatalf("fixture regency: %v", err)
	}
	storageKey := "finalreport-test-" + t.Name()
	var mediaID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_media (program_id, regency_id, activity_type, storage_key, display_name, original_filename, media_type, mime_type, byte_size, checksum, source)
		VALUES ($1::uuid, $2::uuid, 'rakor', $3, 'TEST-RAKOR-PNG', 'test.png', 'image', 'image/png', 100, repeat('a',64), 'gallery')
		RETURNING id::text
	`, programID, regencyID, storageKey).Scan(&mediaID); err != nil {
		t.Fatalf("seed activity_media: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM final_report_documents WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
		pool.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE program_id=$1::uuid AND regency_id=$2::uuid`, programID, regencyID)
		pool.Exec(ctx, `DELETE FROM activity_media WHERE id=$1::uuid`, mediaID)
	})
	if _, err := repo.ReplacePhotoPicks(ctx, programID, regencyID, SlideKeyRapatKoordinasi, []string{mediaID}, unrestrictedScope()); err != nil {
		t.Fatalf("pick png: %v", err)
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	storage := &fakeStorage{files: map[string][]byte{storageKey: pngBuf.Bytes()}}
	svc := NewService(repo, stubProgramContexts{ctx: programs.StorageContext{ProgramType: "farmer", ZoneName: "Zona 1", RegencyName: "KAB. TEST"}}, storage, template.PPTX)
	if _, err := svc.Generate(ctx, auth.Principal{}, programID, regencyID, unrestrictedScope(), auth.ClientMeta{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	slot := zipEntries(t, storage.lastPut)["ppt/media/image8.jpg"]
	placeholder, _ := placeholderJPEG()
	if bytes.Equal(slot, placeholder) {
		t.Fatal("picked PNG slot was left as placeholder")
	}
	if bytes.Equal(slot, pngBuf.Bytes()) {
		t.Fatal("PNG bytes were placed in the .jpg slot without transcoding")
	}
	if _, err := jpeg.Decode(bytes.NewReader(slot)); err != nil {
		t.Fatalf("slot holding the PNG pick is not a JPEG: %v", err)
	}
}
