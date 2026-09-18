package api

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"konkit/internal/activities"
)

const maxActivityMediaRequestBody = 101 << 20 // 100 MiB + multipart overhead margin

func activityFilterFromRequest(r *http.Request) activities.Filter {
	return activities.Filter{
		RegencyID: r.URL.Query().Get("regency_id"), ActivityType: r.URL.Query().Get("activity_type"),
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
		r.Body = http.MaxBytesReader(w, r.Body, maxActivityMediaRequestBody)
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "File melebihi batas 100 MiB")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "File wajib dipilih", map[string]string{"file": "File wajib dipilih"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (100<<20)+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "media_invalid", "File tidak dapat dibaca")
			return
		}
		if len(data) > 100<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "File melebihi batas 100 MiB")
			return
		}
		input := activities.UploadInput{
			RegencyID: strings.TrimSpace(r.FormValue("regency_id")), ActivityType: strings.TrimSpace(r.FormValue("activity_type")),
			OriginalFilename: header.Filename, Source: strings.TrimSpace(r.FormValue("source")), Data: data,
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Activities.Upload(r.Context(), rc.principal, input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
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
