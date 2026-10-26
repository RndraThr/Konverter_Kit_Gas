package activities

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"konkit/internal/auth"
	"konkit/internal/media"
	"konkit/internal/programs"
)

type repository interface {
	GetRegency(ctx context.Context, regencyID string, scope auth.RegencyScope) (regencyInfo, error)
	List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error)
	Insert(ctx context.Context, actor auth.Principal, input insertInput, meta auth.ClientMeta) (ActivityMedia, error)
	GetByID(ctx context.Context, id string, scope auth.RegencyScope) (ActivityMedia, error)
	SoftDelete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) (string, error)
	restoreAfterFailedStorageDelete(ctx context.Context, id string) error
}

type Service struct {
	repository      repository
	slotRepository  slotRepository
	storage         media.Storage
	programContexts programContextResolver
	videoLimiter    *media.VideoLimiter
	now             func() time.Time
}

type programContextResolver interface {
	ResolveStorageContext(context.Context, string, string, auth.RegencyScope) (programs.StorageContext, error)
}

func NewService(repository repository, storage media.Storage, dependencies ...any) *Service {
	var resolver programContextResolver
	var videoLimiter *media.VideoLimiter
	for _, dependency := range dependencies {
		switch value := dependency.(type) {
		case programContextResolver:
			resolver = value
		case *media.VideoLimiter:
			videoLimiter = value
		}
	}
	slotRepo := repository.(slotRepository)
	return &Service{repository: repository, slotRepository: slotRepo, storage: storage, programContexts: resolver, videoLimiter: videoLimiter, now: time.Now}
}

func (s *Service) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	filter.ProgramID = strings.TrimSpace(filter.ProgramID)
	filter.RegencyID = strings.TrimSpace(filter.RegencyID)
	filter.ActivityType = strings.TrimSpace(filter.ActivityType)
	if !isValidActivityType(filter.ActivityType) {
		return Page{}, ErrActivityTypeInvalid
	}
	if filter.RegencyID == "" {
		return Page{}, ErrRegencyRequired
	}
	page, err := s.repository.List(ctx, filter, scope)
	if err != nil {
		return Page{}, err
	}
	for i := range page.Items {
		page.Items[i].ContentURL = "/api/v1/activities/media/" + page.Items[i].ID + "/content"
	}
	return page, nil
}

func (s *Service) Upload(ctx context.Context, actor auth.Principal, input UploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error) {
	input.ProgramID = strings.TrimSpace(input.ProgramID)
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.ActivityType = strings.TrimSpace(input.ActivityType)
	input.Source = strings.TrimSpace(input.Source)
	input.SlotID = strings.TrimSpace(input.SlotID)
	input.CustomName = strings.TrimSpace(input.CustomName)
	if !isValidActivityType(input.ActivityType) {
		return ActivityMedia{}, ErrActivityTypeInvalid
	}
	if input.Source != "camera" && input.Source != "gallery" {
		return ActivityMedia{}, ErrSourceInvalid
	}
	if input.ProgramID == "" || s.programContexts == nil {
		return ActivityMedia{}, ErrProgramRequired
	}

	var nameSegment string
	if input.ActivityType == "dokumen_pendukung" {
		segment, err := s.resolveDocumentNameSegment(ctx, input.RegencyID, input.SlotID, input.CustomName, scope)
		if err != nil {
			return ActivityMedia{}, err
		}
		nameSegment = segment
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, input.ProgramID, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	regency, err := s.repository.GetRegency(ctx, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	detected, err := media.DetectUpload(input.Data, input.OriginalFilename)
	if errors.Is(err, media.ErrUnsupportedUpload) {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if err != nil {
		return ActivityMedia{}, err
	}
	if !media.PolicyAllows(policyFor(input.ActivityType), detected.Kind) {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if input.DeclaredSize > detected.MaxBytes {
		return ActivityMedia{}, ErrFileTooLarge
	}
	if detected.Kind == media.KindVideo {
		release, ok := s.videoLimiter.TryAcquire()
		if !ok {
			return ActivityMedia{}, ErrVideoUploadBusy
		}
		defer release()
	}
	category, child := folderDestination(input.ActivityType)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName,
		RegencyName: storageContext.RegencyName, Category: category, Child: child,
	})
	if err != nil {
		return ActivityMedia{}, err
	}
	if nameSegment == "" {
		nameSegment = activityTypeCodes[input.ActivityType]
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, nameSegment, s.now().Format("20060102-150405"))

	return s.store(ctx, actor, preparedUpload{
		ActivityType: input.ActivityType, ProgramID: input.ProgramID, RegencyID: input.RegencyID,
		DisplayName: displayName, OriginalFilename: strings.TrimSpace(input.OriginalFilename),
		Source: input.Source, Detected: detected,
	}, folderPath, meta)
}

// preparedUpload carries everything store() needs to persist a detected
// upload (a direct file upload or an assembled scan PDF) to storage and the
// activity_media table.
type preparedUpload struct {
	ActivityType     string
	ProgramID        string
	RegencyID        string
	DisplayName      string
	OriginalFilename string
	Source           string
	Detected         media.DetectedUpload
}

// store performs the shared storage+insert tail for Upload and ScanUpload:
// generate a storage key, write the file, enforce the size cap, insert the
// activity_media row, and roll back storage on insert failure.
func (s *Service) store(ctx context.Context, actor auth.Principal, prepared preparedUpload, folderPath []string, meta auth.ClientMeta) (ActivityMedia, error) {
	key, err := newStorageKey()
	if err != nil {
		return ActivityMedia{}, err
	}
	visibleFilename := prepared.DisplayName + activityFileExtension(prepared.Detected.MimeType)
	storageKey, size, checksum, err := media.PutNamed(ctx, s.storage, key, visibleFilename, folderPath, io.LimitReader(prepared.Detected.Reader, prepared.Detected.MaxBytes+1))
	if err != nil {
		return ActivityMedia{}, err
	}
	if size > prepared.Detected.MaxBytes {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, ErrFileTooLarge
	}
	stored, err := s.repository.Insert(ctx, actor, insertInput{
		ProgramID: prepared.ProgramID, RegencyID: prepared.RegencyID, ActivityType: prepared.ActivityType, StorageKey: storageKey,
		DisplayName: prepared.DisplayName, OriginalFilename: prepared.OriginalFilename,
		MediaType: string(prepared.Detected.Kind), MimeType: prepared.Detected.MimeType, ByteSize: size, Checksum: checksum, Source: prepared.Source,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, err
	}
	stored.ContentURL = "/api/v1/activities/media/" + stored.ID + "/content"
	return stored, nil
}

func activityFileExtension(mimeType string) string {
	return map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
		"video/mp4": ".mp4", "video/webm": ".webm", "video/quicktime": ".mov",
		"application/pdf": ".pdf",
	}[mimeType]
}

func folderDestination(activityType string) (media.FolderCategory, string) {
	if activityType == "dokumen_pendukung" {
		return media.FolderSupporting, ""
	}
	return media.FolderPhotos, activityTypeFolderNames[activityType]
}

func policyFor(activityType string) string {
	if activityType == "dokumen_pendukung" {
		return "document"
	}
	return "image_video"
}

// sanitizeSlotName turns a document name into a file-name segment: letters
// and digits kept (upper case), everything else (spaces, commas, slashes,
// colons...) collapsed into single dashes, so a name never breaks the file
// name or the Drive folder path.
func sanitizeSlotName(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToUpper(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// resolveDocumentNameSegment picks the sanitized name segment for a
// dokumen_pendukung upload: a slot from the regency's managed list (preferred),
// or an ad-hoc free-text name for a document outside that list. Exactly one of
// slotID/customName must resolve to a name, or ErrSlotInvalid/ErrDocumentNameRequired
// is returned.
func (s *Service) resolveDocumentNameSegment(ctx context.Context, regencyID, slotID, customName string, scope auth.RegencyScope) (string, error) {
	if slotID != "" {
		slots, err := s.ListSlots(ctx, regencyID, false, scope)
		if err != nil {
			return "", err
		}
		for _, candidate := range slots {
			if candidate.ID == slotID {
				return sanitizeSlotName(candidate.Name), nil
			}
		}
		return "", ErrSlotInvalid
	}
	if customName != "" {
		return sanitizeSlotName(customName), nil
	}
	return "", ErrDocumentNameRequired
}

func (s *Service) Delete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	storageKey, err := s.repository.SoftDelete(ctx, actor, id, meta, scope)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, storageKey); err != nil {
		_ = s.repository.restoreAfterFailedStorageDelete(context.Background(), id)
		return err
	}
	return nil
}

func (s *Service) OpenContent(ctx context.Context, id string, scope auth.RegencyScope) (MediaContent, error) {
	item, err := s.repository.GetByID(ctx, id, scope)
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
