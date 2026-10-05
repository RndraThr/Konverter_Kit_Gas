package bast

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
)

const rakordaFolder = "6. RAKORDA"

type rakordaRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	InsertRakordaUpload(context.Context, auth.Principal, RakordaUpload, auth.RegencyScope, auth.ClientMeta) (RakordaUpload, error)
	ListRakordaUploads(context.Context, string, string, auth.RegencyScope) ([]RakordaUpload, error)
	GetRakordaUpload(context.Context, string, auth.RegencyScope) (RakordaUpload, error)
	DeleteRakordaUpload(context.Context, auth.Principal, string, auth.RegencyScope, auth.ClientMeta) (RakordaUpload, error)
}

type RakordaService struct {
	repository rakordaRepository
	storage    media.Storage
}

func NewRakordaService(repository rakordaRepository, storage media.Storage) *RakordaService {
	return &RakordaService{repository: repository, storage: storage}
}

func (s *RakordaService) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregatePreview{}, err
	}
	if len(logos) == 0 {
		return AggregatePreview{}, ErrBrandingNotConfigured
	}
	snapshot, err := buildRakordaSnapshot(ctxData, settings, logos, documentDate)
	if err != nil {
		return AggregatePreview{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderRakorda(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	return AggregatePreview{PDF: rendered.PDF, Filename: fmt.Sprintf("DAFTAR-HADIR-RAKORDA-%s-%s.pdf", safeFilenamePart(ctxData.RegencyName), documentDate), PageCount: rendered.PageCount}, nil
}

func (s *RakordaService) Upload(ctx context.Context, actor auth.Principal, input RakordaUploadInput, scope auth.RegencyScope, meta auth.ClientMeta) (RakordaUpload, error) {
	input.ScheduleID, input.EventDate, input.OriginalName = strings.TrimSpace(input.ScheduleID), strings.TrimSpace(input.EventDate), filepath.Base(strings.TrimSpace(input.OriginalName))
	if input.ScheduleID == "" || input.OriginalName == "." || input.OriginalName == "" || len(input.Data) == 0 || len(input.Data) > maxRakordaUploadBytes {
		return RakordaUpload{}, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", input.EventDate); err != nil {
		return RakordaUpload{}, ErrInvalidInput
	}
	mimeType := http.DetectContentType(input.Data)
	if mimeType != "application/pdf" && mimeType != "image/jpeg" && mimeType != "image/png" {
		return RakordaUpload{}, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, input.ScheduleID, scope)
	if err != nil {
		return RakordaUpload{}, err
	}
	if strings.TrimSpace(ctxData.ZoneName) == "" || ctxData.ZonePlaceholder {
		return RakordaUpload{}, ErrZoneNotConfigured
	}
	folder, err := media.BuildFolderPath(media.FolderPathInput{ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName, Category: media.FolderBA, Child: rakordaFolder})
	if err != nil {
		return RakordaUpload{}, err
	}
	visibleFilename := formatRakordaUploadFilename(ctxData.RegencyName, input.EventDate, mimeType)
	storageKey, size, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), visibleFilename, folder, bytes.NewReader(input.Data))
	if err != nil {
		return RakordaUpload{}, err
	}
	item, err := s.repository.InsertRakordaUpload(ctx, actor, RakordaUpload{ScheduleID: input.ScheduleID, EventDate: input.EventDate, OriginalName: visibleFilename, MimeType: mimeType, ByteSize: size, StorageKey: storageKey}, scope, meta)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return RakordaUpload{}, err
	}
	return item, nil
}

func formatRakordaUploadFilename(regencyName, eventDate, mimeType string) string {
	date, _ := time.Parse("2006-01-02", eventDate)
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	extension := map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png"}[mimeType]
	return fmt.Sprintf("DAFTAR HADIR RAKORDA - %s - %d %s %d%s", strings.ToUpper(strings.TrimSpace(regencyName)), date.Day(), months[date.Month()], date.Year(), extension)
}

func (s *RakordaService) List(ctx context.Context, scheduleID, eventDate string, scope auth.RegencyScope) ([]RakordaUpload, error) {
	scheduleID, eventDate = strings.TrimSpace(scheduleID), strings.TrimSpace(eventDate)
	if scheduleID == "" {
		return nil, ErrInvalidInput
	}
	if eventDate != "" {
		if _, err := time.Parse("2006-01-02", eventDate); err != nil {
			return nil, ErrInvalidInput
		}
	}
	return s.repository.ListRakordaUploads(ctx, scheduleID, eventDate, scope)
}

func (s *RakordaService) Open(ctx context.Context, id string, scope auth.RegencyScope) (RakordaContent, error) {
	item, err := s.repository.GetRakordaUpload(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return RakordaContent{}, err
	}
	reader, err := s.storage.Open(ctx, item.StorageKey)
	if err != nil {
		return RakordaContent{}, err
	}
	return RakordaContent{Reader: reader, Filename: item.OriginalName, MimeType: item.MimeType}, nil
}

func (s *RakordaService) Delete(ctx context.Context, actor auth.Principal, id string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	item, err := s.repository.GetRakordaUpload(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, item.StorageKey); err != nil {
		return err
	}
	_, err = s.repository.DeleteRakordaUpload(ctx, actor, item.ID, scope, meta)
	return err
}

func safeFilenamePart(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(value)
	return value
}
