package bast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
)

const individualBAFolder = "2. BA PERORANGAN"

type bundleRepository interface {
	repository
	GetActiveBundle(context.Context, string, string, string, auth.RegencyScope) (DailyBundle, error)
	ActivateBundle(context.Context, auth.Principal, BundleActivation, auth.ClientMeta) (BundleActivationResult, error)
	RecordCleanupFailure(context.Context, string, string) error
	GetActiveBundleByID(context.Context, string, auth.RegencyScope) (DailyBundle, error)
	RetireDateBundles(context.Context, string, string, string, auth.RegencyScope) ([]string, error)
}

// Sync statuses of a BA Perorangan date (DateSummary.SyncStatus).
const (
	BundleSynced     = "synced"
	BundleOutOfSync  = "out_of_sync"
	BundleNotCreated = "not_created"
)

type BundleService struct {
	repository bundleRepository
	storage    media.Storage
	core       *Service
}

func NewBundleService(repository bundleRepository, storage media.Storage, location *time.Location) *BundleService {
	return &BundleService{repository: repository, storage: storage, core: NewService(repository, location)}
}

func (s *BundleService) LockRegencyTotal(ctx context.Context, actor auth.Principal, programID, regencyID string, scope auth.RegencyScope, meta auth.ClientMeta) (LockResult, error) {
	return s.core.LockRegencyTotal(ctx, actor, programID, regencyID, scope, meta)
}

// ListDates lists the distribution dates with their Drive sync status. A date is synced when
// its PDF was built from exactly the current content; any change since (recipient data, numbers,
// signatories, marks, logos, recipient signature) makes it out of sync until the next sync. A
// date whose recipients all moved away keeps its old PDF listed so a sync can remove it.
func (s *BundleService) ListDates(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]DateSummary, error) {
	dates, err := s.core.ListDates(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	bundles, err := s.repository.ListActiveBundles(ctx, programID, regencyID, scope)
	if err != nil {
		return nil, err
	}
	listed := make(map[string]bool, len(dates))
	for _, date := range dates {
		listed[date.LocalDate] = true
	}
	for i := range bundles {
		if !listed[bundles[i].LocalDate] {
			dates = append(dates, DateSummary{LocalDate: bundles[i].LocalDate, ValidationStatus: "ready", Bundle: &bundles[i]})
		}
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].LocalDate > dates[j].LocalDate })

	var sources *fingerprintSources
	for i := range dates {
		date := &dates[i]
		switch {
		case date.ValidationStatus != "ready":
			continue
		case date.Bundle == nil:
			date.SyncStatus = BundleNotCreated
			continue
		case date.RecipientCount == 0 || date.Bundle.Status != "active" || date.Bundle.SourceFingerprint == "":
			date.SyncStatus = BundleOutOfSync
			continue
		}
		if sources == nil {
			loaded, err := s.loadFingerprintSources(ctx, programID, regencyID, scope)
			if err != nil {
				return nil, err
			}
			sources = &loaded
		}
		fingerprint, err := s.dateFingerprint(ctx, BundleRequest{ProgramID: programID, RegencyID: regencyID, LocalDate: date.LocalDate}, scope, *sources)
		if err != nil {
			// Data that cannot be built (for example an incomplete recipient) cannot be synced
			// either; the date reads as not synced and the sync reports the reason.
			date.SyncStatus = BundleOutOfSync
			continue
		}
		date.SyncStatus = BundleOutOfSync
		if fingerprint == date.Bundle.SourceFingerprint {
			date.SyncStatus = BundleSynced
		}
	}
	return dates, nil
}

func (s *BundleService) ListRecipients(ctx context.Context, programID, regencyID, localDate string, scope auth.RegencyScope) ([]RecipientDocument, error) {
	return s.core.ListRecipients(ctx, programID, regencyID, localDate, scope)
}

func (s *BundleService) PreviewBundle(ctx context.Context, request BundleRequest, scope auth.RegencyScope) (BundlePreview, error) {
	request = normalizedBundleRequest(request)
	if request.SlotID != "" {
		// A slot's BA belongs to the day its distribution was completed, which
		// can differ from the distribution date set in POS Dokumen.
		date, err := s.completedSlotDate(ctx, request, scope)
		if err != nil {
			return BundlePreview{}, err
		}
		request.LocalDate = date
	}
	documents, _, _, err := s.prepareDocuments(ctx, auth.Principal{}, request, scope, auth.ClientMeta{}, false)
	if err != nil {
		return BundlePreview{}, err
	}
	if request.SlotID != "" {
		documents = slices.DeleteFunc(documents, func(d RecipientDocument) bool { return d.DistributionSlotID != request.SlotID })
		if len(documents) == 0 {
			return BundlePreview{}, ErrNotFound
		}
	}
	rendered, checksum, err := s.render(ctx, request.ProgramID, request.RegencyID, request.LocalDate, documents)
	if err != nil {
		return BundlePreview{}, err
	}
	return BundlePreview{PDF: rendered.PDF, Filename: rendered.Filename, RecipientCount: len(documents), PageCount: rendered.PageCount, Checksum: checksum}, nil
}

// FinalizeBundle syncs one date: it writes the current BA Perorangan to the BA folder in Drive
// (replacing the previous PDF) and records the content it was built from, so the date reads as
// synced until the data changes again. An identical PDF is not uploaded again.
func (s *BundleService) FinalizeBundle(ctx context.Context, actor auth.Principal, request BundleRequest, scope auth.RegencyScope, meta auth.ClientMeta) (DailyBundle, error) {
	request = normalizedBundleRequest(request)
	sources, err := s.loadFingerprintSources(ctx, request.ProgramID, request.RegencyID, scope)
	if err != nil {
		return DailyBundle{}, err
	}
	fingerprint, err := s.dateFingerprint(ctx, request, scope, sources)
	if errors.Is(err, ErrNoRecipients) {
		// Every recipient of this date moved to another date or was reopened: the sync removes
		// the date's old PDF instead of writing a new one.
		return DailyBundle{}, s.RetireDate(ctx, request.ProgramID, request.RegencyID, request.LocalDate, scope)
	}
	if err != nil {
		return DailyBundle{}, err
	}
	documents, documentIDs, sourceContext, err := s.prepareDocuments(ctx, actor, request, scope, meta, true)
	if err != nil {
		return DailyBundle{}, err
	}
	rendered, checksum, err := s.render(ctx, request.ProgramID, request.RegencyID, request.LocalDate, documents)
	if err != nil {
		return DailyBundle{}, err
	}

	active, activeErr := s.repository.GetActiveBundle(ctx, request.ProgramID, request.RegencyID, request.LocalDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return DailyBundle{}, activeErr
	}

	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: sourceContext.ProgramType,
		ZoneName:    sourceContext.ZoneName,
		RegencyName: sourceContext.RegencyName,
		Category:    media.FolderBA,
		Child:       individualBAFolder,
	})
	if err != nil {
		return DailyBundle{}, err
	}
	storageKey := ""
	if activeErr != nil || active.Checksum != checksum {
		storageKey, _, _, err = media.PutNamed(ctx, s.storage, uuid.NewString(), rendered.Filename, folderPath, bytes.NewReader(rendered.PDF))
		if err != nil {
			return DailyBundle{}, err
		}
	}

	items := make([]BundleItemActivation, 0, len(rendered.Recipients))
	for _, pages := range rendered.Recipients {
		items = append(items, BundleItemActivation{IndividualDocumentID: documentIDs[pages.SlotNumber], SlotNumber: pages.SlotNumber, PageStart: pages.PageStart, PageEnd: pages.PageEnd})
	}
	expectedActiveID := ""
	if activeErr == nil {
		expectedActiveID = active.ID
	}
	activation := BundleActivation{
		ProgramID: request.ProgramID, RegencyID: request.RegencyID, LocalDate: request.LocalDate,
		Filename: rendered.Filename, PageCount: rendered.PageCount, Checksum: checksum,
		StorageKey: storageKey, ExpectedActiveID: expectedActiveID, Items: items,
		SourceFingerprint: fingerprint,
	}
	activated, err := s.repository.ActivateBundle(ctx, actor, activation, meta)
	if err != nil {
		if storageKey != "" {
			_ = s.storage.Delete(context.Background(), storageKey)
		}
		return DailyBundle{}, err
	}
	oldKeys := append([]string{activated.OldStorageKey}, activated.StaleStorageKeys...)
	if activated.Unchanged {
		// The PDF already in Drive is identical; a concurrent upload of it is discarded.
		if storageKey != "" {
			_ = s.storage.Delete(context.Background(), storageKey)
		}
		oldKeys = activated.StaleStorageKeys
	}
	for _, oldKey := range oldKeys {
		if oldKey == "" || oldKey == storageKey {
			continue
		}
		if deleteErr := s.storage.Delete(ctx, oldKey); deleteErr != nil {
			message := fmt.Sprintf("cleanup old storage key %s: %v", oldKey, deleteErr)
			_ = s.repository.RecordCleanupFailure(context.Background(), activated.Bundle.ID, message)
		}
	}
	return activated.Bundle, nil
}

// RetireDate removes the PDFs of a date that no longer has completed distributions.
func (s *BundleService) RetireDate(ctx context.Context, programID, regencyID, localDate string, scope auth.RegencyScope) error {
	keys, err := s.repository.RetireDateBundles(ctx, programID, regencyID, localDate, scope)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := s.storage.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete retired BA bundle %s: %w", key, err)
		}
	}
	return nil
}

// completedSlotDate is the bundle day of the slot: its distribution date.
func (s *BundleService) completedSlotDate(ctx context.Context, request BundleRequest, scope auth.RegencyScope) (string, error) {
	slots, err := s.repository.ListCompletedSlots(ctx, request.ProgramID, request.RegencyID, scope)
	if err != nil {
		return "", err
	}
	for _, slot := range slots {
		if slot.ID == request.SlotID {
			return slot.Day(s.core.location), nil
		}
	}
	return "", ErrNotFound
}

func normalizedBundleRequest(request BundleRequest) BundleRequest {
	request.ProgramID = strings.TrimSpace(request.ProgramID)
	request.RegencyID = strings.TrimSpace(request.RegencyID)
	request.LocalDate = strings.TrimSpace(request.LocalDate)
	request.SlotID = strings.TrimSpace(request.SlotID)
	return request
}

func (s *BundleService) OpenBundle(ctx context.Context, id string, scope auth.RegencyScope) (BundleContent, error) {
	bundle, err := s.repository.GetActiveBundleByID(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return BundleContent{}, err
	}
	reader, err := s.storage.Open(ctx, bundle.StorageKey)
	if err != nil {
		return BundleContent{}, err
	}
	return BundleContent{Reader: reader, Filename: bundle.Filename}, nil
}

func (s *BundleService) prepareDocuments(ctx context.Context, actor auth.Principal, request BundleRequest, scope auth.RegencyScope, meta auth.ClientMeta, finalize bool) ([]RecipientDocument, map[int]string, SourceContext, error) {
	request.ProgramID = strings.TrimSpace(request.ProgramID)
	request.RegencyID = strings.TrimSpace(request.RegencyID)
	request.LocalDate = strings.TrimSpace(request.LocalDate)
	if request.ProgramID == "" || request.RegencyID == "" {
		return nil, nil, SourceContext{}, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", request.LocalDate); err != nil {
		return nil, nil, SourceContext{}, ErrInvalidInput
	}
	sourceContext, err := s.repository.GetSourceContext(ctx, request.ProgramID, request.RegencyID, scope)
	if err != nil {
		return nil, nil, SourceContext{}, err
	}
	if err := validateContext(sourceContext); err != nil {
		return nil, nil, SourceContext{}, err
	}
	// BA Perorangan tidak menunggu total dikunci: nomor BA memakai total yang
	// dikunci bila ada, selain itu kuota slot kabupaten.
	recipients, err := s.core.listRecipients(ctx, request.ProgramID, request.RegencyID, request.LocalDate, scope, false)
	if err != nil {
		return nil, nil, SourceContext{}, err
	}
	if len(recipients) == 0 {
		return nil, nil, SourceContext{}, ErrNoRecipients
	}
	if finalize {
		for _, recipient := range recipients {
			if recipient.DocumentNumber == "" {
				return nil, nil, SourceContext{}, ErrSlotOutOfRange
			}
		}
	}
	documentIDs := make(map[int]string, len(recipients))
	for i := range recipients {
		if finalize {
			document, err := s.core.FinalizeRecipient(ctx, actor, request.ProgramID, request.RegencyID, recipients[i], scope, meta)
			if err != nil {
				return nil, nil, SourceContext{}, err
			}
			recipients[i].Snapshot = document.Snapshot
			documentIDs[recipients[i].SlotNumber] = document.ID
			continue
		}
		source, err := s.repository.LoadSourceData(ctx, recipients[i], sourceContext, scope)
		if err != nil {
			return nil, nil, SourceContext{}, err
		}
		snapshot, err := BuildPreviewSnapshot(source)
		if err != nil {
			return nil, nil, SourceContext{}, err
		}
		recipients[i].Snapshot = snapshot
	}
	return recipients, documentIDs, sourceContext, nil
}

func (s *BundleService) render(ctx context.Context, programID, regencyID, localDate string, documents []RecipientDocument) (RenderedBundle, string, error) {
	logoBytes := map[string][]byte{}
	scheduleID, err := s.scheduleFor(ctx, programID, regencyID)
	if err != nil {
		return RenderedBundle{}, "", err
	}
	marks, markImages, err := loadSignatoryMarks(ctx, s.repository, scheduleID, "ba-perorangan")
	if err != nil {
		return RenderedBundle{}, "", err
	}
	if signatureLayoutFrom(ctx) != nil && len(documents) > 1 {
		documents = documents[:1]
	}
	for i := range documents {
		documents[i].Snapshot.Signatures.Marks = marks
	}
	mergeImages(logoBytes, markImages)
	for _, document := range documents {
		for _, logo := range document.Snapshot.Render.Logos {
			if _, exists := logoBytes[logo.AssetID]; exists {
				continue
			}
			data, err := readStoredLogo(ctx, s.storage, logo.StorageKey)
			if err != nil {
				return RenderedBundle{}, "", err
			}
			logoBytes[logo.AssetID] = data
		}
	}
	slotIDs := make([]string, 0, len(documents))
	for _, document := range documents {
		slotIDs = append(slotIDs, document.Snapshot.Signatures.ReceiverSignatureKey)
	}
	if err := addRecipientSignatures(ctx, s.repository, logoBytes, slotIDs); err != nil {
		return RenderedBundle{}, "", err
	}
	rendered, err := RenderPetaniBundle(BundleRenderInput{LocalDate: localDate, Documents: documents, LogoBytes: logoBytes})
	if err != nil {
		return RenderedBundle{}, "", err
	}
	hash := sha256.Sum256(rendered.PDF)
	return rendered, hex.EncodeToString(hash[:]), nil
}

// SchedulePreview merender BA Perorangan satu tanggal untuk jadwal (program
// dan kabupaten jadwal), dipakai editor tanda tangan.
func (s *BundleService) SchedulePreview(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error) {
	finder, ok := s.repository.(interface {
		ScheduleProgramRegency(context.Context, string, auth.RegencyScope) (string, string, error)
	})
	if !ok {
		return AggregatePreview{}, ErrInvalidInput
	}
	programID, regencyID, err := finder.ScheduleProgramRegency(ctx, scheduleID, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	preview, err := s.PreviewBundle(ctx, BundleRequest{ProgramID: programID, RegencyID: regencyID, LocalDate: date}, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	return AggregatePreview{PDF: preview.PDF, Filename: preview.Filename, RecipientCount: preview.RecipientCount, PageCount: preview.PageCount}, nil
}

// scheduleFor is the regency schedule whose Pelaksana/Konsultan signatures BA Perorangan uses.
func (s *BundleService) scheduleFor(ctx context.Context, programID, regencyID string) (string, error) {
	finder, ok := s.repository.(interface {
		ScheduleForRegency(context.Context, string, string) (string, error)
	})
	if !ok {
		return "", nil
	}
	return finder.ScheduleForRegency(ctx, programID, regencyID)
}

// fingerprintSources are the regency-wide inputs of every date's content, loaded once per list.
type fingerprintSources struct {
	marks      SignatureMarks
	markImages map[string][]byte
}

func (s *BundleService) loadFingerprintSources(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (fingerprintSources, error) {
	scheduleID, err := s.scheduleFor(ctx, programID, regencyID)
	if err != nil {
		return fingerprintSources{}, err
	}
	marks, images, err := loadSignatoryMarks(ctx, s.repository, scheduleID, "ba-perorangan")
	if err != nil {
		return fingerprintSources{}, err
	}
	return fingerprintSources{marks: marks, markImages: images}, nil
}

// dateFingerprint hashes everything a date's PDF is built from: each recipient's current
// snapshot (identity, number, equipment, components, signatory names, logos), the signatory
// marks with their images, and the recipients' own signatures.
func (s *BundleService) dateFingerprint(ctx context.Context, request BundleRequest, scope auth.RegencyScope, sources fingerprintSources) (string, error) {
	documents, _, _, err := s.prepareDocuments(ctx, auth.Principal{}, request, scope, auth.ClientMeta{}, false)
	if err != nil {
		return "", err
	}
	snapshots := make([]Snapshot, len(documents))
	slotIDs := make([]string, 0, len(documents))
	for i, document := range documents {
		snapshots[i] = document.Snapshot
		slotIDs = append(slotIDs, document.Snapshot.Signatures.ReceiverSignatureKey)
	}
	signatures := map[string][]byte{}
	if err := addRecipientSignatures(ctx, s.repository, signatures, slotIDs); err != nil {
		return "", err
	}
	images := map[string]string{}
	for key, data := range sources.markImages {
		images[key] = checksumHex(data)
	}
	for key, data := range signatures {
		images[key] = checksumHex(data)
	}
	payload, err := json.Marshal(struct {
		Snapshots []Snapshot        `json:"snapshots"`
		Marks     SignatureMarks    `json:"marks"`
		Images    map[string]string `json:"images"`
	}{snapshots, sources.marks, images})
	if err != nil {
		return "", err
	}
	return checksumHex(payload), nil
}

func checksumHex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
