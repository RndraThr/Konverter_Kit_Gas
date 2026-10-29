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

const dp3Folder = "1. DP3"

type dp3Repository interface {
	GetDP3Context(context.Context, string, auth.RegencyScope) (DP3Context, error)
	ListDP3Recipients(context.Context, string, string, auth.RegencyScope) ([]DP3Recipient, error)
	ListDailyRecapDates(context.Context, string, auth.RegencyScope) ([]dailyRecapDateRow, error)
	ListActiveAggregatesForType(context.Context, string, string, auth.RegencyScope) ([]AggregateDocument, error)
	ListActiveLogos(context.Context, string) ([]LogoSnapshot, error)
	GetScheduleSettings(context.Context, string, auth.RegencyScope) (ScheduleSettings, error)
	GetActiveAggregate(context.Context, string, string, string, auth.RegencyScope) (AggregateDocument, error)
	NextAggregateVersion(context.Context, string, string, string) (int, error)
	ActivateAggregate(context.Context, auth.Principal, AggregateActivation, auth.ClientMeta) (AggregateActivationResult, error)
	RecordAggregateCleanupFailure(context.Context, string, string) error
	GetAggregateByID(context.Context, string, auth.RegencyScope) (AggregateDocument, error)
	ListAggregates(context.Context, string, string, string, auth.RegencyScope) ([]AggregateDocument, error)
}

type DP3Service struct {
	repository dp3Repository
	storage    media.Storage
	location   *time.Location
}

func NewDP3Service(repository dp3Repository, storage media.Storage, location *time.Location) *DP3Service {
	if location == nil {
		location = time.UTC
	}
	return &DP3Service{repository: repository, storage: storage, location: location}
}

func (s *DP3Service) Summary(ctx context.Context, scheduleID string, scope auth.RegencyScope) (DP3Summary, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	_, recipients, err := s.load(ctx, scheduleID, "", scope, false)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return DP3Summary{}, err
		}
		return DP3Summary{ValidationStatus: dp3ValidationStatus(err)}, nil
	}
	// Status mengikuti syarat finalisasi; preview tetap bisa dibuat selama ada penerima.
	status := dp3ValidationStatus(validateDP3Recipients(recipients))
	if status == "ready" {
		settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
		if err != nil {
			return DP3Summary{}, err
		}
		status = dp3ValidationStatus(validateDP3Settings(settings))
	}
	numbered, unmounted := 0, 0
	for _, recipient := range recipients {
		if recipient.Mounted() {
			numbered++
		} else {
			unmounted++
		}
	}
	return DP3Summary{TotalRecipients: len(recipients), NumberedRecipients: numbered, UnmountedRecipients: unmounted, ValidationStatus: status}, nil
}

// Recipients mengembalikan penerima DP3 satu hari distribusi (localDate
// kosong: semua hari), bernomor urut berjalan.
func (s *DP3Service) Recipients(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]DP3Recipient, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if _, err := s.repository.GetDP3Context(ctx, scheduleID, scope); err != nil {
		return nil, err
	}
	return s.repository.ListDP3Recipients(ctx, scheduleID, localDate, scope)
}

// Dates mengembalikan hari distribusi jadwal beserta status dan dokumen DP3
// aktifnya, seperti Rekap Harian: satu DP3 per hari distribusi.
func (s *DP3Service) Dates(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]DailyRecapDate, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	contextErr := validateDP3Context(ctxData)
	settings, settingsErr := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if settingsErr != nil {
		contextErr = settingsErr
	} else if contextErr == nil {
		contextErr = validateDP3Settings(settings)
	}
	dateRows, err := s.repository.ListDailyRecapDates(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	activeDocs, err := s.repository.ListActiveAggregatesForType(ctx, scheduleID, AggregateDocumentDP3, scope)
	if err != nil {
		return nil, err
	}
	docByDate := map[string]AggregateDocument{}
	for i := range activeDocs {
		docByDate[activeDocs[i].DocumentDate] = activeDocs[i]
	}
	dates := make([]DailyRecapDate, 0, len(dateRows))
	for _, row := range dateRows {
		validationErr := contextErr
		if validationErr == nil {
			recipients, recipientsErr := s.repository.ListDP3Recipients(ctx, scheduleID, row.LocalDate, scope)
			if recipientsErr != nil {
				return nil, recipientsErr
			}
			validationErr = validateDP3Recipients(recipients)
		}
		date := DailyRecapDate{LocalDate: row.LocalDate, RecipientCount: row.RecipientCount, ValidationStatus: dp3ValidationStatus(validationErr), HandoverLocation: row.HandoverLocations}
		if doc, ok := docByDate[row.LocalDate]; ok {
			date.Document = &doc
		}
		dates = append(dates, date)
	}
	return dates, nil
}

func (s *DP3Service) Preview(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) (AggregatePreview, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	// Preview tidak menuntut pengaturan dan data penerima lengkap; isian kosong
	// dibiarkan kosong. Finalisasi tetap memvalidasi semuanya.
	ctxData, recipients, err := s.load(ctx, scheduleID, documentDate, scope, false)
	if err != nil {
		return AggregatePreview{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return AggregatePreview{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot, err := buildDP3Snapshot(ctxData, settings, logos, documentDate, recipients)
	if err != nil {
		return AggregatePreview{}, err
	}
	marks, markImages, err := loadSignatoryMarks(ctx, s.repository, ctxData.ScheduleID, "dp3")
	if err != nil {
		return AggregatePreview{}, err
	}
	snapshot.SignatureMarks = marks
	wet, wetImages, err := loadWetSignature(ctx, s.repository, ctxData.ScheduleID, "dp3", documentDate, snapshot)
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
	if err := addRecipientSignatures(ctx, s.repository, logoBytes, dp3SlotIDs(recipients)); err != nil {
		return AggregatePreview{}, err
	}
	rendered, err := RenderDP3(snapshot, logoBytes)
	if err != nil {
		return AggregatePreview{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	return AggregatePreview{PDF: rendered.PDF, Filename: formatDP3Filename(ctxData.RegencyName, documentDate, 1), RecipientCount: len(recipients), PageCount: rendered.PageCount, Checksum: hex.EncodeToString(hash[:])}, nil
}

func (s *DP3Service) Finalize(ctx context.Context, actor auth.Principal, scheduleID, documentDate string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	documentDate = strings.TrimSpace(documentDate)
	ctxData, recipients, err := s.load(ctx, scheduleID, documentDate, scope, true)
	if err != nil {
		return AggregateDocument{}, err
	}
	settings, err := s.repository.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return AggregateDocument{}, err
	}
	if err := validateDP3Settings(settings); err != nil {
		return AggregateDocument{}, err
	}
	logos, err := s.repository.ListActiveLogos(ctx, ctxData.ProgramID)
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot, err := buildDP3Snapshot(ctxData, settings, logos, documentDate, recipients)
	if err != nil {
		return AggregateDocument{}, err
	}
	marks, markImages, err := loadSignatoryMarks(ctx, s.repository, ctxData.ScheduleID, "dp3")
	if err != nil {
		return AggregateDocument{}, err
	}
	snapshot.SignatureMarks = marks
	wet, wetImages, err := loadWetSignature(ctx, s.repository, ctxData.ScheduleID, "dp3", documentDate, snapshot)
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
	if err := addRecipientSignatures(ctx, s.repository, logoBytes, dp3SlotIDs(recipients)); err != nil {
		return AggregateDocument{}, err
	}
	rendered, err := RenderDP3(snapshot, logoBytes)
	if err != nil {
		return AggregateDocument{}, err
	}
	hash := sha256.Sum256(rendered.PDF)
	checksum := hex.EncodeToString(hash[:])

	active, activeErr := s.repository.GetActiveAggregate(ctx, scheduleID, AggregateDocumentDP3, documentDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return AggregateDocument{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
	}

	version, err := s.repository.NextAggregateVersion(ctx, scheduleID, AggregateDocumentDP3, documentDate)
	if err != nil {
		return AggregateDocument{}, err
	}
	filename := formatDP3Filename(ctxData.RegencyName, documentDate, version)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: ctxData.ProgramType, ZoneName: ctxData.ZoneName, RegencyName: ctxData.RegencyName,
		Category: media.FolderBA, Child: dp3Folder,
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
		DocumentType: AggregateDocumentDP3, DocumentDate: documentDate, Filename: filename,
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

func (s *DP3Service) Documents(ctx context.Context, scheduleID, documentDate string, scope auth.RegencyScope) ([]AggregateDocument, error) {
	return s.repository.ListAggregates(ctx, strings.TrimSpace(scheduleID), AggregateDocumentDP3, strings.TrimSpace(documentDate), scope)
}

func (s *DP3Service) Open(ctx context.Context, id string, scope auth.RegencyScope) (AggregateContent, error) {
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

// load melakukan validasi konteks + recipients yang dipakai bersama preview/finalize/summary.
// strict=false hanya mensyaratkan ada penerima (untuk preview).
func (s *DP3Service) load(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope, strict bool) (DP3Context, []DP3Recipient, error) {
	if scheduleID == "" {
		return DP3Context{}, nil, ErrInvalidInput
	}
	ctxData, err := s.repository.GetDP3Context(ctx, scheduleID, scope)
	if err != nil {
		return DP3Context{}, nil, err
	}
	if err := validateDP3Context(ctxData); err != nil {
		return DP3Context{}, nil, err
	}
	recipients, err := s.repository.ListDP3Recipients(ctx, scheduleID, localDate, scope)
	if err != nil {
		return DP3Context{}, nil, err
	}
	if !strict && len(recipients) == 0 {
		return DP3Context{}, nil, ErrAggregateNoRecipients
	}
	if strict {
		if err := validateDP3Recipients(recipients); err != nil {
			return DP3Context{}, nil, err
		}
	}
	return ctxData, recipients, nil
}

func dp3SlotIDs(recipients []DP3Recipient) []string {
	ids := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		ids = append(ids, recipient.SlotID)
	}
	return ids
}
