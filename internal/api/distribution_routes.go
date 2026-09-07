package api

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"konkit/internal/distribution"
)

const maxMediaRequestBody = 11 << 20

func (h *Handler) handleDistributionSearch(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if scheduleID == "" || query == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan kata pencarian wajib diisi", map[string]string{"schedule_id": "Jadwal wajib dipilih", "q": "Kata pencarian wajib diisi"})
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Batas hasil tidak valid", map[string]string{"limit": "Gunakan angka 1 sampai 20"})
			return
		}
		limit = parsed
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.Search(r.Context(), scheduleID, query, limit, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionAllocation(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "distribution.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Distribution.GetWorkspace(r.Context(), parts[0], scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "draft" && r.Method == http.MethodPatch {
		if !h.authorize(w, r, rc.principal, "distribution.manage") {
			return
		}
		var input distribution.DraftInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Distribution.SaveDraft(r.Context(), rc.principal, parts[0], input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "complete" && r.Method == http.MethodPost {
		if !h.authorize(w, r, rc.principal, "distribution.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Distribution.Complete(r.Context(), rc.principal, parts[0], clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "draft" {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	if len(parts) == 2 && parts[1] == "complete" {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func (h *Handler) handleDistributionSlot(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "media" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "documentation.manage") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaRequestBody)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Foto melebihi batas 10 MiB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Foto wajib dipilih", map[string]string{"file": "Foto wajib dipilih"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "media_invalid", "Foto tidak dapat dibaca")
		return
	}
	if len(data) > 10<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Foto melebihi batas 10 MiB")
		return
	}
	input := distribution.UploadMediaInput{SlotID: parts[0], OriginalFilename: header.Filename, Source: strings.TrimSpace(r.FormValue("source")), Data: data}
	if raw := strings.TrimSpace(r.FormValue("captured_at")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Waktu pengambilan tidak valid", map[string]string{"captured_at": "Gunakan waktu RFC3339"})
			return
		}
		input.CapturedAt = &value
	}
	latitude, err := optionalFloat(r.FormValue("latitude"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"latitude": "Latitude tidak valid"})
		return
	}
	input.Latitude = latitude
	longitude, err := optionalFloat(r.FormValue("longitude"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"longitude": "Longitude tidak valid"})
		return
	}
	input.Longitude = longitude
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.UploadMedia(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) handleDistributionMedia(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "distribution.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		content, err := h.deps.Distribution.OpenMedia(r.Context(), parts[0], scope)
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
		if !h.authorize(w, r, rc.principal, "documentation.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Distribution.DeleteMedia(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func optionalFloat(raw string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
