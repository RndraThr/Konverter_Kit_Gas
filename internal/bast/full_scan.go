package bast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

// Pindai lengkap: seluruh BA dicetak, ditandatangani dan diparaf basah, lalu
// semua halamannya dipindai (atau PDF hasil pindai diunggah). Selama ada
// pindaian, PDF pindaian itulah yang menjadi preview dan PDF final; snapshot
// data BA tetap disimpan seperti biasa. Tanpa pindaian BA dibuat sistem
// (tanda tangan digital, atau halaman tanda tangan basah). Kode isi (sama
// dengan tanda tangan basah) mengikat pindaian ke isi BA: bila isi berubah,
// finalisasi menunggu pindaian ulang (atau pindaian dihapus).

// WetMode adalah mode tanda tangan satu BA dari panel Penandatangan:
// halaman tanda tangan basah (Wet) atau digital.
type WetMode struct {
	Wet bool
}

// fullScanDocuments adalah BA yang mendukung pindai lengkap.
var fullScanDocuments = map[string]bool{
	"dp3": true, "rekap-harian": true, "closing-titik-serah": true,
	"closing-kabupaten": true, "servis-berkala": true, "tkdn": true,
	"ba-perorangan": true, "gas-elpiji": true,
}

// MaxFullScanBytes membatasi PDF pindaian satu BA.
const MaxFullScanBytes = 30 << 20

var (
	// ErrFullScanRequired: pindaian lengkap BA ini dibuat untuk isi lama;
	// perlu dipindai ulang atau dihapus sebelum finalisasi.
	ErrFullScanRequired = errors.New("full document scan does not match the document")
	// ErrFullScanInvalid: berkas bukan PDF yang dapat dibaca atau terlalu besar.
	ErrFullScanInvalid = errors.New("full document scan must be a readable PDF up to 30 MB")
)

// FullScanPageError: jumlah halaman pindaian tidak sama dengan BA yang dicetak.
type FullScanPageError struct{ Want, Got int }

func (e *FullScanPageError) Error() string {
	return fmt.Sprintf("full document scan has %d pages, the document has %d", e.Got, e.Want)
}

// systemRenderKey: Preview merender BA buatan sistem walau pindaian lengkap
// sudah ada (untuk dicetak, dan menghitung halaman yang harus dipindai).
type systemRenderKey struct{}

func withSystemRender(ctx context.Context) context.Context {
	return context.WithValue(ctx, systemRenderKey{}, true)
}

func systemRender(ctx context.Context) bool {
	value, _ := ctx.Value(systemRenderKey{}).(bool)
	return value
}

// blankPrintKey: BA dicetak tanpa tanda tangan dan stempel digital, untuk
// ditandatangani di kertas lalu dipindai lengkap.
type blankPrintKey struct{}

func blankPrint(ctx context.Context) bool {
	value, _ := ctx.Value(blankPrintKey{}).(bool)
	return value
}

// fullScanPreview: preview BA menampilkan pindaian yang cocok bila ada.
func fullScanPreview(wet WetSignature, pdf []byte, pages int) ([]byte, int) {
	if !wet.Full || len(wet.FullPDF) == 0 {
		return pdf, pages
	}
	if count, err := pdfPageCount(wet.FullPDF); err == nil {
		pages = count
	}
	return wet.FullPDF, pages
}

type fullScanLoader interface {
	ResolveFullScan(ctx context.Context, scheduleID, documentKey, date string) (exists bool, pdf []byte, code string, err error)
}

// loadFullScan: ok bila mode pindai lengkap berlaku untuk BA ini.
func loadFullScan(ctx context.Context, repository any, scheduleID, documentKey, date, code string) (WetSignature, bool, error) {
	if !fullScanDocuments[wetBaseKey(documentKey)] {
		return WetSignature{}, false, nil
	}
	loader, ok := repository.(fullScanLoader)
	if !ok {
		return WetSignature{}, false, nil
	}
	exists, pdf, scanCode, err := loader.ResolveFullScan(ctx, scheduleID, documentKey, date)
	if err != nil || !exists {
		return WetSignature{}, false, err
	}
	wet := WetSignature{Full: true, Code: code}
	if len(pdf) > 0 && scanCode == code && !systemRender(ctx) {
		wet.FullPDF = pdf
	}
	return wet, true, nil
}

// fullScanFinalPDF: PDF yang disimpan saat finalisasi; pindaian bila mode
// pindai lengkap aktif (wajib ada), selain itu PDF buatan sistem.
func fullScanFinalPDF(wet WetSignature, rendered []byte) ([]byte, error) {
	if !wet.Full {
		return rendered, nil
	}
	if len(wet.FullPDF) == 0 {
		return nil, ErrFullScanRequired
	}
	return wet.FullPDF, nil
}

// pdfPageCount membaca jumlah halaman PDF; galat berarti bukan PDF yang sah.
func pdfPageCount(data []byte) (int, error) {
	count, err := pdfapi.PageCount(bytes.NewReader(data), nil)
	if err != nil || count < 1 {
		return 0, ErrFullScanInvalid
	}
	return count, nil
}

// ── Repository ──────────────────────────────────────────────────────────

// FullScanMeta menjelaskan pindaian lengkap yang tersimpan.
type FullScanMeta struct {
	ContentCode string
	PageCount   int
	ByteSize    int
	UploadedAt  time.Time
}

// ResolveFullScan mengembalikan pindaian lengkap BA ini (exists false bila
// belum ada).
func (r *Repository) ResolveFullScan(ctx context.Context, scheduleID, documentKey, date string) (bool, []byte, string, error) {
	var pdf []byte
	var code string
	err := r.pool.QueryRow(ctx, `SELECT pdf, content_code FROM bast_full_scans WHERE schedule_id::text=$1 AND document_key=$2 AND document_date=$3::date`, scheduleID, documentKey, date).Scan(&pdf, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, "", nil
	}
	if err != nil {
		return false, nil, "", fmt.Errorf("resolve full scan: %w", err)
	}
	return true, pdf, code, nil
}

func (r *Repository) GetFullScan(ctx context.Context, scheduleID, documentKey, date string, scope auth.RegencyScope) (*FullScanMeta, error) {
	var meta FullScanMeta
	err := r.pool.QueryRow(ctx, `
		SELECT s.content_code, s.page_count, s.byte_size, s.uploaded_at FROM bast_full_scans s
		JOIN program_schedules ps ON ps.id = s.schedule_id
		WHERE s.schedule_id::text=$1 AND s.document_key=$2 AND s.document_date=$3::date AND ($4 OR ps.regency_id::text = ANY($5))
	`, scheduleID, documentKey, date, scope.Unrestricted, scope.RegencyIDs).Scan(&meta.ContentCode, &meta.PageCount, &meta.ByteSize, &meta.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get full scan: %w", err)
	}
	return &meta, nil
}

func (r *Repository) OpenFullScan(ctx context.Context, scheduleID, documentKey, date string, scope auth.RegencyScope) ([]byte, error) {
	var pdf []byte
	err := r.pool.QueryRow(ctx, `
		SELECT s.pdf FROM bast_full_scans s
		JOIN program_schedules ps ON ps.id = s.schedule_id
		WHERE s.schedule_id::text=$1 AND s.document_key=$2 AND s.document_date=$3::date AND ($4 OR ps.regency_id::text = ANY($5))
	`, scheduleID, documentKey, date, scope.Unrestricted, scope.RegencyIDs).Scan(&pdf)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open full scan: %w", err)
	}
	return pdf, nil
}

func (r *Repository) PutFullScan(ctx context.Context, actor auth.Principal, scheduleID, documentKey, date, code string, pageCount int, pdf []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin full scan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bast_full_scans(schedule_id, document_key, document_date, content_code, page_count, pdf, byte_size, uploaded_by)
		VALUES ($1::uuid, $2, $3::date, $4, $5, $6, $7, NULLIF($8,'')::uuid)
		ON CONFLICT (schedule_id, document_key, document_date) DO UPDATE SET content_code=EXCLUDED.content_code, page_count=EXCLUDED.page_count, pdf=EXCLUDED.pdf, byte_size=EXCLUDED.byte_size, uploaded_by=EXCLUDED.uploaded_by, uploaded_at=now()
	`, scheduleID, documentKey, date, code, pageCount, pdf, len(pdf), actor.UserID); err != nil {
		return fmt.Errorf("save full scan: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.full_scan_uploaded", ResourceType: "bast_full_scan", ResourceID: scheduleID, Metadata: map[string]any{"document_key": documentKey, "document_date": date, "content_code": code, "page_count": pageCount}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeleteFullScan(ctx context.Context, actor auth.Principal, scheduleID, documentKey, date string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete full scan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_full_scans WHERE schedule_id::text=$1 AND document_key=$2 AND document_date=$3::date`, scheduleID, documentKey, date); err != nil {
		return fmt.Errorf("delete full scan: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.full_scan_deleted", ResourceType: "bast_full_scan", ResourceID: scheduleID, Metadata: map[string]any{"document_key": documentKey, "document_date": date}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── Service ─────────────────────────────────────────────────────────────

type fullScanRepository interface {
	GetFullScan(context.Context, string, string, string, auth.RegencyScope) (*FullScanMeta, error)
	OpenFullScan(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
	PutFullScan(context.Context, auth.Principal, string, string, string, string, int, []byte, auth.RegencyScope, auth.ClientMeta) error
	DeleteFullScan(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
}

// FullScanDocument adalah satu BA yang mendukung pindai lengkap: Preview
// merender BA lengkap (yang dicetak), Code mengembalikan kode isinya.
type FullScanDocument struct {
	Preview func(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error)
	Code    func(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (string, error)
}

// WetPageCode adalah Code untuk BA yang mendukung tanda tangan basah: kode
// isi dari halaman tanda tangannya.
func WetPageCode(previewer WetPreviewer) func(context.Context, string, string, auth.RegencyScope) (string, error) {
	return func(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (string, error) {
		pageCtx, request := withWetPage(ctx)
		if _, err := previewer.Preview(pageCtx, scheduleID, date, scope); err != nil {
			return "", err
		}
		return request.code, nil
	}
}

// FullScanStatus adalah status pindai lengkap satu BA pada satu tanggal.
type FullScanStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Code      string `json:"code,omitempty"`
	// PageCount: jumlah halaman BA yang dicetak (pindaian harus sama).
	PageCount int           `json:"page_count,omitempty"`
	Scan      *FullScanView `json:"scan,omitempty"`
}

// FullScanView: Valid false berarti isi BA berubah setelah dipindai.
type FullScanView struct {
	UploadedAt time.Time `json:"uploaded_at"`
	PageCount  int       `json:"page_count"`
	Valid      bool      `json:"valid"`
}

type FullScanService struct {
	repository fullScanRepository
	documents  map[string]FullScanDocument
}

func NewFullScanService(repository fullScanRepository, documents map[string]FullScanDocument) *FullScanService {
	return &FullScanService{repository: repository, documents: documents}
}

func (s *FullScanService) document(documentKey string) (FullScanDocument, bool) {
	document, ok := s.documents[documentKey]
	return document, ok && fullScanDocuments[documentKey] && document.Preview != nil && document.Code != nil
}

// Print merender BA lengkap untuk dicetak dan ditandatangani.
func (s *FullScanService) Print(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error) {
	document, ok := s.document(documentKey)
	if !ok || !validWetInput(scheduleID, date) {
		return AggregatePreview{}, ErrInvalidInput
	}
	printCtx := context.WithValue(withSystemRender(ctx), blankPrintKey{}, true)
	return document.Preview(printCtx, strings.TrimSpace(scheduleID), strings.TrimSpace(date), scope)
}

func (s *FullScanService) Status(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) (FullScanStatus, error) {
	document, ok := s.document(documentKey)
	if !ok {
		return FullScanStatus{}, nil
	}
	if !validWetInput(scheduleID, date) {
		return FullScanStatus{}, ErrInvalidInput
	}
	scheduleID, date = strings.TrimSpace(scheduleID), strings.TrimSpace(date)
	status := FullScanStatus{Supported: true}
	if code, err := document.Code(ctx, scheduleID, date, scope); err == nil {
		status.Code = code
	}
	if preview, err := document.Preview(withSystemRender(ctx), scheduleID, date, scope); err == nil {
		status.PageCount = preview.PageCount
	}
	key, err := s.scanKey(ctx, documentKey, scheduleID, scope)
	if err != nil {
		return FullScanStatus{}, err
	}
	meta, err := s.repository.GetFullScan(ctx, scheduleID, key, date, scope)
	if err != nil {
		return FullScanStatus{}, err
	}
	if meta != nil {
		status.Scan = &FullScanView{UploadedAt: meta.UploadedAt, PageCount: meta.PageCount, Valid: status.Code != "" && meta.ContentCode == status.Code}
	}
	return status, nil
}

// Upload menyimpan PDF pindaian bila jumlah halamannya sama dengan BA yang
// dicetak, dengan kode isi BA saat ini.
func (s *FullScanService) Upload(ctx context.Context, actor auth.Principal, documentKey, scheduleID, date string, pdf []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	document, ok := s.document(documentKey)
	if !ok || !validWetInput(scheduleID, date) {
		return ErrInvalidInput
	}
	if len(pdf) == 0 || len(pdf) > MaxFullScanBytes {
		return ErrFullScanInvalid
	}
	got, err := pdfPageCount(pdf)
	if err != nil {
		return err
	}
	scheduleID, date = strings.TrimSpace(scheduleID), strings.TrimSpace(date)
	preview, err := document.Preview(withSystemRender(ctx), scheduleID, date, scope)
	if err != nil {
		return err
	}
	if preview.PageCount > 0 && got != preview.PageCount {
		return &FullScanPageError{Want: preview.PageCount, Got: got}
	}
	code, err := document.Code(ctx, scheduleID, date, scope)
	if err != nil {
		return err
	}
	key, err := s.scanKey(ctx, documentKey, scheduleID, scope)
	if err != nil {
		return err
	}
	return s.repository.PutFullScan(ctx, actor, scheduleID, key, date, code, got, pdf, scope, meta)
}

func (s *FullScanService) Delete(ctx context.Context, actor auth.Principal, documentKey, scheduleID, date string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if _, ok := s.document(documentKey); !ok || !validWetInput(scheduleID, date) {
		return ErrInvalidInput
	}
	key, err := s.scanKey(ctx, documentKey, scheduleID, scope)
	if err != nil {
		return err
	}
	return s.repository.DeleteFullScan(ctx, actor, strings.TrimSpace(scheduleID), key, strings.TrimSpace(date), scope, meta)
}

func (s *FullScanService) Open(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) ([]byte, error) {
	if _, ok := s.document(documentKey); !ok || !validWetInput(scheduleID, date) {
		return nil, ErrInvalidInput
	}
	key, err := s.scanKey(ctx, documentKey, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	return s.repository.OpenFullScan(ctx, strings.TrimSpace(scheduleID), key, strings.TrimSpace(date), scope)
}

// scanKey: BA Serah Terima menyimpan satu pindaian per titik serah, sama
// seperti tanda tangan basah.
func (s *FullScanService) scanKey(ctx context.Context, documentKey, scheduleID string, scope auth.RegencyScope) (string, error) {
	if documentKey != "closing-titik-serah" {
		return documentKey, nil
	}
	lister, ok := s.repository.(handoverLocationLister)
	if !ok {
		return documentKey, nil
	}
	locations, err := lister.ListHandoverLocations(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return "", err
	}
	location, err := pickHandoverLocation(locations, handoverLocationFrom(ctx))
	if err != nil {
		return "", err
	}
	return closingWetKey(location.ID), nil
}
