package bast

import (
	"bytes"
	"context"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
)

const brandingFolder = "BA BRANDING"

type brandingRepository interface {
	GetProgramCode(context.Context, string) (string, error)
	ListLogos(context.Context, string) ([]LogoAsset, error)
	SaveLogo(context.Context, auth.Principal, LogoAsset, auth.ClientMeta) (LogoAsset, string, error)
	UpdateLogo(context.Context, auth.Principal, LogoPatchInput, auth.ClientMeta) (LogoAsset, error)
	GetLogo(context.Context, string, string) (LogoAsset, error)
}

// BrandingService manages the per-program BA logo set. It is independent of the
// bundle/finalize pipeline so the logo configuration can evolve on its own.
type BrandingService struct {
	repository brandingRepository
	storage    media.Storage
}

func NewBrandingService(repository brandingRepository, storage media.Storage) *BrandingService {
	return &BrandingService{repository: repository, storage: storage}
}

// ListBranding returns a program's BA logos, confirming the program exists.
func (s *BrandingService) ListBranding(ctx context.Context, programID string) ([]LogoAsset, error) {
	programID = strings.TrimSpace(programID)
	if _, err := s.repository.GetProgramCode(ctx, programID); err != nil {
		return nil, err
	}
	return s.repository.ListLogos(ctx, programID)
}

// UploadLogo validates and stores a logo for a program slot.
func (s *BrandingService) UploadLogo(ctx context.Context, actor auth.Principal, input LogoUploadInput, meta auth.ClientMeta) (LogoAsset, error) {
	if s.storage == nil {
		return LogoAsset{}, ErrInvalidInput
	}
	mimeType, err := input.normalizeUpload()
	if err != nil {
		return LogoAsset{}, err
	}
	programCode, err := s.repository.GetProgramCode(ctx, input.ProgramID)
	if err != nil {
		return LogoAsset{}, err
	}
	storageKey, size, checksum, err := s.storage.Put(ctx, uuid.NewString(), []string{"PROGRAM ASSETS", programCode, brandingFolder}, bytes.NewReader(input.Data))
	if err != nil {
		return LogoAsset{}, err
	}
	logo, oldKey, err := s.repository.SaveLogo(ctx, actor, LogoAsset{ProgramID: input.ProgramID, SlotCode: input.SlotCode, StorageKey: storageKey, OriginalFilename: input.OriginalFilename, MimeType: mimeType, ByteSize: size, Checksum: checksum, SortOrder: input.SortOrder, MaxWidthMM: input.MaxWidthMM, MaxHeightMM: input.MaxHeightMM, IsVisible: true}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return LogoAsset{}, err
	}
	if oldKey != "" && oldKey != storageKey {
		_ = s.storage.Delete(context.Background(), oldKey)
	}
	return logo, nil
}

// PatchLogo updates ordering, size bounds, and visibility.
func (s *BrandingService) PatchLogo(ctx context.Context, actor auth.Principal, input LogoPatchInput, meta auth.ClientMeta) (LogoAsset, error) {
	if err := input.normalizePatch(); err != nil {
		return LogoAsset{}, err
	}
	return s.repository.UpdateLogo(ctx, actor, input, meta)
}

// OpenLogo streams a stored logo for preview.
func (s *BrandingService) OpenLogo(ctx context.Context, programID, logoID string) (LogoContent, error) {
	if s.storage == nil {
		return LogoContent{}, ErrNotFound
	}
	logo, err := s.repository.GetLogo(ctx, strings.TrimSpace(programID), strings.TrimSpace(logoID))
	if err != nil {
		return LogoContent{}, err
	}
	reader, err := s.storage.Open(ctx, logo.StorageKey)
	if err != nil {
		return LogoContent{}, err
	}
	return LogoContent{Reader: reader, MimeType: logo.MimeType, Filename: logo.OriginalFilename}, nil
}
