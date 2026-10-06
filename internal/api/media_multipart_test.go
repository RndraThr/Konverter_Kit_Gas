package api

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func mediaMultipartRequest(t *testing.T, parts func(*multipart.Writer)) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	parts(w)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func writeMultipartField(t *testing.T, w *multipart.Writer, name, value string) {
	t.Helper()
	if err := w.WriteField(name, value); err != nil {
		t.Fatal(err)
	}
}

func writeMultipartFile(t *testing.T, w *multipart.Writer, name, filename, content string) {
	t.Helper()
	part, err := w.CreateFormFile(name, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
}

func TestMediaMultipartStreamsFileAndValidatesFinalEOF(t *testing.T) {
	content := strings.Repeat("stream-me-", 100)
	req := mediaMultipartRequest(t, func(w *multipart.Writer) {
		writeMultipartField(t, w, "source", "camera")
		writeMultipartField(t, w, "file_size", strconv.Itoa(len(content)))
		writeMultipartFile(t, w, "file", "proof.jpg", content)
	})
	upload, err := openMediaMultipart(httptest.NewRecorder(), req, 501<<20, map[string]int64{"source": 16, "file_size": 20}, []string{"source", "file_size"})
	if err != nil {
		t.Fatal(err)
	}
	if upload.Filename != "proof.jpg" || upload.DeclaredSize != int64(len(content)) || upload.Fields["source"] != "camera" {
		t.Fatalf("upload=%+v", upload)
	}
	buffer := make([]byte, 37)
	var got bytes.Buffer
	for {
		n, readErr := upload.File.Read(buffer)
		got.Write(buffer[:n])
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
	if got.String() != content {
		t.Fatal("file stream content changed")
	}
}

func TestMediaMultipartRejectsInvalidStructureAndMetadata(t *testing.T) {
	tests := map[string]struct {
		parts func(*multipart.Writer)
		want  error
	}{
		"file before metadata": {parts: func(w *multipart.Writer) { writeMultipartFile(t, w, "file", "proof.jpg", "x") }, want: errMultipartMetadataRequired},
		"duplicate field": {parts: func(w *multipart.Writer) {
			writeMultipartField(t, w, "source", "camera")
			writeMultipartField(t, w, "source", "gallery")
		}, want: errMultipartDuplicateField},
		"unknown field": {parts: func(w *multipart.Writer) { writeMultipartField(t, w, "secret", "value") }, want: errMultipartUnknownField},
		"negative size": {parts: func(w *multipart.Writer) {
			writeMultipartField(t, w, "source", "camera")
			writeMultipartField(t, w, "file_size", "-1")
			writeMultipartFile(t, w, "file", "proof.jpg", "x")
		}, want: errMultipartInvalidFileSize},
		"nonnumeric size": {parts: func(w *multipart.Writer) {
			writeMultipartField(t, w, "source", "camera")
			writeMultipartField(t, w, "file_size", "large")
			writeMultipartFile(t, w, "file", "proof.jpg", "x")
		}, want: errMultipartInvalidFileSize},
		"oversize field": {parts: func(w *multipart.Writer) { writeMultipartField(t, w, "source", strings.Repeat("x", 65537)) }, want: errMultipartMetadataTooLarge},
		"aggregate metadata": {parts: func(w *multipart.Writer) {
			writeMultipartField(t, w, "source", strings.Repeat("x", 65530))
			writeMultipartField(t, w, "file_size", "1234567890")
		}, want: errMultipartMetadataTooLarge},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := mediaMultipartRequest(t, tt.parts)
			limits := map[string]int64{"source": 65536, "file_size": 20}
			_, err := openMediaMultipart(httptest.NewRecorder(), req, 501<<20, limits, []string{"source", "file_size"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v, want %v", err, tt.want)
			}
		})
	}
}

func TestMediaMultipartRejectsTrailingOrMultipleFilePartsWhileStreaming(t *testing.T) {
	for name, trailing := range map[string]func(*multipart.Writer){
		"trailing field": func(w *multipart.Writer) { writeMultipartField(t, w, "source", "again") },
		"multiple files": func(w *multipart.Writer) { writeMultipartFile(t, w, "file", "second.jpg", "second") },
	} {
		t.Run(name, func(t *testing.T) {
			req := mediaMultipartRequest(t, func(w *multipart.Writer) {
				writeMultipartField(t, w, "source", "camera")
				writeMultipartField(t, w, "file_size", "5")
				writeMultipartFile(t, w, "file", "first.jpg", "first")
				trailing(w)
			})
			upload, err := openMediaMultipart(httptest.NewRecorder(), req, 501<<20, map[string]int64{"source": 16, "file_size": 20}, []string{"source", "file_size"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, upload.File)
			if !errors.Is(err, errMultipartTrailingPart) {
				t.Fatalf("err=%v, want errMultipartTrailingPart", err)
			}
		})
	}
}
