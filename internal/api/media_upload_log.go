package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"konkit/internal/media"
)

func logMediaUpload(endpoint string, kind media.Kind, size int64, status int, started time.Time) {
	log.Printf("media_upload endpoint=%s kind=%s size=%d status=%d duration_ms=%d", endpoint, kind, size, status, time.Since(started).Milliseconds())
}

type statusTrackingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusTrackingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func mediaKindFromMIME(mimeType string) media.Kind {
	if strings.HasPrefix(mimeType, "image/") {
		return media.KindImage
	}
	if strings.HasPrefix(mimeType, "video/") {
		return media.KindVideo
	}
	return media.Kind("unknown")
}
