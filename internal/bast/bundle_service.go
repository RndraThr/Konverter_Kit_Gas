package bast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
}

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

func (s *BundleService) ListDates(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]DateSummary, error) {
	return s.core.ListDates(ctx, programID, regencyID, scope)
}

func (s *BundleService) ListRecipients(ctx context.Context, programID, regencyID, localDate string, scope auth.RegencyScope) ([]RecipientDocument, error) {
	return s.core.ListRecipients(ctx, programID, regencyID, localDate, scope)
}

func (s *BundleService) PreviewBundle(ctx context.Context, request BundleRequest, scope auth.RegencyScope) (BundlePreview, error) {
	request = normalizedBundleRequest(request)
	documents, _, _, err := s.prepareDocuments(ctx, auth.Principal{}, request, scope, auth.ClientMeta{}, false)
	if err != nil {
		return BundlePreview{}, err
	}
	rendered, checksum, err := s.render(ctx, request.LocalDate, documents)
	if err != nil {
		return BundlePreview{}, err
	}
	return BundlePreview{PDF: rendered.PDF, Filename: rendered.Filename, RecipientCount: len(documents), PageCount: rendered.PageCount, Checksum: checksum}, nil
}

func (s *BundleService) FinalizeBundle(ctx context.Context, actor auth.Principal, request BundleRequest, scope auth.RegencyScope, meta auth.ClientMeta) (DailyBundle, error) {
	request = normalizedBundleRequest(request)
	documents, documentIDs, sourceContext, err := s.prepareDocuments(ctx, actor, request, scope, meta, true)
	if err != nil {
		return DailyBundle{}, err
	}
	rendered, checksum, err := s.render(ctx, request.LocalDate, documents)
	if err != nil {
		return DailyBundle{}, err
	}

	active, activeErr := s.repository.GetActiveBundle(ctx, request.ProgramID, request.RegencyID, request.LocalDate, scope)
	if activeErr != nil && !errors.Is(activeErr, ErrNotFound) {
		return DailyBundle{}, activeErr
	}
	if activeErr == nil && active.Checksum == checksum {
		return active, nil
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
	storageKey, _, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), rendered.Filename, folderPath, bytes.NewReader(rendered.PDF))
	if err != nil {
		return DailyBundle{}, err
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
		ProfileVersionID: documents[0].Snapshot.Profile.VersionID,
		Filename:         rendered.Filename, PageCount: rendered.PageCount, Checksum: checksum,
		StorageKey: storageKey, ExpectedActiveID: expectedActiveID, Items: items,
	}
	activated, err := s.repository.ActivateBundle(ctx, actor, activation, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return DailyBundle{}, err
	}
	if activated.Unchanged {
		_ = s.storage.Delete(context.Background(), storageKey)
		return activated.Bundle, nil
	}
	if activated.OldStorageKey != "" && activated.OldStorageKey != storageKey {
		if deleteErr := s.storage.Delete(ctx, activated.OldStorageKey); deleteErr != nil {
			message := fmt.Sprintf("cleanup old storage key %s: %v", activated.OldStorageKey, deleteErr)
			_ = s.repository.RecordCleanupFailure(context.Background(), activated.Bundle.ID, message)
		}
	}
	return activated.Bundle, nil
}

func normalizedBundleRequest(request BundleRequest) BundleRequest {
	request.ProgramID = strings.TrimSpace(request.ProgramID)
	request.RegencyID = strings.TrimSpace(request.RegencyID)
	request.LocalDate = strings.TrimSpace(request.LocalDate)
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
	recipients, err := s.core.ListRecipients(ctx, request.ProgramID, request.RegencyID, request.LocalDate, scope)
	if err != nil {
		return nil, nil, SourceContext{}, err
	}
	if len(recipients) == 0 {
		return nil, nil, SourceContext{}, ErrNoRecipients
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
		snapshot, err := BuildSnapshot(source)
		if err != nil {
			return nil, nil, SourceContext{}, err
		}
		recipients[i].Snapshot = snapshot
	}
	return recipients, documentIDs, sourceContext, nil
}

func (s *BundleService) render(ctx context.Context, localDate string, documents []RecipientDocument) (RenderedBundle, string, error) {
	logoBytes := map[string][]byte{}
	for _, document := range documents {
		for _, logo := range document.Snapshot.Profile.Logos {
			if _, exists := logoBytes[logo.AssetID]; exists {
				continue
			}
			reader, err := s.storage.Open(ctx, logo.StorageKey)
			if err != nil {
				return RenderedBundle{}, "", err
			}
			data, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil {
				return RenderedBundle{}, "", readErr
			}
			if closeErr != nil {
				return RenderedBundle{}, "", closeErr
			}
			logoBytes[logo.AssetID] = data
		}
	}
	rendered, err := RenderPetaniBundle(BundleRenderInput{LocalDate: localDate, Documents: documents, LogoBytes: logoBytes})
	if err != nil {
		return RenderedBundle{}, "", err
	}
	hash := sha256.Sum256(rendered.PDF)
	return rendered, hex.EncodeToString(hash[:]), nil
}
