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

const dailyRecapFolder = "3. REKAP HARIAN"

type dailyRecapRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListDailyRecapDates(context.Context, string, auth.RegencyScope) ([]dailyRecapDateRow, error)
	ListDailyRecapRecipients(context.Context, string, string, auth.RegencyScope) ([]DailyRecapRecipient, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	ListActiveAggregatesForType(context.Context, string, string, auth.RegencyScope) ([]AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
}

type DailyRecapService struct {
	repository dailyRecapRepository
	storage    media.Storage
	location   *time.Location
}

func NewDailyRecapService(repository dailyRecapRepository, storage media.Storage, location *time.Location) *DailyRecapService {
	if location == nil {
		location = time.UTC
	}
	return &DailyRecapService{repository: repository, storage: storage, location: location}
}

func (s *DailyRecapService) Dates(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]DailyRecapDate, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	contextErr := validateDP3Context(ctxData)
	if _, settingsErr := s.repository.GetScheduleSettings(ctx, scheduleID, scope); settingsErr != nil {
		contextErr = settingsErr
	}
	dateRows, err := s.repository.ListDailyRecapDates(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	activeDocs, err := s.repository.ListActiveAggregatesForType(ctx, scheduleID, AggregateDocumentDailyRecap, scope)
	if err != nil {
		return nil, err
	}
	docByDate := map[string]AggregateDocument{}
	for i := range activeDocs {
		docByDate[activeDocs[i].DocumentDate] = activeDocs[i]
	}
	dates := make([]DailyRecapDate, 0, len(dateRows))
	for _, row := range dateRows {
		status := "ready"
		if contextErr != nil {
			status = dp3ValidationStatus(contextErr)
		}
		date := DailyRecapDate{LocalDate: row.LocalDate, RecipientCount: row.RecipientCount, ValidationStatus: status}
		if doc, ok := docByDate[row.LocalDate]; ok {
			date.Document = &doc
		}
		dates = append(dates, date)
	}
	return dates, nil
}

func (s *DailyRecapService) Recipients(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]DailyRecapRecipient, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3Context(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	return s.repository.ListDailyRecapRecipients(ctx, scheduleID, localDate, scope)
}

func (s *DailyRecapService) Preview(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	localDate = strings.TrimSpace(localDate)
	ctxData, recipients, settings, err := s.load(ctx, scheduleID, localDate, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot := buildDailyRecapSnapshot(ctxData, settings, logos, localDate, recipients)
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderDailyRecap(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatDailyRecapFilename(ctxData.RegencyName, localDate, 1), RecipientCount: len(recipients), PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *DailyRecapService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, localDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	localDate = strings.TrimSpace(localDate)
	ctxData, recipients, settings, err := s.load(ctx, scheduleID, localDate, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot := buildDailyRecapSnapshot(ctxData, settings, logos, localDate, recipients)
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := RenderDailyRecap(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentDailyRecap, localDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, AggregateDocumentDailyRecap, localDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	filename := formatDailyRecapFilename(ctxData.RegencyName, localDate, version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: dailyRecapFolder,
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
		DocumentType: AggregateDocumentDailyRecap, DocumentDate: localDate, Filename: filename,
		RecipientCount: len(recipients), PageCount: rendered.PageCount, Checksum: checksum,
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

func (s *DailyRecapService) Documents(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentDailyRecap, strings.TrimSpace(localDate), scope)
}

func (s *DailyRecapService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
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

func (s *DailyRecapService) load(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) (DP3Context, []DailyRecapRecipient, ScheduleSettings, error) {
	if scheduleID == "" || localDate == "" {
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
	if err := validateDailyRecapSettings(settings); err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	recipients, err := s.repository.ListDailyRecapRecipients(ctx, scheduleID, localDate, scope)
	if err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	if err := validateDailyRecapRecipients(recipients); err != nil {
		return DP3Context{}, nil, ScheduleSettings{}, err
	}
	return ctxData, recipients, settings, nil
}
