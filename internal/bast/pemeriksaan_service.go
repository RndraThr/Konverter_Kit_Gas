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

const pemeriksaanFolder = "10. BA PEMERIKSAAN"

type pemeriksaanRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListClosingKabupatenRows(context.Context, string, auth.RegencyScope) ([]ClosingKabupatenRow, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetPemeriksaanProfile(context.Context, string) (PemeriksaanProfile, error)
	UpsertPemeriksaanProfile(context.Context, auth.Principal, PemeriksaanProfile, auth.ClientMeta) (PemeriksaanProfile, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregatesByTypePrefix(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
	itemResolverRepository
}

// PemeriksaanService menerbitkan BA Pemeriksaan Barang: satu template, satu
// dokumen berversi per form barang. Preview menggabungkan semua form ke satu
// PDF agar mudah diperiksa sekaligus.
type PemeriksaanService struct {
	repository pemeriksaanRepository
	storage    media.Storage
}

func NewPemeriksaanService(repository pemeriksaanRepository, storage media.Storage) *PemeriksaanService {
	return &PemeriksaanService{repository: repository, storage: storage}
}

func (s *PemeriksaanService) Profile(ctx context.Context, programID string) (PemeriksaanProfile, error) {
	programID = strings.TrimSpace(programID)
	if programID == "" {
		return PemeriksaanProfile{}, ErrInvalidInput
	}
	return s.repository.GetPemeriksaanProfile(ctx, programID)
}

func (s *PemeriksaanService) SaveProfile(ctx context.Context, actor auth.Principal, profile PemeriksaanProfile, meta auth.ClientMeta) (PemeriksaanProfile, error) {
	if err := profile.normalize(); err != nil {
		return PemeriksaanProfile{}, err
	}
	if _, err := s.repository.GetPemeriksaanProfile(ctx, profile.ProgramID); err != nil {
		return PemeriksaanProfile{}, err
	}
	return s.repository.UpsertPemeriksaanProfile(ctx, actor, profile, meta)
}

func (s *PemeriksaanService) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (PemeriksaanSummary, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return PemeriksaanSummary{}, err
	}
	rows, total, err := s.closingRows(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return PemeriksaanSummary{}, err
	}
	profile, err := s.repository.GetPemeriksaanProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return PemeriksaanSummary{}, err
	}
	entries, resolved, _, err := resolveScheduleItems(ctx, s.repository, ctxData, rows)
	if err != nil {
		return PemeriksaanSummary{}, err
	}
	forms := make([]PemeriksaanFormSummary, 0, len(profile.Forms))
	for _, form := range profile.Forms {
		formRows, poNumber := pemeriksaanRowsFor(form, entries, resolved)
		forms = append(forms, PemeriksaanFormSummary{Code: form.Code, Title: form.Title, PONumber: poNumber, Rows: formRows})
	}
	return PemeriksaanSummary{TotalPackages: total, Profile: profile, Forms: forms, Entries: entries}, nil
}

// Preview merender semua form (atau hanya formCodes bila diisi) ke satu PDF.
func (s *PemeriksaanService) Preview(ctx context.Context, scheduleID, documentDate string, formCodes []string, scope auth.RegencyScope) (AggregatePreview, error) {
	snapshots, ctxData, err := s.snapshots(ctx, strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate), formCodes, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, snapshots[0].Logos)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderPemeriksaan(snapshots, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	filename := fmt.Sprintf("BA PEMERIKSAAN - %s - %s.pdf", strings.ToUpper(strings.Join(strings.Fields(ctxData.RegencyName), " ")), documentDate)
	return AggregatePreview{PDF: rendered.PDF, Filename: filename, RecipientCount: snapshots[0].TotalPackages, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

// Finalize menerbitkan satu PDF berversi per form (semua form, atau hanya
// formCodes bila diisi) dan mengembalikan dokumen aktif masing-masing.
func (s *PemeriksaanService) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, formCodes []string, scope auth.RegencyScope, meta auth.ClientMeta) ([]AggregateDocument, error) {
	scheduleID, documentDate = strings.TrimSpace(scheduleID), strings.TrimSpace(documentDate)
	snapshots, ctxData, err := s.snapshots(ctx, scheduleID, documentDate, formCodes, scope)
	if err != nil {
		return nil, err
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, snapshots[0].Logos)
	if err != nil {
		return nil, err
	}
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: pemeriksaanFolder,
	})
	if err != nil {
		return nil, err
	}
	documents := make([]AggregateDocument, 0, len(snapshots))
	for _, snapshot := range snapshots {
		doc, err := s.finalizeForm(ctx, actor, ctxData, snapshot, logoBytes, folderPath, meta, scope)
		if err != nil {
			return nil, fmt.Errorf("finalize form %s: %w", snapshot.Form.Code, err)
		}
		documents = append(documents, doc)
	}
	return documents, nil
}

func (s *PemeriksaanService) finalizeForm(ctx context.Context, actor auth.Principal, ctxData DP3Context, snapshot PemeriksaanSnapshot, logoBytes map[string][]byte, folderPath []string, meta auth.ClientMeta, scope auth.RegencyScope) (AggregateDocument, error) {
	documentType := snapshot.DocumentType
	version, err := s.repository.NextAggregateVersion(ctx, ctxData.ScheduleID, documentType, snapshot.DocumentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := RenderPemeriksaan([]PemeriksaanSnapshot{snapshot}, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])
	active, activeErr := s.repository.GetActiveAggregate(ctx, ctxData.ScheduleID, documentType, snapshot.DocumentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	filename := formatPemeriksaanFilename(snapshot.Form.Title, ctxData.RegencyName, snapshot.DocumentDate, version)
	storageKey, _, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), filename, folderPath, bytes.NewReader(rendered.PDF))
	if err != nil {
		return AggregateDocument{}, err
	}
	expectedActiveID := ""
	if activeErr == nil {
		expectedActiveID = active.ID
	}
	activated, err := s.repository.ActivateAggregate(ctx, actor, AggregateActivation{
		ScheduleID: ctxData.ScheduleID, ProgramID: ctxData.ProgramID, RegencyID: ctxData.RegencyID,
		DocumentType: documentType, DocumentDate: snapshot.DocumentDate, Filename: filename,
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

// Documents mengembalikan semua versi dokumen BA Pemeriksaan (semua form)
// pada satu tanggal dokumen.
func (s *PemeriksaanService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregatesByTypePrefix(ctx, strings.TrimSpace(scheduleID), pemeriksaanDocumentPrefix, strings.TrimSpace(documentDate), scope)
}

func (s *PemeriksaanService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
	doc, err := s.repository.GetAggregateByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return AggregateContent{}, err
	}
	if !strings.HasPrefix(doc.DocumentType, pemeriksaanDocumentPrefix) {
		return AggregateContent{}, ErrNotFound
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return AggregateContent{}, err
	}
	return AggregateContent{Reader: reader, Filename: doc.Filename}, nil
}

func (s *PemeriksaanService) snapshots(ctx context.Context, scheduleID, documentDate string, formCodes []string, scope auth.RegencyScope) ([]PemeriksaanSnapshot, DP3Context, error) {
	if scheduleID == "" || !validRequiredDate(documentDate) {
		return nil, DP3Context{}, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return nil, DP3Context{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return nil, DP3Context{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return nil, DP3Context{}, err
	}
	profile, err := s.repository.GetPemeriksaanProfile(ctx, ctxData.ProgramID)
	if err != nil {
		return nil, DP3Context{}, err
	}
	closingRows, total, err := s.closingRows(ctx, scheduleID, scope)
	if err != nil {
		return nil, DP3Context{}, err
	}
	if total == 0 {
		return nil, DP3Context{}, ErrAggregateNoRecipients
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return nil, DP3Context{}, err
	}
	if len(logos) == 0 {
		return nil, DP3Context{}, ErrBrandingNotConfigured
	}
	entries, resolved, _, err := resolveScheduleItems(ctx, s.repository, ctxData, closingRows)
	if err != nil {
		return nil, DP3Context{}, err
	}
	wanted := map[string]bool{}
	for _, code := range formCodes {
		if code = strings.TrimSpace(code); code != "" {
			wanted[code] = true
		}
	}
	var snapshots []PemeriksaanSnapshot
	for _, form := range profile.Forms {
		if len(wanted) > 0 && !wanted[form.Code] {
			continue
		}
		// Form yang barangnya tidak ada di template jadwal tidak diterbitkan.
		rows, poNumber := pemeriksaanRowsFor(form, entries, resolved)
		if len(rows) == 0 {
			continue
		}
		form.Rows = rows
		snapshots = append(snapshots, buildPemeriksaanSnapshot(ctxData, settings, form, poNumber, logos, documentDate, total))
	}
	if len(snapshots) == 0 {
		return nil, DP3Context{}, ErrInvalidInput
	}
	return snapshots, ctxData, nil
}

func (s *PemeriksaanService) closingRows(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ClosingKabupatenRow, int, error) {
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
