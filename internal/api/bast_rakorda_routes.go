package api

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"konkit/internal/activities"
	"konkit/internal/bast"
)

// handleBASTRakorda melayani rute daftar hadir yang sama untuk setiap jenis
// (bast/rakorda/..., bast/sosialisasi/...); service menentukan jenisnya.
func (h *Handler) handleBASTRakorda(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService, suffix string) {
	if service == nil {
		writeUnavailable(w)
		return
	}
	switch {
	case suffix == "preview":
		h.handleRakordaPreview(w, r, rc, service)
	case suffix == "uploads":
		h.handleRakordaUploads(w, r, rc, service)
	case suffix == "dates":
		h.handleRakordaDates(w, r, rc, service)
	case strings.HasPrefix(suffix, "uploads/"):
		rest := strings.Split(strings.TrimPrefix(suffix, "uploads/"), "/")
		if len(rest) == 2 && rest[0] != "" && rest[1] == "content" {
			h.handleRakordaContent(w, r, rc, service, rest[0])
			return
		}
		if len(rest) == 1 && rest[0] != "" {
			h.handleRakordaDelete(w, r, rc, service, rest[0])
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleRakordaPreview(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService) {
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
	result, err := service.Preview(r.Context(), input.ScheduleID, input.Date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleRakordaUploads(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		items, err := service.List(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("date"), scope)
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
		// Pages scanned on the phone ("page" parts) are bound into one PDF
		// here, like a Dokumen Pendukung scan; a single "file" is stored as is.
		r.Body = http.MaxBytesReader(w, r.Body, (40<<20)+(1<<20))
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Berkas melebihi batas 20 MiB")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if pages := r.MultipartForm.File["page"]; len(pages) > 0 {
			data, ok := assembleRakordaScan(w, pages)
			if !ok {
				return
			}
			item, err := service.Upload(r.Context(), rc.principal, bast.RakordaUploadInput{ScheduleID: r.FormValue("schedule_id"), EventDate: r.FormValue("date"), OriginalName: "pindaian.pdf", Data: data}, scope, clientMeta(r))
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeData(w, http.StatusCreated, item)
			return
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
		item, err := service.Upload(r.Context(), rc.principal, bast.RakordaUploadInput{ScheduleID: r.FormValue("schedule_id"), EventDate: r.FormValue("date"), OriginalName: header.Filename, Data: data}, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRakordaContent(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService, id string) {
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
	content, err := service.Open(r.Context(), id, scope)
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

func (h *Handler) handleRakordaDelete(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService, id string) {
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
	if err := service.Delete(r.Context(), rc.principal, id, scope, clientMeta(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRakordaDates mengembalikan tanggal distribusi dan jumlah peserta untuk
// daftar hadir Training (GET ?schedule_id=).
func (h *Handler) handleRakordaDates(w http.ResponseWriter, r *http.Request, rc requestContext, service RakordaService) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := service.Dates(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.TrainingDate{}
	}
	writeData(w, http.StatusOK, result)
}

// assembleRakordaScan binds the scanned pages into one PDF of at most 20 MiB.
func assembleRakordaScan(w http.ResponseWriter, pages []*multipart.FileHeader) ([]byte, bool) {
	readers := make([]io.Reader, 0, len(pages))
	for _, header := range pages {
		file, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Halaman pindaian tidak dapat dibaca")
			return nil, false
		}
		defer file.Close()
		readers = append(readers, file)
	}
	data, err := activities.AssembleScanPDF(readers)
	switch {
	case errors.Is(err, activities.ErrScanTooManyPages):
		writeError(w, http.StatusBadRequest, "too_many_pages", "Maksimal 30 halaman per dokumen")
	case errors.Is(err, activities.ErrMediaTypeInvalid):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Halaman pindaian harus JPG atau PNG")
	case errors.Is(err, activities.ErrFileTooLarge) || (err == nil && len(data) > 20<<20):
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Hasil pindaian melebihi batas 20 MiB")
	case err != nil:
		writeServiceError(w, err)
	default:
		return data, true
	}
	return nil, false
}
