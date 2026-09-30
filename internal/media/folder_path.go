package media

import (
	"errors"
	"strings"
	"unicode"
)

// FolderCategory names one of the fixed top-level document categories that
// sit under a regency folder in storage.
type FolderCategory string

const (
	FolderBA         FolderCategory = "BERITA ACARA (BA)"
	FolderSupporting FolderCategory = "DOKUMEN PENDUKUNG"
	FolderPhotos     FolderCategory = "DOKUMENTASI (FOTO)"
)

// FolderPathInput describes the pieces needed to build a storage folder
// path for a program zone. ProgramType is the raw program type ("farmer" or
// "fisherman"); Child is an optional trailing segment (e.g. an activity
// name) and is omitted from the built path when empty.
type FolderPathInput struct {
	ProgramType, ZoneName, RegencyName string
	Category                           FolderCategory
	Child                              string
}

// ErrInvalidFolderPath is returned by BuildFolderPath when a required field
// is missing, a program type is unrecognized, or a segment contains a path
// separator or control character.
var ErrInvalidFolderPath = errors.New("media: invalid folder path")

// BuildFolderPath constructs the storage folder path segments for a
// program zone, e.g.:
//
//	BuildFolderPath(FolderPathInput{ProgramType: "farmer", ZoneName: "Zona 1",
//	    RegencyName: "Kabupaten Wajo", Category: FolderPhotos, Child: "RAKOR"})
//	// -> []string{"PETANI", "ZONA 1", "KABUPATEN WAJO", "DOKUMENTASI (FOTO)", "RAKOR"}
//
// Segments are trimmed, have internal whitespace collapsed, and are
// uppercased for display. Segments containing '/', '\', or control
// characters are rejected (ErrInvalidFolderPath) rather than being split
// into extra path levels.
func BuildFolderPath(input FolderPathInput) ([]string, error) {
	programSegment, err := programTypeSegment(input.ProgramType)
	if err != nil {
		return nil, err
	}

	zoneSegment, err := sanitizeFolderSegment(input.ZoneName)
	if err != nil {
		return nil, err
	}
	if zoneSegment == "" {
		return nil, ErrInvalidFolderPath
	}

	regencySegment, err := sanitizeFolderSegment(input.RegencyName)
	if err != nil {
		return nil, err
	}
	if regencySegment == "" {
		return nil, ErrInvalidFolderPath
	}

	categorySegment, err := sanitizeFolderSegment(string(input.Category))
	if err != nil {
		return nil, err
	}
	if categorySegment == "" {
		return nil, ErrInvalidFolderPath
	}

	path := []string{programSegment, zoneSegment, regencySegment, categorySegment}

	if strings.TrimSpace(input.Child) != "" {
		childSegment, err := sanitizeFolderSegment(input.Child)
		if err != nil {
			return nil, err
		}
		if childSegment != "" {
			path = append(path, childSegment)
		}
	}

	return path, nil
}

func programTypeSegment(programType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(programType)) {
	case "farmer":
		return "PETANI", nil
	case "fisherman":
		return "NELAYAN", nil
	default:
		return "", ErrInvalidFolderPath
	}
}

// sanitizeFolderSegment trims leading/trailing whitespace, collapses
// repeated internal whitespace, and uppercases the result. It rejects
// values containing '/', '\', or control characters rather than splitting
// them into extra path levels.
func sanitizeFolderSegment(value string) (string, error) {
	for _, r := range value {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return "", ErrInvalidFolderPath
		}
	}

	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToUpper(strings.Join(fields, " ")), nil
}
