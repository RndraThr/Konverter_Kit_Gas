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

const servisBerkalaFolder = "11. SERVIS BERKALA"

type servisBerkalaRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListClosingKabupatenRows(context.Context, string, auth.RegencyScope) ([]ClosingKabupatenRow, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
}

// ServisBerkalaService menerbitkan BA Agenda Servis Berkala per jadwal
// kabupaten. Jumlah paket pada nomor dokumen dihitung dari distribusi selesai
// (sama dengan total Closing Kabupaten); jadwal servis dari pengaturan jadwal.
type ServisBerkalaService struct {
	repository servisBerkalaRepository
	storage    media.Storage
	location   *time.Location
}

func NewServisBerkalaService(repository servisBerkalaRepository, storage media.Storage, location *time.Location) *ServisBerkalaService {
	if location == nil {
		location = time.UTC
	}
	return &ServisBerkalaService{repository: repository, storage: storage, location: location}
}

func (s *ServisBerkalaService) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (ServisBerkalaSummary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3Context(ctx, scheduleID, scope); err != nil {
		return ServisBerkalaSummary{}, err
	}
	total, err := s.totalPackages(ctx, scheduleID, scope)
	if err != nil {
		return ServisBerkalaSummary{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return ServisBerkalaSummary{}, err
	}
	periods := servisPeriods(settings)
	return ServisBerkalaSummary{TotalPackages: total, Services: periods, ScheduleReady: validateServisPeriods(periods) == nil}, nil
}

func (s *ServisBerkalaService) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID, documentDate = strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate)
	snapshot, ctxData, err := s.snapshot(ctx, scheduleID, documentDate, 1, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, snapshot.Logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderServisBerkala(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatServisBerkalaFilename(ctxData.RegencyName, documentDate, 1), RecipientCount: snapshot.TotalPackages, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *ServisBerkalaService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID, documentDate = strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate)
	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, AggregateDocumentServisBerkala, documentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot, ctxData, err := s.snapshot(ctx, scheduleID, documentDate, version, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, snapshot.Logos)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := RenderServisBerkala(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentServisBerkala, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	filename := formatServisBerkalaFilename(ctxData.RegencyName, documentDate, version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: servisBerkalaFolder,
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
		DocumentType: AggregateDocumentServisBerkala, DocumentDate: documentDate, Filename: filename,
		RecipientCount: snapshot.TotalPackages, PageCount: rendered.PageCount, Checksum: checksum,
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

func (s *ServisBerkalaService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentServisBerkala, strings.TrimSpace(documentDate), scope)
}

func (s *ServisBerkalaService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
	doc, err := s.repository.GetAggregateByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return AggregateContent{}, err
	}
	if doc.DocumentType != AggregateDocumentServisBerkala {
		return AggregateContent{}, ErrNotFound
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return AggregateContent{}, err
	}
	return AggregateContent{Reader: reader, Filename: doc.Filename}, nil
}

func (s *ServisBerkalaService) snapshot(ctx context.Context, scheduleID, documentDate string, version int, scope auth.RegencyScope) (ServisBerkalaSnapshot, DP3Context, error) {
	if scheduleID == "" || !validRequiredDate(documentDate) {
		return ServisBerkalaSnapshot{}, DP3Context{}, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	if err := validateServisPeriods(servisPeriods(settings)); err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	total, err := s.totalPackages(ctx, scheduleID, scope)
	if err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	if total == 0 {
		return ServisBerkalaSnapshot{}, DP3Context{}, ErrAggregateNoRecipients
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return ServisBerkalaSnapshot{}, DP3Context{}, err
	}
	if len(logos) == 0 {
		return ServisBerkalaSnapshot{}, DP3Context{}, ErrBrandingNotConfigured
	}
	return buildServisBerkalaSnapshot(ctxData, settings, logos, documentDate, version, total), ctxData, nil
}

func (s *ServisBerkalaService) totalPackages(ctx context.Context, scheduleID string, scope auth.RegencyScope) (int, error) {
	rows, err := s.repository.ListClosingKabupatenRows(ctx, scheduleID, scope)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return total, nil
}
