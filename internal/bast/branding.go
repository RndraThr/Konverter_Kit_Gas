package bast

import (
	"io"
	"net/http"
	"regexp"
	"strings"
)

// maxLogoBytes bounds an uploaded BA logo to 10 MiB (matches the DB CHECK on
// program_ba_logo_assets.byte_size).
const maxLogoBytes = 10 << 20

var brandingSlotPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// LogoAsset is a single BA branding logo belonging to a program.
type LogoAsset struct {
	ID               string  `json:"id"`
	ProgramID        string  `json:"program_id"`
	SlotCode         string  `json:"slot_code"`
	StorageKey       string  `json:"-"`
	OriginalFilename string  `json:"original_filename"`
	MimeType         string  `json:"mime_type"`
	ByteSize         int64   `json:"byte_size"`
	Checksum         string  `json:"checksum"`
	SortOrder        int     `json:"sort_order"`
	MaxWidthMM       float64 `json:"max_width_mm"`
	MaxHeightMM      float64 `json:"max_height_mm"`
	IsVisible        bool    `json:"is_visible"`
}

// LogoUploadInput carries a new or replacement logo for a program slot.
type LogoUploadInput struct {
	ProgramID        string
	SlotCode         string
	OriginalFilename string
	Data             []byte
	SortOrder        int
	MaxWidthMM       float64
	MaxHeightMM      float64
}

// LogoPatchInput updates ordering, size bounds, and visibility of a logo.
type LogoPatchInput struct {
	ID          string
	ProgramID   string
	SortOrder   int
	MaxWidthMM  float64
	MaxHeightMM float64
	IsVisible   bool
}

// LogoContent streams a stored logo back to the caller for preview.
type LogoContent struct {
	Reader   io.ReadCloser
	MimeType string
	Filename string
}

// normalizeUpload trims, lowercases the slot, applies size defaults, and
// validates the payload. It returns the detected MIME type.
func (in *LogoUploadInput) normalizeUpload() (string, error) {
	in.ProgramID = strings.TrimSpace(in.ProgramID)
	in.SlotCode = strings.ToLower(strings.TrimSpace(in.SlotCode))
	in.OriginalFilename = strings.TrimSpace(in.OriginalFilename)
	if in.ProgramID == "" || !brandingSlotPattern.MatchString(in.SlotCode) || len(in.Data) == 0 || len(in.Data) > maxLogoBytes || in.SortOrder < 0 {
		return "", ErrInvalidInput
	}
	mimeType := http.DetectContentType(in.Data[:min(len(in.Data), 512)])
	if mimeType != "image/png" && mimeType != "image/jpeg" {
		return "", ErrInvalidInput
	}
	if in.MaxWidthMM <= 0 {
		in.MaxWidthMM = 35
	}
	if in.MaxHeightMM <= 0 {
		in.MaxHeightMM = 18
	}
	return mimeType, nil
}

func (in *LogoPatchInput) normalizePatch() error {
	in.ID = strings.TrimSpace(in.ID)
	in.ProgramID = strings.TrimSpace(in.ProgramID)
	if in.ID == "" || in.ProgramID == "" || in.SortOrder < 0 || in.MaxWidthMM <= 0 || in.MaxHeightMM <= 0 {
		return ErrInvalidInput
	}
	return nil
}
