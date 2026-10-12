package bast

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/textnorm"
)

// ErrSignatoryImageInvalid: berkas bukan PNG atau terlalu besar.
var ErrSignatoryImageInvalid = errors.New("signatory image must be a PNG up to 2 MB")

const maxSignatoryImageBytes = 2 << 20

type signatoryRepository interface {
	GetSignatureSettings(context.Context, string, string, auth.RegencyScope) (SignatureSettingsView, error)
	SaveSignatureRules(context.Context, auth.Principal, string, string, bool, SignatureRules, bool, auth.RegencyScope, auth.ClientMeta) error
	SaveSignatoryName(context.Context, auth.Principal, string, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	PutSignatoryAsset(context.Context, auth.Principal, string, string, string, []byte, auth.RegencyScope, auth.ClientMeta) error
	DeleteSignatoryAsset(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	OpenSignatoryAsset(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
}

// SignatoryService mengelola kartu penandatangan BA (nama, NIP, tanda tangan
// digital, stempel) per jadwal serta aturan cetaknya (global jadwal atau
// khusus per BA).
type SignatoryService struct {
	repository signatoryRepository
}

func NewSignatoryService(repository signatoryRepository) *SignatoryService {
	return &SignatoryService{repository: repository}
}

func validSignatureDocument(documentKey string) bool {
	_, ok := signatureDocuments[documentKey]
	return ok
}

func (s *SignatoryService) Settings(ctx context.Context, scheduleID, documentKey string, scope auth.RegencyScope) (SignatureSettingsView, error) {
	scheduleID, documentKey = strings.TrimSpace(scheduleID), strings.TrimSpace(documentKey)
	if scheduleID == "" || !validSignatureDocument(documentKey) {
		return SignatureSettingsView{}, ErrInvalidInput
	}
	return s.repository.GetSignatureSettings(ctx, scheduleID, documentKey, scope)
}

// SaveRules menyimpan aturan dari panel BA lalu mengembalikan isi panel terbaru.
func (s *SignatoryService) SaveRules(ctx context.Context, actor auth.Principal, scheduleID, documentKey string, override bool, rules SignatureRules, wet bool, scope auth.RegencyScope, meta auth.ClientMeta) (SignatureSettingsView, error) {
	scheduleID, documentKey = strings.TrimSpace(scheduleID), strings.TrimSpace(documentKey)
	if scheduleID == "" || !validSignatureDocument(documentKey) {
		return SignatureSettingsView{}, ErrInvalidInput
	}
	for role := range rules {
		if !validSignatoryRole(role) {
			return SignatureSettingsView{}, ErrInvalidInput
		}
	}
	// Mode tanda tangan basah hanya untuk BA yang mendukungnya.
	wet = wet && wetSignatureDocuments[documentKey]
	if err := s.repository.SaveSignatureRules(ctx, actor, scheduleID, documentKey, override, rules, wet, scope, meta); err != nil {
		return SignatureSettingsView{}, err
	}
	return s.repository.GetSignatureSettings(ctx, scheduleID, documentKey, scope)
}

// SaveName menyimpan nama penandatangan (huruf kapital seperti data bisnis
// lain; dicetak huruf judul) dan NIP untuk Dinas.
func (s *SignatoryService) SaveName(ctx context.Context, actor auth.Principal, scheduleID, role, name, nip string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if strings.TrimSpace(scheduleID) == "" || !validSignatoryRole(role) {
		return ErrInvalidInput
	}
	return s.repository.SaveSignatoryName(ctx, actor, strings.TrimSpace(scheduleID), role, textnorm.BusinessUpper(strings.Join(strings.Fields(name), " ")), strings.TrimSpace(nip), scope, meta)
}

func (s *SignatoryService) PutAsset(ctx context.Context, actor auth.Principal, scheduleID, role, kind string, image []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if strings.TrimSpace(scheduleID) == "" || !validSignatoryRole(role) || !validSignatoryKind(kind) {
		return ErrInvalidInput
	}
	if len(image) == 0 || len(image) > maxSignatoryImageBytes {
		return ErrSignatoryImageInvalid
	}
	normalized, err := normalizeSignatoryPNG(image)
	if err != nil {
		return ErrSignatoryImageInvalid
	}
	return s.repository.PutSignatoryAsset(ctx, actor, strings.TrimSpace(scheduleID), role, kind, normalized, scope, meta)
}

func (s *SignatoryService) DeleteAsset(ctx context.Context, actor auth.Principal, scheduleID, role, kind string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	if strings.TrimSpace(scheduleID) == "" || !validSignatoryRole(role) || !validSignatoryKind(kind) {
		return ErrInvalidInput
	}
	return s.repository.DeleteSignatoryAsset(ctx, actor, strings.TrimSpace(scheduleID), role, kind, scope, meta)
}

func (s *SignatoryService) OpenAsset(ctx context.Context, scheduleID, role, kind string, scope auth.RegencyScope) ([]byte, error) {
	if strings.TrimSpace(scheduleID) == "" || !validSignatoryRole(role) || !validSignatoryKind(kind) {
		return nil, ErrInvalidInput
	}
	return s.repository.OpenSignatoryAsset(ctx, strings.TrimSpace(scheduleID), role, kind, scope)
}

// normalizeSignatoryPNG menyimpan ulang gambar sebagai PNG 8-bit RGBA tanpa
// interlace: fpdf tidak dapat memuat PNG 16-bit atau interlaced, dan gambar
// yang gagal dimuat akan terlewat diam-diam saat BA dicetak.
func normalizeSignatoryPNG(data []byte) ([]byte, error) {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
