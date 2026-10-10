package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"konkit/internal/activities"
	"konkit/internal/auth"
	"konkit/internal/media"
)

const maxActivityMediaRequestBody = 501 << 20

func activityFilterFromRequest(r *http.Request) activities.Filter {
	return activities.Filter{
		ProgramID: r.URL.Query().Get("program_id"), RegencyID: r.URL.Query().Get("regency_id"), ActivityType: r.URL.Query().Get("activity_type"),
		Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 24),
	}
}

func (h *Handler) handleActivitiesMedia(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		page, err := h.deps.Activities.List(r.Context(), activityFilterFromRequest(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, page)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "activities.manage") {
			return
		}
		started := time.Now()
		kind := media.Kind("unknown")
		loggedSize := int64(0)
		tracked := &statusTrackingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		w = tracked
		defer func() { logMediaUpload("activity", kind, loggedSize, tracked.status, started) }()

		upload, err := openMediaMultipart(w, r, maxActivityMediaRequestBody, map[string]int64{
			"program_id": 128, "regency_id": 128, "activity_type": 64, "source": 16, "file_size": 20,
		}, []string{"program_id", "regency_id", "activity_type", "source", "file_size"})
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
			} else {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			}
			return
		}
		loggedSize = upload.DeclaredSize
		input := activities.UploadInput{
			ProgramID: strings.TrimSpace(upload.Fields["program_id"]), RegencyID: strings.TrimSpace(upload.Fields["regency_id"]), ActivityType: strings.TrimSpace(upload.Fields["activity_type"]),
			OriginalFilename: upload.Filename, Source: strings.TrimSpace(upload.Fields["source"]), Data: upload.File, DeclaredSize: upload.DeclaredSize,
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Activities.Upload(r.Context(), rc.principal, input, clientMeta(r), scope)
		if err != nil {
			var tooLarge *http.MaxBytesError
			switch {
			case errors.Is(err, errMultipartTrailingPart):
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			case errors.As(err, &tooLarge):
				writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
			default:
				writeServiceError(w, err)
			}
			return
		}
		kind = mediaKindFromMIME(result.MimeType)
		loggedSize = result.ByteSize
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleActivityMediaItem(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		content, err := h.deps.Activities.OpenContent(r.Context(), parts[0], scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", content.MimeType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if !h.authorize(w, r, rc.principal, "activities.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Activities.Delete(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

// ActivitySyncService is implemented by the activities service: delta sync of
// a program+regency's activity documentation for the mobile app.
type ActivitySyncService interface {
	Sync(ctx context.Context, programID, regencyID, since string, scope auth.RegencyScope) (activities.SyncResult, error)
}

func (h *Handler) handleActivitiesSync(w http.ResponseWriter, r *http.Request, rc requestContext) {
	service, ok := h.deps.Activities.(ActivitySyncService)
	if !ok {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "activities.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	q := r.URL.Query()
	result, err := service.Sync(r.Context(), q.Get("program_id"), q.Get("regency_id"), q.Get("since"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
