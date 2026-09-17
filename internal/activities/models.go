package activities

import (
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound            = errors.New("activity media not found")
	ErrRegencyRequired     = errors.New("regency_id is required")
	ErrRegencyNotFound     = errors.New("regency not found")
	ErrActivityTypeInvalid = errors.New("activity_type is not a recognized activity type")
	ErrMediaTypeInvalid    = errors.New("file must be a supported image or video format")
	ErrFileTooLarge        = errors.New("file exceeds the 100 MiB size limit")
	ErrSourceInvalid       = errors.New(`source must be "camera" or "gallery"`)
)

// maxFileBytes mirrors activity_media.byte_size's CHECK constraint (100 MiB).
const maxFileBytes = 100 << 20

var activityTypeCodes = map[string]string{
	"ceremony_sosialisasi":  "CEREMONY",
	"pelatihan_teknis":      "PELATIHAN",
	"rakor":                 "RAKOR",
	"training_10":           "TRAINING10",
	"training_100":          "TRAINING100",
	"unloading_konkit":      "UNLOADKONKIT",
	"unloading_mesin_pompa": "UNLOADPOMPA",
	"unloading_oli":         "UNLOADOLI",
	"unloading_selang":      "UNLOADSELANG",
	"unloading_tabung_gas":  "UNLOADTABUNG",
}

// activityTypeFolderNames are the exact Google Drive folder names under
// "DOKUMENTASI FOTO & VIDEO" for each activity type (spec section 4.3).
var activityTypeFolderNames = map[string]string{
	"ceremony_sosialisasi":  "Ceremony & Sosialisasi",
	"pelatihan_teknis":      "Pelatihan Teknis",
	"rakor":                 "Rakor",
	"training_10":           "Training 10%",
	"training_100":          "Training 100%",
	"unloading_konkit":      "Unloading Konkit",
	"unloading_mesin_pompa": "Unloading Mesin Pompa",
	"unloading_oli":         "Unloading Oli",
	"unloading_selang":      "Unloading Selang Hisap & Buang",
	"unloading_tabung_gas":  "Unloading Tabung Gas",
}

var allowedMimeTypes = map[string]string{
	"image/jpeg":      "image",
	"image/png":       "image",
	"image/webp":      "image",
	"video/mp4":       "video",
	"video/webm":      "video",
	"video/quicktime": "video",
}

func isValidActivityType(activityType string) bool {
	_, ok := activityTypeCodes[activityType]
	return ok
}

type ActivityMedia struct {
	ID                  string    `json:"id"`
	RegencyID           string    `json:"regency_id"`
	RegencyName         string    `json:"regency_name"`
	RegencyDocumentCode string    `json:"regency_document_code"`
	ActivityType        string    `json:"activity_type"`
	StorageKey          string    `json:"-"`
	DisplayName         string    `json:"display_name"`
	OriginalFilename    string    `json:"original_filename"`
	MediaType           string    `json:"media_type"`
	MimeType            string    `json:"mime_type"`
	ByteSize            int64     `json:"byte_size"`
	Checksum            string    `json:"checksum"`
	Source              string    `json:"source"`
	Status              string    `json:"status"`
	UploadedBy          *string   `json:"uploaded_by,omitempty"`
	UploadedAt          time.Time `json:"uploaded_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ContentURL          string    `json:"content_url"`
}

type Filter struct {
	RegencyID    string
	ActivityType string
	Page         int
	PageSize     int
}

type Page struct {
	Items    []ActivityMedia `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total"`
}

type UploadInput struct {
	RegencyID        string
	ActivityType     string
	OriginalFilename string
	Source           string
	Data             []byte
}

type MediaContent struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}
