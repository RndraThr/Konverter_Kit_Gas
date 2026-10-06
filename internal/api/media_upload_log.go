package api

import (
	"log"
	"time"

	"konkit/internal/media"
)

func logMediaUpload(endpoint string, kind media.Kind, size int64, status int, started time.Time) {
	log.Printf("media_upload endpoint=%s kind=%s size=%d status=%d duration_ms=%d", endpoint, kind, size, status, time.Since(started).Milliseconds())
}
