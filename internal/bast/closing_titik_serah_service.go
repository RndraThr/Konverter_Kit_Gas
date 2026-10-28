package bast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
)

const closingFolder = "4. CLOSING TITIK SERAH"

type closingRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListHandoverLocations(context.Context, string, auth.RegencyScope) ([]HandoverLocation, error)
	ListHandoverRecipients(context.Context, string, auth.RegencyScope) ([]handoverRecipient, error)
	StaleClosingKabupaten(context.Context, string) error
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
}

type ClosingTitikSerahService struct {
	repository closingRepository
	storage    media.Storage
	location   *time.Location
}

func NewClosingTitikSerahService(repository closingRepository, storage media.Storage, location *time.Location) *ClosingTitikSerahService {
	if location == nil {
		location = time.UTC
	}
	return &ClosingTitikSerahService{repository: repository, storage: storage, location: location}
}

func (s *ClosingTitikSerahService) Rows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingRow, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3Context(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	_, rows, err := s.locationRows(ctx, scheduleID, scope)
	return rows, err
}

// locationRows mengembalikan titik serah terpilih (dari konteks) dan rekap
// distribusi selesai milik lokasi itu.
func (s *ClosingTitikSerahService) locationRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) (HandoverLocation, []ClosingRow, error) {
	locations, err := s.repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil {
		return HandoverLocation{}, nil, err
	}
	selected := handoverLocationFrom(ctx)
	// Tata letak tanda tangan sama untuk semua lokasi: cukup lokasi pertama.
	if selected == "" && signatureLayoutFrom(ctx) != nil && len(locations) > 0 {
		selected = locations[0].ID
	}
	location, err := pickHandoverLocation(locations, selected)
	if err != nil {
		return HandoverLocation{}, nil, err
	}
	recipients, err := s.repository.ListHandoverRecipients(ctx, scheduleID, scope)
	if err != nil {
		return HandoverLocation{}, nil, err
	}
	return location, groupClosingRows(closingRawRowsFor(recipients, location.ID)), nil
}

func (s *ClosingTitikSerahService) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	ctxData, location, rows, settings, err := s.load(ctx, scheduleID, scope, false)
	if err != nil {
		return AggregatePreview{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregatePreview{}, err
	}
	documentType := closingDocumentType(location.ID)
	snapshot := buildClosingTitikSerahSnapshot(ctxData, settings, logos, documentDate, nextVersionOrFirst(ctx, s.repository, scheduleID, documentType, documentDate), rows)
	snapshot.HandoverLocation = location.Name
	marks, markImages, err := loadSignatoryMarks(ctx, s.repository, ctxData.ScheduleID, "closing-titik-serah")
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot.SignatureMarks = marks
	wet, wetImages, err := loadWetSignature(ctx, s.repository, ctxData.ScheduleID, closingWetKey(location.ID), documentDate, snapshot)
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot.Wet = wet
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	mergeImages(logoBytes, markImages)
	mergeImages(logoBytes, wetImages)
	rendered, err := RenderClosingTitikSerah(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatClosingFilename(ctxData.RegencyName, location.Name, documentDate, 1), RecipientCount: snapshot.GrandTotal, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *ClosingTitikSerahService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	ctxData, location, rows, settings, err := s.load(ctx, scheduleID, scope, true)
	if err != nil {
		return AggregateDocument{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregateDocument{}, err
	}
	documentType := closingDocumentType(location.ID)
	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, documentType, documentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot := buildClosingTitikSerahSnapshot(ctxData, settings, logos, documentDate, version, rows)
	snapshot.HandoverLocation = location.Name
	marks, markImages, err := loadSignatoryMarks(ctx, s.repository, ctxData.ScheduleID, "closing-titik-serah")
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot.SignatureMarks = marks
	wet, wetImages, err := loadWetSignature(ctx, s.repository, ctxData.ScheduleID, closingWetKey(location.ID), documentDate, snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	if wet.Enabled && !wet.Scanned {
		return AggregateDocument{}, ErrWetSignatureRequired
	}
	snapshot.Wet = wet
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregateDocument{}, err
	}
	mergeImages(logoBytes, markImages)
	mergeImages(logoBytes, wetImages)
	rendered, err := RenderClosingTitikSerah(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, documentType, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	filename := formatClosingFilename(ctxData.RegencyName, location.Name, documentDate, version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: closingFolder,
	})
	if err != nil {
		return AggregateDocument{}, err
	}
	storageKey, _, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), filename, folderPath, bytes.NewReader(rendered.PDF))
	if err != nil {
		return AggregateDocument{}, err
	}
	expectedActiveID := ""
	if activeErr == nil {
		expectedActiveID = active.ID
	}
	activated, err := s.repository.ActivateAggregate(ctx, actor, AggregateActivation{
		ScheduleID: scheduleID, ProgramID: ctxData.ProgramID, RegencyID: ctxData.RegencyID,
		DocumentType: documentType, DocumentDate: documentDate, Filename: filename,
		RecipientCount: snapshot.GrandTotal, PageCount: rendered.PageCount, Checksum: checksum,
		StorageKey: storageKey, Snapshot: snapshotJSON, ExpectedVersion: version, ExpectedActiveID: expectedActiveID,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return AggregateDocument{}, err
	}
	if activated.Unchanged {
		_ = s.storage.Delete(context.Background(), storageKey)
		return activated.Document, nil
	}
	if activated.OldStorageKey != "" && activated.OldStorageKey != storageKey {
		if deleteErr := s.storage.Delete(ctx, activated.OldStorageKey); deleteErr != nil {
			_ = s.repository.RecordAggregateCleanupFailure(context.Background(), activated.Document.ID, fmt.Sprintf("cleanup old storage key %s: %v", activated.OldStorageKey, deleteErr))
		}
	}
	// Closing Kabupaten merekap BA Serah Terima final tiap lokasi.
	if err := s.repository.StaleClosingKabupaten(ctx, scheduleID); err != nil {
		return AggregateDocument{}, err
	}
	return activated.Document, nil
}

// Documents mengembalikan versi BA Serah Terima titik serah terpilih (dari
// konteks) pada satu tanggal.
func (s *ClosingTitikSerahService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	locations, err := s.repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	location, err := pickHandoverLocation(locations, handoverLocationFrom(ctx))
	if err != nil {
		return nil, err
	}
	return s.repository.ListAggregates(ctx, scheduleID, closingDocumentType(location.ID), strings.TrimSpace(documentDate), scope)
}

func (s *ClosingTitikSerahService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
	doc, err := s.repository.GetAggregateByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return AggregateContent{}, err
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return AggregateContent{}, err
	}
	return AggregateContent{Reader: reader, Filename: doc.Filename}, nil
}

func (s *ClosingTitikSerahService) load(ctx context.Context, scheduleID string, scope auth.RegencyScope, strict bool) (DP3Context, HandoverLocation, []ClosingRow, ScheduleSettings, error) {
	fail := func(err error) (DP3Context, HandoverLocation, []ClosingRow, ScheduleSettings, error) {
		return DP3Context{}, HandoverLocation{}, nil, ScheduleSettings{}, err
	}
	if scheduleID == "" {
		return fail(ErrInvalidInput)
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return fail(err)
	}
	if err := validateDP3Context(ctxData); err != nil {
		return fail(err)
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return fail(err)
	}
	// Preview (strict=false) tidak menuntut pengaturan dan data mesin lengkap.
	if strict {
		if err := validateClosingSettings(settings); err != nil {
			return fail(err)
		}
	}
	location, rows, err := s.locationRows(ctx, scheduleID, scope)
	if err != nil {
		return fail(err)
	}
	if !strict && len(rows) == 0 {
		return fail(ErrAggregateNoRecipients)
	}
	if strict {
		if err := validateClosingRows(rows); err != nil {
			return fail(err)
		}
	}
	return ctxData, location, rows, settings, nil
}
