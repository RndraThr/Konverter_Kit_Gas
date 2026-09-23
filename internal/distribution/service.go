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

type mediaRepository interface {
	GetMediaSlot(context.Context, string, auth.RegencyScope) (MediaSlot, error)
	SaveMedia(context.Context, auth.Principal, MediaFileInput, auth.ClientMeta) (MediaFile, error)
	GetMedia(context.Context, string, auth.RegencyScope) (MediaFile, error)
	DeleteMedia(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) (MediaFile, error)
	RestoreMedia(context.Context, string) error
}

type posMesinRepository interface {
	CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error)
}

type posDokumenRepository interface {
	SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error)
	LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
}

type posPenyerahanRepository interface {
	SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error)
	CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error)
}

type Service struct {
	mediaRepository         mediaRepository
	posMesinRepository      posMesinRepository
	posDokumenRepository    posDokumenRepository
	posPenyerahanRepository posPenyerahanRepository
	storage                 media.Storage
}

func NewService(repository any, storage ...media.Storage) *Service {
	service := &Service{}
	service.mediaRepository, _ = repository.(mediaRepository)
	service.posMesinRepository, _ = repository.(posMesinRepository)
	service.posDokumenRepository, _ = repository.(posDokumenRepository)
	service.posPenyerahanRepository, _ = repository.(posPenyerahanRepository)
	if len(storage) > 0 {
		service.storage = storage[0]
	}
	return service
}

func (s *Service) CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	input.MachineOptionCode = strings.TrimSpace(input.MachineOptionCode)
	input.MachineSerialNumber = strings.TrimSpace(input.MachineSerialNumber)
	input.HoseOptionCode = strings.TrimSpace(input.HoseOptionCode)
	input.HoseSerialNumber = strings.TrimSpace(input.HoseSerialNumber)
	input.ConverterSerialNumber = strings.TrimSpace(input.ConverterSerialNumber)
	if s.posMesinRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Mesin is unavailable")
	}
	return s.posMesinRepository.CreateSlot(ctx, actor, input, meta)
}

func (s *Service) SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error) {
	scheduleID, nik = strings.TrimSpace(scheduleID), stripNonDigits.ReplaceAllString(nik, "")
	if scheduleID == "" {
		return CandidateMatch{}, ErrScheduleRequired
	}
	if len(nik) != 16 {
		return CandidateMatch{}, ErrNIKInvalid
	}
	if s.posDokumenRepository == nil {
		return CandidateMatch{}, errors.New("distribution POS Dokumen is unavailable")
	}
	return s.posDokumenRepository.SearchCandidate(ctx, scheduleID, nik, scope)
}

func (s *Service) LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber <= 0 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	input.NIK = stripNonDigits.ReplaceAllString(input.NIK, "")
	if len(input.NIK) != 16 {
		return DistributionSlot{}, ErrNIKInvalid
	}
	input.Address = strings.TrimSpace(input.Address)
	input.Village = strings.TrimSpace(input.Village)
	input.District = strings.TrimSpace(input.District)
	input.PhoneNumber = stripNonDigits.ReplaceAllString(input.PhoneNumber, "")
	input.SectorIdentifier = normalizeIdentifier(input.SectorIdentifier)
	if s.posDokumenRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Dokumen is unavailable")
	}
	return s.posDokumenRepository.LinkSlot(ctx, actor, input, meta, scope)
}

func (s *Service) SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	scheduleID, query = strings.TrimSpace(scheduleID), strings.TrimSpace(query)
	if scheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if query == "" {
		return DistributionSlot{}, ErrQueryRequired
	}
	if s.posPenyerahanRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Penyerahan is unavailable")
	}
	return s.posPenyerahanRepository.SearchLinkedSlot(ctx, scheduleID, query, scope)
}

func (s *Service) CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		return DistributionSlot{}, ErrScheduleRequired
	}
	if input.SlotNumber <= 0 {
		return DistributionSlot{}, ErrSlotNumberRequired
	}
	if s.posPenyerahanRepository == nil {
		return DistributionSlot{}, errors.New("distribution POS Penyerahan is unavailable")
	}
	return s.posPenyerahanRepository.CompleteSlot(ctx, actor, input, meta, scope)
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

func (s *Service) UploadMedia(ctx context.Context, actor auth.Principal, input UploadMediaInput, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaFile{}, ErrMediaUnavailable
	}
	if len(input.Data) == 0 {
		return MediaFile{}, ErrMediaTypeInvalid
	}
	if len(input.Data) > maxMediaBytes {
		return MediaFile{}, ErrMediaTooLarge
	}
	slot, err := s.mediaRepository.GetMediaSlot(ctx, strings.TrimSpace(input.SlotID), scope)
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
	storageKey, size, checksum, err := s.storage.Put(ctx, key, nil, bytes.NewReader(input.Data))
	if err != nil {
		return MediaFile{}, err
	}
	stored, err := s.mediaRepository.SaveMedia(ctx, actor, MediaFileInput{SlotID: slot.ID, StorageKey: storageKey, OriginalFilename: strings.TrimSpace(input.OriginalFilename), MimeType: mimeType, Checksum: checksum, Source: input.Source, ByteSize: size, CapturedAt: input.CapturedAt, Latitude: input.Latitude, Longitude: input.Longitude}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return MediaFile{}, err
	}
	stored.ContentURL = "/api/v1/distribution/media/" + stored.ID + "/content"
	return stored, nil
}

func (s *Service) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	if s.storage == nil || s.mediaRepository == nil {
		return ErrMediaUnavailable
	}
	item, err := s.mediaRepository.DeleteMedia(ctx, actor, strings.TrimSpace(mediaID), meta, scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, item.StorageKey); err != nil {
		_ = s.mediaRepository.RestoreMedia(context.Background(), item.ID)
		return err
	}
	return nil
}

func (s *Service) OpenMedia(ctx context.Context, mediaID string, scope auth.RegencyScope) (MediaContent, error) {
	if s.storage == nil || s.mediaRepository == nil {
		return MediaContent{}, ErrMediaUnavailable
	}
	item, err := s.mediaRepository.GetMedia(ctx, strings.TrimSpace(mediaID), scope)
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
