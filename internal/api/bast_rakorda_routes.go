package api

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strings"

	"konkit/internal/bast"
)

func (h *Handler) handleBASTRakorda(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.Rakorda == nil {
		writeUnavailable(w)
		return
	}
	switch {
	case suffix == "preview":
		h.handleRakordaPreview(w, r, rc)
	case suffix == "uploads":
		h.handleRakordaUploads(w, r, rc)
	case strings.HasPrefix(suffix, "uploads/"):
		rest := strings.Split(strings.TrimPrefix(suffix, "uploads/"), "/")
		if len(rest) == 2 && rest[0] != "" && rest[1] == "content" {
			h.handleRakordaContent(w, r, rc, rest[0])
			return
		}
		if len(rest) == 1 && rest[0] != "" {
			h.handleRakordaDelete(w, r, rc, rest[0])
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleRakordaPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	var input struct {
		ScheduleID string `json:"schedule_id"`
		Date       string `json:"date"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.ScheduleID) == "" || !validISODate(input.Date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Rakorda.Preview(r.Context(), input.ScheduleID, input.Date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleRakordaUploads(w http.ResponseWriter, r *http.Request, rc requestContext) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		items, err := h.deps.Rakorda.List(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("date"), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if items == nil {
			items = []bast.RakordaUpload{}
		}
		writeData(w, http.StatusOK, items)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, (20<<20)+(1<<20))
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Berkas melebihi batas 20 MiB")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Berkas wajib dipilih", map[string]string{"file": "wajib diisi"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (20<<20)+1))
		if err != nil || len(data) > 20<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Berkas melebihi batas 20 MiB")
			return
		}
		item, err := h.deps.Rakorda.Upload(r.Context(), rc.principal, bast.RakordaUploadInput{ScheduleID: r.FormValue("schedule_id"), EventDate: r.FormValue("date"), OriginalName: header.Filename, Data: data}, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRakordaContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	content, err := h.deps.Rakorda.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Content-Type", content.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

func (h *Handler) handleRakordaDelete(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	if err := h.deps.Rakorda.Delete(r.Context(), rc.principal, id, scope, clientMeta(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
