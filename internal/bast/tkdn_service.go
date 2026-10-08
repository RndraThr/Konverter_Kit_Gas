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

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
)

const tkdnFolder = "12. TKDN"

type tkdnRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListClosingKabupatenRows(context.Context, string, auth.RegencyScope) ([]ClosingKabupatenRow, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetTKDNProfile(context.Context, string) (TKDNProfile, error)
	UpsertTKDNProfile(context.Context, auth.Principal, TKDNProfile, auth.ClientMeta) (TKDNProfile, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
	itemResolverRepository
}

// TKDNService menerbitkan Realisasi TKDN per jadwal kabupaten dari daftar
// TKDN program dan jumlah paket yang sudah didistribusikan.
type TKDNService struct {
	repository tkdnRepository
	storage    media.Storage
}

func NewTKDNService(repository tkdnRepository, storage media.Storage) *TKDNService {
	return &TKDNService{repository: repository, storage: storage}
}

func (s *TKDNService) Profile(ctx context.Context, programID string) (TKDNProfile, error) {
	programID = strings.TrimSpace(programID)
	if programID == "" {
		return TKDNProfile{}, ErrInvalidInput
	}
	return s.repository.GetTKDNProfile(ctx, programID)
}

func (s *TKDNService) SaveProfile(ctx context.Context, actor auth.Principal, profile TKDNProfile, meta auth.ClientMeta) (TKDNProfile, error) {
	if err := profile.normalize(); err != nil {
		return TKDNProfile{}, err
	}
	if _, err := s.repository.GetTKDNProfile(ctx, profile.ProgramID); err != nil {
		return TKDNProfile{}, err
	}
	return s.repository.UpsertTKDNProfile(ctx, actor, profile, meta)
}

func (s *TKDNService) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (TKDNSummary, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return TKDNSummary{}, err
	}
	rows, total, err := s.closingRows(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return TKDNSummary{}, err
	}
	profile, err := s.repository.GetTKDNProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return TKDNSummary{}, err
	}
	entries, resolved, _, err := resolveScheduleItems(ctx, s.repository, ctxData, rows)
	if err != nil {
		return TKDNSummary{}, err
	}
	return TKDNSummary{TotalPackages: total, Profile: profile, Items: tkdnItemsFor(profile.Rows, resolved), Entries: entries}, nil
}

func (s *TKDNService) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID, documentDate = strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate)
	snapshot, ctxData, err := s.snapshot(ctx, scheduleID, documentDate, 1, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, snapshot.Logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderTKDN(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatTKDNFilename(ctxData.RegencyName, documentDate, 1), RecipientCount: snapshot.TotalPackages, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *TKDNService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID, documentDate = strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate)
	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, AggregateDocumentTKDN, documentDate)
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
	rendered, err := RenderTKDN(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentTKDN, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	filename := formatTKDNFilename(ctxData.RegencyName, documentDate, version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: tkdnFolder,
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
		DocumentType: AggregateDocumentTKDN, DocumentDate: documentDate, Filename: filename,
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

func (s *TKDNService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentTKDN, strings.TrimSpace(documentDate), scope)
}

func (s *TKDNService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
	doc, err := s.repository.GetAggregateByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return AggregateContent{}, err
	}
	if doc.DocumentType != AggregateDocumentTKDN {
		return AggregateContent{}, ErrNotFound
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return AggregateContent{}, err
	}
	return AggregateContent{Reader: reader, Filename: doc.Filename}, nil
}

func (s *TKDNService) snapshot(ctx context.Context, scheduleID, documentDate string, version int, scope auth.RegencyScope) (TKDNSnapshot, DP3Context, error) {
	if scheduleID == "" || !validRequiredDate(documentDate) {
		return TKDNSnapshot{}, DP3Context{}, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	profile, err := s.repository.GetTKDNProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	rows, total, err := s.closingRows(ctx, scheduleID, scope)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	if total == 0 {
		return TKDNSnapshot{}, DP3Context{}, ErrAggregateNoRecipients
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	if len(logos) == 0 {
		return TKDNSnapshot{}, DP3Context{}, ErrBrandingNotConfigured
	}
	_, resolved, _, err := resolveScheduleItems(ctx, s.repository, ctxData, rows)
	if err != nil {
		return TKDNSnapshot{}, DP3Context{}, err
	}
	return buildTKDNSnapshot(ctxData, settings, profile, tkdnItemsFor(profile.Rows, resolved), logos, documentDate, version, total), ctxData, nil
}

func (s *TKDNService) closingRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingKabupatenRow, int, error) {
	rows, err := s.repository.ListClosingKabupatenRows(ctx, scheduleID, scope)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return rows, total, nil
}
