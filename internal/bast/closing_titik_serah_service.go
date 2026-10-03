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
	ListClosingRows(context.Context, string, auth.RegencyScope) ([]ClosingRow, error)
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
	return s.repository.ListClosingRows(ctx, scheduleID, scope)
}

func (s *ClosingTitikSerahService) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	ctxData, rows, settings, err := s.load(ctx, scheduleID, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot := buildClosingTitikSerahSnapshot(ctxData, settings, logos, documentDate, 1, rows)
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderClosingTitikSerah(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatClosingFilename(ctxData.RegencyName, documentDate, 1), RecipientCount: snapshot.GrandTotal, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *ClosingTitikSerahService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	ctxData, rows, settings, err := s.load(ctx, scheduleID, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregateDocument{}, err
	}
	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, AggregateDocumentClosingTitikSerah, documentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot := buildClosingTitikSerahSnapshot(ctxData, settings, logos, documentDate, version, rows)
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := RenderClosingTitikSerah(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentClosingTitikSerah, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	filename := formatClosingFilename(ctxData.RegencyName, documentDate, version)
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
		DocumentType: AggregateDocumentClosingTitikSerah, DocumentDate: documentDate, Filename: filename,
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
	return activated.Document, nil
}

func (s *ClosingTitikSerahService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentClosingTitikSerah, strings.TrimSpace(documentDate), scope)
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

func (s *ClosingTitikSerahService) load(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3Context, []ClosingRow, ScheduleSettings, error) {
	if scheduleID == "" {
		return DP3Context{}, nil, ScheduleSettings{}, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	if err := validateClosingSettings(settings); err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	rows, err := s.repository.ListClosingRows(ctx, scheduleID, scope)
	if err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	if err := validateClosingRows(rows); err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	return ctxData, rows, settings, nil
}
