package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

const maxMultipartMetadataBytes int64 = 64 << 10

var (
	errMultipartMetadataRequired = errors.New("multipart metadata must precede the file")
	errMultipartDuplicateField   = errors.New("duplicate multipart field")
	errMultipartUnknownField     = errors.New("unknown multipart field")
	errMultipartMetadataTooLarge = errors.New("multipart metadata is too large")
	errMultipartInvalidFileSize  = errors.New("invalid multipart file_size")
	errMultipartFileRequired     = errors.New("multipart file is required")
	errMultipartTrailingPart     = errors.New("multipart file must be the final part")
)

type mediaMultipart struct {
	Fields       map[string]string
	Filename     string
	File         io.Reader
	DeclaredSize int64
}

func openMediaMultipart(w http.ResponseWriter, r *http.Request, maxBody int64, allowedFields map[string]int64, requiredFields []string) (mediaMultipart, error) {
	if maxBody > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return mediaMultipart{}, fmt.Errorf("open multipart body: %w", err)
	}
	fields := make(map[string]string, len(allowedFields))
	var metadataBytes int64
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			return mediaMultipart{}, errMultipartFileRequired
		}
		if nextErr != nil {
			return mediaMultipart{}, fmt.Errorf("read multipart part: %w", nextErr)
		}

		name := part.FormName()
		if part.FileName() != "" {
			if name != "file" {
				_ = part.Close()
				return mediaMultipart{}, errMultipartUnknownField
			}
			for _, required := range requiredFields {
				if _, ok := fields[required]; !ok {
					_ = part.Close()
					return mediaMultipart{}, fmt.Errorf("%w: %s", errMultipartMetadataRequired, required)
				}
			}
			declaredSize := int64(0)
			if raw, ok := fields["file_size"]; ok {
				declaredSize, err = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil || declaredSize < 0 {
					_ = part.Close()
					return mediaMultipart{}, errMultipartInvalidFileSize
				}
			}
			return mediaMultipart{
				Fields:       fields,
				Filename:     part.FileName(),
				File:         &finalMultipartFileReader{part: part, multipart: reader},
				DeclaredSize: declaredSize,
			}, nil
		}

		limit, ok := allowedFields[name]
		if !ok {
			_ = part.Close()
			return mediaMultipart{}, fmt.Errorf("%w: %s", errMultipartUnknownField, name)
		}
		if _, duplicate := fields[name]; duplicate {
			_ = part.Close()
			return mediaMultipart{}, fmt.Errorf("%w: %s", errMultipartDuplicateField, name)
		}
		value, readErr := io.ReadAll(io.LimitReader(part, limit+1))
		_ = part.Close()
		if readErr != nil {
			return mediaMultipart{}, fmt.Errorf("read multipart metadata: %w", readErr)
		}
		metadataBytes += int64(len(value))
		if int64(len(value)) > limit || metadataBytes > maxMultipartMetadataBytes {
			return mediaMultipart{}, errMultipartMetadataTooLarge
		}
		fields[name] = string(value)
	}
}

type finalMultipartFileReader struct {
	part      *multipart.Part
	multipart *multipart.Reader
	finished  bool
}

func (r *finalMultipartFileReader) Read(p []byte) (int, error) {
	if r.finished {
		return 0, io.EOF
	}
	n, err := r.part.Read(p)
	if err == nil {
		return n, nil
	}
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	if n > 0 {
		return n, nil
	}
	_ = r.part.Close()
	next, nextErr := r.multipart.NextPart()
	if errors.Is(nextErr, io.EOF) {
		r.finished = true
		return 0, io.EOF
	}
	if nextErr != nil {
		return 0, fmt.Errorf("validate final multipart part: %w", nextErr)
	}
	_ = next.Close()
	return 0, errMultipartTrailingPart
}
