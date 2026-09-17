package activities

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
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
	repository repository
	storage    media.Storage
}

func NewService(repository repository, storage media.Storage) *Service {
	return &Service{repository: repository, storage: storage}
}

func (s *Service) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
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
	input.RegencyID = strings.TrimSpace(input.RegencyID)
	input.ActivityType = strings.TrimSpace(input.ActivityType)
	input.Source = strings.TrimSpace(input.Source)
	if !isValidActivityType(input.ActivityType) {
		return ActivityMedia{}, ErrActivityTypeInvalid
	}
	if input.Source != "camera" && input.Source != "gallery" {
		return ActivityMedia{}, ErrSourceInvalid
	}
	if len(input.Data) == 0 {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	if len(input.Data) > maxFileBytes {
		return ActivityMedia{}, ErrFileTooLarge
	}
	regency, err := s.repository.GetRegency(ctx, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	mimeType, mediaType, ok := detectMediaType(input.Data, input.OriginalFilename)
	if !ok {
		return ActivityMedia{}, ErrMediaTypeInvalid
	}
	key, err := newStorageKey()
	if err != nil {
		return ActivityMedia{}, err
	}
	folderPath := []string{
		fmt.Sprintf("Konkit %d", time.Now().Year()),
		regency.Name,
		"DOKUMENTASI FOTO & VIDEO",
		activityTypeFolderNames[input.ActivityType],
	}
	storageKey, size, checksum, err := s.storage.Put(ctx, key, folderPath, bytes.NewReader(input.Data))
	if err != nil {
		return ActivityMedia{}, err
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, activityTypeCodes[input.ActivityType], time.Now().Format("20060102-150405"))
	stored, err := s.repository.Insert(ctx, actor, insertInput{
		RegencyID: input.RegencyID, ActivityType: input.ActivityType, StorageKey: storageKey,
		DisplayName: displayName, OriginalFilename: strings.TrimSpace(input.OriginalFilename),
		MediaType: mediaType, MimeType: mimeType, ByteSize: size, Checksum: checksum, Source: input.Source,
	}, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey)
		return ActivityMedia{}, err
	}
	stored.ContentURL = "/api/v1/activities/media/" + stored.ID + "/content"
	return stored, nil
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

// detectMediaType sniffs the upload's mime type. http.DetectContentType
// alone is unreliable for some video containers (e.g. .mov often sniffs as
// application/octet-stream), so an allow-listed extension is used as a
// fallback — never trusting the client-supplied header, only sniffed bytes
// or the upload's own filename extension.
func detectMediaType(data []byte, filename string) (mimeType, mediaType string, ok bool) {
	sniffed := http.DetectContentType(data[:min(len(data), 512)])
	if mt, found := allowedMimeTypes[sniffed]; found {
		return sniffed, mt, true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		return "video/mp4", "video", true
	case ".webm":
		return "video/webm", "video", true
	case ".mov":
		return "video/quicktime", "video", true
	}
	return "", "", false
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
