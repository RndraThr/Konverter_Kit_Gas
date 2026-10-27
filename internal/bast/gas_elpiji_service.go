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

const gasElpijiFolder = "13. GAS ELPIJI"

type gasElpijiRepository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
	ListActiveAggregatesWithSnapshot(context.Context, string, string, auth.RegencyScope) ([]AggregateWithSnapshot, error)
	RetireAggregate(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (AggregateDocument, error)
}

// GasElpijiService menerbitkan BA Gas Elpiji per penerimaan LPG dan BA
// gabungan seluruh penerimaan satu jadwal kabupaten.
type GasElpijiService struct {
	repository gasElpijiRepository
	storage    media.Storage
}

func NewGasElpijiService(repository gasElpijiRepository, storage media.Storage) *GasElpijiService {
	return &GasElpijiService{repository: repository, storage: storage}
}

// Summary mengembalikan penerimaan aktif (urut tanggal) dan status gabungan.
func (s *GasElpijiService) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (GasElpijiSummary, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return GasElpijiSummary{}, err
	}
	receipts, err := s.receipts(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return GasElpijiSummary{}, err
	}
	summary := GasElpijiSummary{Receipts: receipts}
	for _, receipt := range receipts {
		summary.TotalCylinders += receipt.Input.CylinderCount
	}
	merged, err := s.repository.ListActiveAggregatesWithSnapshot(ctx, ctxData.ScheduleID, AggregateDocumentGasElpijiMerged, scope)
	if err != nil {
		return GasElpijiSummary{}, err
	}
	if len(merged) > 0 {
		doc := merged[len(merged)-1].Document
		summary.Merged = &doc
		var snapshot GasElpijiMergedSnapshot
		if json.Unmarshal(merged[len(merged)-1].Snapshot, &snapshot) == nil {
			summary.MergedCurrent = mergedMatches(snapshot, receipts)
		}
	}
	return summary, nil
}

func mergedMatches(snapshot GasElpijiMergedSnapshot, receipts []GasElpijiReceipt) bool {
	if len(snapshot.Receipts) != len(receipts) {
		return false
	}
	for i, receipt := range receipts {
		if snapshot.Receipts[i].DocumentID != receipt.Document.ID || snapshot.Receipts[i].Checksum != receipt.Document.Checksum {
			return false
		}
	}
	return true
}

// Preview merender satu penerimaan dari isian yang belum disimpan.
func (s *GasElpijiService) Preview(ctx context.Context, input GasElpijiInput, scope auth.RegencyScope) (AggregatePreview, error) {
	snapshot, ctxData, err := s.snapshot(ctx, input, false, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := s.render(ctx, []GasElpijiSnapshot{snapshot})
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatGasElpijiFilename(ctxData.RegencyName, snapshot.DocumentDate, 1), RecipientCount: snapshot.CylinderCount, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

// Finalize menyimpan satu penerimaan sebagai dokumen tanggalnya (versi naik
// bila isian tanggal itu diubah).
func (s *GasElpijiService) Finalize(ctx context.Context, actor auth.Principal, input GasElpijiInput, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	snapshot, ctxData, err := s.snapshot(ctx, input, true, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := s.render(ctx, []GasElpijiSnapshot{snapshot})
	if err != nil {
		return AggregateDocument{}, err
	}
	return s.store(ctx, actor, ctxData, AggregateDocumentGasElpiji, snapshot.DocumentDate, snapshot, rendered, snapshot.CylinderCount, scope, meta,
		func(version int) string { return formatGasElpijiFilename(ctxData.RegencyName, snapshot.DocumentDate, version) })
}

// MergedPreview menggabungkan semua penerimaan aktif menjadi satu PDF.
func (s *GasElpijiService) MergedPreview(ctx context.Context, scheduleID string, scope auth.RegencyScope) (AggregatePreview, error) {
	merged, ctxData, err := s.mergedSnapshot(ctx, scheduleID, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := s.render(ctx, merged.pages())
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatGasElpijiMergedFilename(ctxData.RegencyName, 1), RecipientCount: merged.TotalCylinders, PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

// FinalizeMerged menyimpan BA gabungan. Kuncinya tanggal mulai jadwal agar
// selalu satu dokumen aktif; penggabungan ulang menaikkan versinya.
func (s *GasElpijiService) FinalizeMerged(ctx context.Context, actor auth.Principal, scheduleID string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	merged, ctxData, err := s.mergedSnapshot(ctx, scheduleID, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := s.render(ctx, merged.pages())
	if err != nil {
		return AggregateDocument{}, err
	}
	return s.store(ctx, actor, ctxData, AggregateDocumentGasElpijiMerged, ctxData.StartDate, merged, rendered, merged.TotalCylinders, scope, meta,
		func(version int) string { return formatGasElpijiMergedFilename(ctxData.RegencyName, version) })
}

// Documents mengembalikan riwayat versi penerimaan satu tanggal.
func (s *GasElpijiService) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentGasElpiji, strings.TrimSpace(documentDate), scope)
}

// MergedDocuments mengembalikan riwayat versi BA gabungan.
func (s *GasElpijiService) MergedDocuments(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return nil, err
	}
	return s.repository.ListAggregates(ctx, ctxData.ScheduleID, AggregateDocumentGasElpijiMerged, ctxData.StartDate, scope)
}

// Cancel membatalkan penerimaan yang salah input; berkasnya dihapus dari
// penyimpanan dan penerimaan itu tidak lagi ikut digabung.
func (s *GasElpijiService) Cancel(ctx context.Context, actor auth.Principal, id string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	doc, err := s.repository.RetireAggregate(ctx, actor, strings.TrimSpace(id), AggregateDocumentGasElpiji, scope, meta)
	if err != nil {
		return err
	}
	if doc.StorageKey != "" {
		if deleteErr := s.storage.Delete(ctx, doc.StorageKey); deleteErr != nil {
			_ = s.repository.RecordAggregateCleanupFailure(context.Background(), doc.ID, fmt.Sprintf("cleanup cancelled storage key %s: %v", doc.StorageKey, deleteErr))
		}
	}
	return nil
}

func (s *GasElpijiService) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
	doc, err := s.repository.GetAggregateByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return AggregateContent{}, err
	}
	if doc.DocumentType != AggregateDocumentGasElpiji && doc.DocumentType != AggregateDocumentGasElpijiMerged {
		return AggregateContent{}, ErrNotFound
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return AggregateContent{}, err
	}
	return AggregateContent{Reader: reader, Filename: doc.Filename}, nil
}

func (s *GasElpijiService) receipts(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]GasElpijiReceipt, error) {
	rows, err := s.repository.ListActiveAggregatesWithSnapshot(ctx, scheduleID, AggregateDocumentGasElpiji, scope)
	if err != nil {
		return nil, err
	}
	receipts := make([]GasElpijiReceipt, 0, len(rows))
	for _, row := range rows {
		var snapshot GasElpijiSnapshot
		if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return nil, fmt.Errorf("decode gas elpiji snapshot %s: %w", row.Document.ID, err)
		}
		receipts = append(receipts, GasElpijiReceipt{Document: row.Document, Input: snapshot.input(scheduleID), snapshot: snapshot})
	}
	return receipts, nil
}

func (s *GasElpijiService) snapshot(ctx context.Context, input GasElpijiInput, requireComplete bool, scope auth.RegencyScope) (GasElpijiSnapshot, DP3Context, error) {
	if err := input.normalize(); err != nil {
		return GasElpijiSnapshot{}, DP3Context{}, err
	}
	if requireComplete && !input.complete() {
		return GasElpijiSnapshot{}, DP3Context{}, ErrGasElpijiIncomplete
	}
	ctxData, err := s.repository.GetDP3Context(ctx, input.ScheduleID, scope)
	if err != nil {
		return GasElpijiSnapshot{}, DP3Context{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return GasElpijiSnapshot{}, DP3Context{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return GasElpijiSnapshot{}, DP3Context{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return GasElpijiSnapshot{}, DP3Context{}, err
	}
	if len(logos) == 0 {
		return GasElpijiSnapshot{}, DP3Context{}, ErrBrandingNotConfigured
	}
	return buildGasElpijiSnapshot(ctxData, settings, logos, input), ctxData, nil
}

func (s *GasElpijiService) mergedSnapshot(ctx context.Context, scheduleID string, scope auth.RegencyScope) (GasElpijiMergedSnapshot, DP3Context, error) {
	ctxData, err := s.repository.GetDP3Context(ctx, strings.TrimSpace(scheduleID), scope)
	if err != nil {
		return GasElpijiMergedSnapshot{}, DP3Context{}, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return GasElpijiMergedSnapshot{}, DP3Context{}, err
	}
	if !validRequiredDate(ctxData.StartDate) {
		return GasElpijiMergedSnapshot{}, DP3Context{}, ErrInvalidInput
	}
	receipts, err := s.receipts(ctx, ctxData.ScheduleID, scope)
	if err != nil {
		return GasElpijiMergedSnapshot{}, DP3Context{}, err
	}
	if len(receipts) == 0 {
		return GasElpijiMergedSnapshot{}, DP3Context{}, ErrGasElpijiNoReceipts
	}
	merged := GasElpijiMergedSnapshot{DocumentType: AggregateDocumentGasElpijiMerged}
	for _, receipt := range receipts {
		merged.Receipts = append(merged.Receipts, GasElpijiMergedReceipt{DocumentID: receipt.Document.ID, Checksum: receipt.Document.Checksum, Snapshot: receipt.snapshot})
		merged.TotalCylinders += receipt.snapshot.CylinderCount
	}
	return merged, ctxData, nil
}

func (m GasElpijiMergedSnapshot) pages() []GasElpijiSnapshot {
	pages := make([]GasElpijiSnapshot, len(m.Receipts))
	for i, receipt := range m.Receipts {
		pages[i] = receipt.Snapshot
	}
	return pages
}

func (s *GasElpijiService) render(ctx context.Context, snapshots []GasElpijiSnapshot) (RenderedAggregate, error) {
	var logos []LogoSnapshot
	for _, snapshot := range snapshots {
		logos = append(logos, snapshot.Logos...)
	}
	logoBytes, err := loadLogoBytes(ctx, s.storage, logos)
	if err != nil {
		return RenderedAggregate{}, err
	}
	return RenderGasElpiji(snapshots, logoBytes)
}

// store mengunggah PDF ke folder BA Gas Elpiji lalu mengaktifkannya; isi yang
// sama dengan dokumen aktif tidak membuat versi baru.
func (s *GasElpijiService) store(ctx context.Context, actor auth.Principal, ctxData DP3Context, documentType, documentDate string, snapshot any, rendered RenderedAggregate, count int, scope auth.RegencyScope, meta auth.ClientMeta, filenameFor func(int) string) (AggregateDocument, error) {
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])
	active, activeErr := s.repository.GetActiveAggregate(ctx, ctxData.ScheduleID, documentType, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}
	version, err := s.repository.NextAggregateVersion(ctx, ctxData.ScheduleID, documentType, documentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return AggregateDocument{}, err
	}
	filename := filenameFor(version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: gasElpijiFolder,
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
		ScheduleID: ctxData.ScheduleID, ProgramID: ctxData.ProgramID, RegencyID: ctxData.RegencyID,
		DocumentType: documentType, DocumentDate: documentDate, Filename: filename,
		RecipientCount: count, PageCount: rendered.PageCount, Checksum: checksum,
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
