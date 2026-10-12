package bast

import (
	"context"
	"errors"
	"strings"
	"time"

	"konkit/internal/auth"
)

// WetPreviewer adalah layanan BA yang mendukung tanda tangan basah; Preview
// dengan konteks withWetPage hanya merender halaman tanda tangan.
type WetPreviewer interface {
	Preview(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error)
}

type wetRepository interface {
	WetSignatureEnabled(context.Context, string, string, auth.RegencyScope) (bool, error)
	GetWetScan(context.Context, string, string, string, auth.RegencyScope) (*WetScanMeta, error)
	OpenWetScan(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
	PutWetScan(context.Context, auth.Principal, string, string, string, string, []byte, auth.RegencyScope, auth.ClientMeta) error
	DeleteWetScan(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
}

// ErrWetScanInvalid: berkas pindaian bukan gambar yang dapat dibaca atau
// lebih dari 8 MB.
var ErrWetScanInvalid = errors.New("wet signature scan must be an image up to 8 MB")

// WetSignatureStatus adalah status tanda tangan basah satu BA pada satu tanggal.
type WetSignatureStatus struct {
	Supported bool         `json:"supported"`
	Enabled   bool         `json:"enabled"`
	Code      string       `json:"code,omitempty"`
	Scan      *WetScanView `json:"scan,omitempty"`
}

// WetScanView: Valid false berarti isi dokumen berubah setelah halaman
// dipindai, sehingga halaman perlu ditandatangani ulang.
type WetScanView struct {
	UploadedAt time.Time `json:"uploaded_at"`
	Valid      bool      `json:"valid"`
}

// WetSignatureService mengelola cetak halaman tanda tangan dan unggah hasil
// pindaian tanda tangan basah untuk BA yang mendukungnya.
type WetSignatureService struct {
	repository wetRepository
	documents  map[string]WetPreviewer
}

func NewWetSignatureService(repository wetRepository, documents map[string]WetPreviewer) *WetSignatureService {
	return &WetSignatureService{repository: repository, documents: documents}
}

func (s *WetSignatureService) document(documentKey string) (WetPreviewer, bool) {
	previewer, ok := s.documents[documentKey]
	return previewer, ok && previewer != nil && wetSignatureDocuments[documentKey]
}

func validWetInput(scheduleID, date string) bool {
	if strings.TrimSpace(scheduleID) == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	return err == nil
}

// Page merender halaman tanda tangan untuk dicetak dan mengembalikan kode
// dokumennya.
func (s *WetSignatureService) Page(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, string, error) {
	previewer, ok := s.document(documentKey)
	if !ok || !validWetInput(scheduleID, date) {
		return AggregatePreview{}, "", ErrInvalidInput
	}
	pageCtx, request := withWetPage(ctx)
	preview, err := previewer.Preview(pageCtx, strings.TrimSpace(scheduleID), strings.TrimSpace(date), scope)
	if err != nil {
		return AggregatePreview{}, "", err
	}
	return preview, request.code, nil
}

func (s *WetSignatureService) Status(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) (WetSignatureStatus, error) {
	if !wetSignatureDocuments[documentKey] {
		return WetSignatureStatus{}, nil
	}
	if !validWetInput(scheduleID, date) {
		return WetSignatureStatus{}, ErrInvalidInput
	}
	enabled, err := s.repository.WetSignatureEnabled(ctx, strings.TrimSpace(scheduleID), documentKey, scope)
	if err != nil {
		return WetSignatureStatus{}, err
	}
	status := WetSignatureStatus{Supported: true, Enabled: enabled}
	if !enabled {
		return status, nil
	}
	if _, code, err := s.Page(ctx, documentKey, scheduleID, date, scope); err == nil {
		status.Code = code
	}
	meta, err := s.repository.GetWetScan(ctx, strings.TrimSpace(scheduleID), documentKey, strings.TrimSpace(date), scope)
	if err != nil {
		return WetSignatureStatus{}, err
	}
	if meta != nil {
		status.Scan = &WetScanView{UploadedAt: meta.UploadedAt, Valid: status.Code != "" && meta.ContentCode == status.Code}
	}
	return status, nil
}

// Upload menyimpan pindaian (PNG, dinormalisasi 8-bit) dengan kode isi
// dokumen saat ini.
func (s *WetSignatureService) Upload(ctx context.Context, actor auth.Principal, documentKey, scheduleID, date string, image []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if len(image) == 0 || len(image) > 8<<20 {
		return ErrWetScanInvalid
	}
	normalized, err := normalizeSignatoryPNG(image)
	if err != nil {
		return ErrWetScanInvalid
	}
	_, code, err := s.Page(ctx, documentKey, scheduleID, date, scope)
	if err != nil {
		return err
	}
	return s.repository.PutWetScan(ctx, actor, strings.TrimSpace(scheduleID), documentKey, strings.TrimSpace(date), code, normalized, scope, meta)
}

func (s *WetSignatureService) Delete(ctx context.Context, actor auth.Principal, documentKey, scheduleID, date string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if !wetSignatureDocuments[documentKey] || !validWetInput(scheduleID, date) {
		return ErrInvalidInput
	}
	return s.repository.DeleteWetScan(ctx, actor, strings.TrimSpace(scheduleID), documentKey, strings.TrimSpace(date), scope, meta)
}

func (s *WetSignatureService) Open(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) ([]byte, error) {
	if !wetSignatureDocuments[documentKey] || !validWetInput(scheduleID, date) {
		return nil, ErrInvalidInput
	}
	return s.repository.OpenWetScan(ctx, strings.TrimSpace(scheduleID), documentKey, strings.TrimSpace(date), scope)
}
