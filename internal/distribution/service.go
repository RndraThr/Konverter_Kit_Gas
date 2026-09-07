package distribution

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"konkit/internal/auth"
	"konkit/internal/media"
)

var onlyDigits = regexp.MustCompile(`^[0-9]+$`)
var stripNonDigits = regexp.MustCompile(`[^0-9]+`)

type repository interface {
	Search(context.Context, string, string, int, auth.RegencyScope) ([]SearchRecord, error)
	GetWorkspace(context.Context, string, auth.RegencyScope) (RecipientWorkspace, error)
	SaveDraft(context.Context, auth.Principal, string, DraftInput, auth.ClientMeta) (RecipientWorkspace, error)
}

type mediaRepository interface {
	GetMediaSlot(context.Context, string) (MediaSlot, error)
	SaveMedia(context.Context, auth.Principal, MediaFileInput, auth.ClientMeta) (MediaFile, error)
	GetMedia(context.Context, string) (MediaFile, error)
	DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta) (MediaFile, error)
	RestoreMedia(context.Context, string) error
}

type completionRepository interface {
	Complete(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (DistributionRecord, error)
}

type Service struct {
	repository      repository
	mediaRepository mediaRepository
	completion      completionRepository
	storage         media.Storage
}

func NewService(repository repository, storage ...media.Storage) *Service {
	service := &Service{repository: repository}
	service.mediaRepository, _ = repository.(mediaRepository)
	service.completion, _ = repository.(completionRepository)
	if len(storage) > 0 {
		service.storage = storage[0]
	}
	return service
}

func (s *Service) Complete(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionRecord, error) {
	allocationID = strings.TrimSpace(allocationID)
	if allocationID == "" {
		return DistributionRecord{}, ErrAllocationNotFound
	}
	if s.completion == nil {
		return DistributionRecord{}, errors.New("distribution completion is unavailable")
	}
	return s.completion.Complete(ctx, actor, allocationID, meta, scope)
}

func (s *Service) Search(ctx context.Context, scheduleID, query string, limit int, scope auth.RegencyScope) ([]SearchResult, error) {
	scheduleID, query = strings.TrimSpace(scheduleID), strings.TrimSpace(query)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	if query == "" {
		return nil, ErrQueryRequired
	}
	if len([]rune(query)) < 2 && !onlyDigits.MatchString(query) {
		return nil, ErrQueryTooShort
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	records, err := s.repository.Search(ctx, scheduleID, query, limit, scope)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(records))
	for _, record := range records {
		results = append(results, SearchResult{
			AllocationID: record.AllocationID, DistributionNumber: record.DistributionNumber,
			FullName: record.FullName, MaskedNIK: maskNIK(record.NIK), Location: record.Location,
			ProgramType: record.ProgramType, Eligibility: record.Eligibility,
			AllocationStatus: record.AllocationStatus, Documentation: record.Documentation,
		})
	}
	return results, nil
}

func (s *Service) GetWorkspace(ctx context.Context, allocationID string, scope auth.RegencyScope) (RecipientWorkspace, error) {
	if strings.TrimSpace(allocationID) == "" {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	return s.repository.GetWorkspace(ctx, strings.TrimSpace(allocationID), scope)
}

func (s *Service) SaveDraft(ctx context.Context, actor auth.Principal, allocationID string, input DraftInput, meta auth.ClientMeta, scope auth.RegencyScope) (RecipientWorkspace, error) {
	current, err := s.GetWorkspace(ctx, allocationID, scope)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	input.NIK = stripNonDigits.ReplaceAllString(input.NIK, "")
	input.Address = strings.TrimSpace(input.Address)
	input.Village = strings.TrimSpace(input.Village)
	input.District = strings.TrimSpace(input.District)
	input.PhoneNumber = stripNonDigits.ReplaceAllString(input.PhoneNumber, "")
	input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)
	input.IdentityChangeReason = strings.TrimSpace(input.IdentityChangeReason)
	input.MachineOptionCode = strings.TrimSpace(input.MachineOptionCode)
	input.MachineSerialNumber = strings.TrimSpace(input.MachineSerialNumber)
	input.HoseOptionCode = strings.TrimSpace(input.HoseOptionCode)
	input.HoseSerialNumber = strings.TrimSpace(input.HoseSerialNumber)
	input.ConverterSerialNumber = strings.TrimSpace(input.ConverterSerialNumber)
	if input.NIK != "" && len(input.NIK) != 16 {
		return RecipientWorkspace{}, ErrNIKInvalid
	}
	if input.NIK != current.NIK && input.IdentityChangeReason == "" {
		return RecipientWorkspace{}, ErrIdentityChangeReasonRequired
	}
	return s.repository.SaveDraft(ctx, actor, strings.TrimSpace(allocationID), input, meta)
}

func maskNIK(value string) string {
	value = stripNonDigits.ReplaceAllString(value, "")
	if len(value) < 8 {
		return ""
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func normalizeIdentifier(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToUpper(character)
		}
		return -1
	}, value)
}

const maxMediaBytes = 10 << 20

func (s *Service) UploadMedia(ctx context.Context, actor auth.Principal, input UploadMediaInput, meta auth.ClientMeta) (MediaFile, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaFile{}, ErrMediaUnavailable
	}
	if len(input.Data) == 0 {
		return MediaFile{}, ErrMediaTypeInvalid
	}
	if len(input.Data) > maxMediaBytes {
		return MediaFile{}, ErrMediaTooLarge
	}
	slot, err := s.mediaRepository.GetMediaSlot(ctx, strings.TrimSpace(input.SlotID))
	if err != nil {
		return MediaFile{}, err
	}
	input.Source = strings.TrimSpace(input.Source)
	if (slot.InputSource != "both" && slot.InputSource != input.Source) || (input.Source != "camera" && input.Source != "gallery") {
		return MediaFile{}, ErrMediaSourceInvalid
	}
	if slot.AcceptedFiles >= slot.MaxFiles {
		return MediaFile{}, ErrMediaLimitReached
	}
	if slot.RequireLocation && (input.Latitude == nil || input.Longitude == nil) {
		return MediaFile{}, ErrMediaLocationRequired
	}
	if slot.RequireCapturedAt && input.CapturedAt == nil {
		return MediaFile{}, ErrMediaCapturedAtRequired
	}
	mimeType := http.DetectContentType(input.Data[:min(len(input.Data), 512)])
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		return MediaFile{}, ErrMediaTypeInvalid
	}
	key, err := newStorageKey()
	if err != nil {
		return MediaFile{}, err
	}
	size, checksum, err := s.storage.Put(ctx, key, bytes.NewReader(input.Data))
	if err != nil {
		return MediaFile{}, err
	}
	stored, err := s.mediaRepository.SaveMedia(ctx, actor, MediaFileInput{SlotID: slot.ID, StorageKey: key, OriginalFilename: strings.TrimSpace(input.OriginalFilename), MimeType: mimeType, Checksum: checksum, Source: input.Source, ByteSize: size, CapturedAt: input.CapturedAt, Latitude: input.Latitude, Longitude: input.Longitude}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return MediaFile{}, err
	}
	stored.ContentURL = "/api/v1/distribution/media/" + stored.ID + "/content"
	return stored, nil
}

func (s *Service) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta) error {
	if s.storage == nil || s.mediaRepository == nil {
		return ErrMediaUnavailable
	}
	item, err := s.mediaRepository.DeleteMedia(ctx, actor, strings.TrimSpace(mediaID), meta)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, item.StorageKey); err != nil {
		_ = s.mediaRepository.RestoreMedia(context.Background(), item.ID)
		return err
	}
	return nil
}

func (s *Service) OpenMedia(ctx context.Context, mediaID string) (MediaContent, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaContent{}, ErrMediaUnavailable
	}
	item, err := s.mediaRepository.GetMedia(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return MediaContent{}, err
	}
	reader, err := s.storage.Open(ctx, item.StorageKey)
	if err != nil {
		return MediaContent{}, err
	}
	return MediaContent{Reader: reader, MimeType: item.MimeType, Filename: item.OriginalFilename}, nil
}

func newStorageKey() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", fmt.Errorf("generate media key: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
