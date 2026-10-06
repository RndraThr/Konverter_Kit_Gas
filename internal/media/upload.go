package media

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"

	MaxImageBytes int64 = 25 << 20
	MaxVideoBytes int64 = 500 << 20
)

var ErrUnsupportedUpload = errors.New("unsupported media upload")

type DetectedUpload struct {
	Kind      Kind
	MimeType  string
	Extension string
	Reader    io.Reader
	MaxBytes  int64
}

func DetectUpload(source io.Reader, filename string) (DetectedUpload, error) {
	if source == nil {
		return DetectedUpload{}, ErrUnsupportedUpload
	}
	prefix := make([]byte, 512)
	n, err := io.ReadFull(source, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return DetectedUpload{}, err
	}
	if n == 0 {
		return DetectedUpload{}, ErrUnsupportedUpload
	}
	prefix = prefix[:n]
	mimeType := http.DetectContentType(prefix)
	extension := strings.ToLower(filepath.Ext(filename))

	detected := DetectedUpload{Reader: io.MultiReader(bytes.NewReader(prefix), source)}
	switch mimeType {
	case "image/jpeg":
		detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindImage, mimeType, ".jpg", MaxImageBytes
	case "image/png":
		detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindImage, mimeType, ".png", MaxImageBytes
	case "image/webp":
		detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindImage, mimeType, ".webp", MaxImageBytes
	case "video/mp4":
		detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindVideo, mimeType, ".mp4", MaxVideoBytes
		if extension == ".mov" {
			detected.MimeType, detected.Extension = "video/quicktime", ".mov"
		}
	case "video/webm":
		detected.Kind, detected.MimeType, detected.Extension, detected.MaxBytes = KindVideo, mimeType, ".webm", MaxVideoBytes
	case "application/octet-stream":
		detected.Kind, detected.MaxBytes = KindVideo, MaxVideoBytes
		switch extension {
		case ".mp4":
			detected.MimeType, detected.Extension = "video/mp4", extension
		case ".mov":
			detected.MimeType, detected.Extension = "video/quicktime", extension
		case ".webm":
			detected.MimeType, detected.Extension = "video/webm", extension
		default:
			return DetectedUpload{}, ErrUnsupportedUpload
		}
	default:
		return DetectedUpload{}, ErrUnsupportedUpload
	}
	return detected, nil
}

func PolicyAllows(policy string, kind Kind) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "image":
		return kind == KindImage
	case "video":
		return kind == KindVideo
	case "image_video":
		return kind == KindImage || kind == KindVideo
	default:
		return false
	}
}

type VideoLimiter struct {
	permits chan struct{}
}

func NewVideoLimiter(maxConcurrent int) *VideoLimiter {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &VideoLimiter{permits: make(chan struct{}, maxConcurrent)}
}

func (l *VideoLimiter) TryAcquire() (release func(), ok bool) {
	if l == nil {
		return func() {}, true
	}
	select {
	case l.permits <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-l.permits })
		}, true
	default:
		return nil, false
	}
}
