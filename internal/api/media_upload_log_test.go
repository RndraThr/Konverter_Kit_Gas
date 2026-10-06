package api

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"konkit/internal/media"
)

func TestLogMediaUploadContainsOnlySafeFixedFields(t *testing.T) {
	var output bytes.Buffer
	original := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(original) })

	logMediaUpload("distribution", media.KindVideo, 12345, 201, time.Now().Add(-25*time.Millisecond))
	got := output.String()
	for _, field := range []string{"endpoint=distribution", "kind=video", "size=12345", "status=201", "duration_ms="} {
		if !strings.Contains(got, field) {
			t.Fatalf("log %q missing %q", got, field)
		}
	}
	for _, sensitive := range []string{"filename", "storage_key", "oauth", "nik", "content"} {
		if strings.Contains(strings.ToLower(got), sensitive) {
			t.Fatalf("log %q contains sensitive key %q", got, sensitive)
		}
	}
}
