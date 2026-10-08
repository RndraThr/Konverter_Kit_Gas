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

type rakordaRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	InsertRakordaUpload(context.Context, auth.Principal, RakordaUpload, auth.RegencyScope, auth.ClientMeta) (RakordaUpload, error)
	ListRakordaUploads(context.Context, RakordaKind, string, string, auth.RegencyScope) ([]RakordaUpload, error)
	GetRakordaUpload(context.Context, RakordaKind, string, auth.RegencyScope) (RakordaUpload, error)
	DeleteRakordaUpload(context.Context, auth.Principal, RakordaKind, string, auth.RegencyScope, auth.ClientMeta) (RakordaUpload, error)
	ListTrainingParticipants(context.Context, string, string, auth.RegencyScope) ([]ActivityParticipant, error)
	ListDailyRecapDates(context.Context, string, auth.RegencyScope) ([]dailyRecapDateRow, error)
}

// rakordaDocumentSpec menjelaskan satu jenis daftar hadir: dari mana lokasi dan
// jumlah barisnya dibaca, cara merender lembar kosongnya, dan folder Drive
// tempat hasil terisi diarsipkan.
type rakordaDocumentSpec struct {
	kind     RakordaKind
	label    string
	folder   string
	settings func(ScheduleSettings) (location string, rowCount int)
	render   func(RakordaSnapshot, map[string][]byte) (RenderedAggregate, error)
	// participants: Training memakai penerima distribusi per tanggal, bukan lembar kosong.
	participants participantMode
}

var rakordaSpec = rakordaDocumentSpec{
	kind:     RakordaKindRakorda,
	label:    "DAFTAR HADIR RAKORDA",
	folder:   "6. RAKORDA",
	settings: func(s ScheduleSettings) (string, int) { return s.RakordaLocation, s.RakordaRowCount },
	render:   RenderRakorda,
}

var sosialisasiSpec = rakordaDocumentSpec{
	kind:     RakordaKindSosialisasi,
	label:    "BA SOSIALISASI",
	folder:   "7. SOSIALISASI",
	settings: func(s ScheduleSettings) (string, int) { return s.SosialisasiLocation, s.SosialisasiRowCount },
	render:   RenderSosialisasi,
}

var training10Spec = rakordaDocumentSpec{
	kind:     RakordaKindTraining10,
	label:    "BA TRAINING 10%",
	folder:   "8. TRAINING 10%",
	settings: func(s ScheduleSettings) (string, int) { return s.Training10Location, s.Training10RowCount },
	render:   RenderTraining10,

	participants: participantsTenPercent,
}

var training100Spec = rakordaDocumentSpec{
	kind:     RakordaKindTraining100,
	label:    "BA TRAINING 100%",
	folder:   "9. TRAINING 100%",
	settings: func(s ScheduleSettings) (string, int) { return s.Training100Location, s.Training100RowCount },
	render:   RenderTraining100,

	participants: participantsAll,
}

type RakordaService struct {
	spec       rakordaDocumentSpec
	repository rakordaRepository
	storage    media.Storage
}

func NewRakordaService(repository rakordaRepository, storage media.Storage) *RakordaService {
	return &RakordaService{spec: rakordaSpec, repository: repository, storage: storage}
}

func NewSosialisasiService(repository rakordaRepository, storage media.Storage) *RakordaService {
	return &RakordaService{spec: sosialisasiSpec, repository: repository, storage: storage}
}

func NewTraining10Service(repository rakordaRepository, storage media.Storage) *RakordaService {
	return &RakordaService{spec: training10Spec, repository: repository, storage: storage}
}

func NewTraining100Service(repository rakordaRepository, storage media.Storage) *RakordaService {
	return &RakordaService{spec: training100Spec, repository: repository, storage: storage}
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
	var participants []ActivityParticipant
	if s.spec.participants != participantsNone {
		if participants, err = s.repository.ListTrainingParticipants(ctx, ctxData.ScheduleID, strings.TrimSpace(documentDate), scope); err != nil {
			return AggregatePreview{}, err
		}
	}
	snapshot, err := buildRakordaSnapshot(s.spec, ctxData, settings, logos, documentDate, participants)
	if err != nil {
		return AggregatePreview{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := s.spec.render(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	filename := fmt.Sprintf("%s-%s-%s.pdf", safeFilenamePart(s.spec.label), safeFilenamePart(ctxData.RegencyName), documentDate)
	return AggregatePreview{PDF: rendered.PDF, Filename: filename, RecipientCount: len(snapshot.Participants), PageCount: rendered.PageCount}, nil
}

// Dates mengembalikan tanggal distribusi (seperti BA perorangan) beserta jumlah
// peserta Training. Hanya untuk jenis yang memakai peserta.
func (s *RakordaService) Dates(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]TrainingDate, error) {
	if s.spec.participants == participantsNone {
		return nil, ErrNotFound
	}
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return nil, err
	}
	rows, err := s.repository.ListDailyRecapDates(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return nil, err
	}
	dates := make([]TrainingDate, 0, len(rows))
	for _, row := range rows {
		count := row.RecipientCount
		if s.spec.participants == participantsTenPercent {
			count = tenPercentCount(count)
		}
		dates = append(dates, TrainingDate{LocalDate: row.LocalDate, RecipientCount: row.RecipientCount, ParticipantCount: count})
	}
	return dates, nil
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
	folder, err := media.BuildFolderPath(media.FolderPathInput{ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName, Category: media.FolderBA, Child: s.spec.folder})
	if err != nil {
		return RakordaUpload{}, err
	}
	visibleFilename := formatRakordaUploadFilename(s.spec.label, ctxData.RegencyName, input.EventDate, mimeType)
	storageKey, size, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), visibleFilename, folder, bytes.NewReader(input.Data))
	if err != nil {
		return RakordaUpload{}, err
	}
	item, err := s.repository.InsertRakordaUpload(ctx, actor, RakordaUpload{ScheduleID: input.ScheduleID, DocumentKind: s.spec.kind, EventDate: input.EventDate, OriginalName: visibleFilename, MimeType: mimeType, ByteSize: size, StorageKey: storageKey}, scope, meta)
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return RakordaUpload{}, err
	}
	return item, nil
}

func formatRakordaUploadFilename(label, regencyName, eventDate, mimeType string) string {
	date, _ := time.Parse("2006-01-02", eventDate)
	months := [...]string{"", "JANUARI", "FEBRUARI", "MARET", "APRIL", "MEI", "JUNI", "JULI", "AGUSTUS", "SEPTEMBER", "OKTOBER", "NOVEMBER", "DESEMBER"}
	extension := map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png"}[mimeType]
	return fmt.Sprintf("%s - %s - %d %s %d%s", label, strings.ToUpper(strings.TrimSpace(regencyName)), date.Day(), months[date.Month()], date.Year(), extension)
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
	return s.repository.ListRakordaUploads(ctx, s.spec.kind, scheduleID, eventDate, scope)
}

func (s *RakordaService) Open(ctx context.Context, id string, scope auth.RegencyScope) (RakordaContent, error) {
	item, err := s.repository.GetRakordaUpload(ctx, s.spec.kind, strings.TrimSpace(id), scope)
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
	item, err := s.repository.GetRakordaUpload(ctx, s.spec.kind, strings.TrimSpace(id), scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, item.StorageKey); err != nil {
		return err
	}
	_, err = s.repository.DeleteRakordaUpload(ctx, actor, s.spec.kind, item.ID, scope, meta)
	return err
}

func safeFilenamePart(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(value)
	return value
}
