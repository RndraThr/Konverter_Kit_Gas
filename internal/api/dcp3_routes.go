package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"konkit/internal/dcp3"
)

const maxDCP3RequestBody = 11 << 20

func (h *Handler) handleDCP3PreviewCreate(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.deps.DCP3 == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "dcp3.import") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDCP3RequestBody)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "workbook_too_large", "File DCP3 melebihi batas 10 MiB")
		} else {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Form upload DCP3 tidak valid")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	scheduleID := strings.TrimSpace(r.FormValue("schedule_id"))
	file, header, err := r.FormFile("file")
	if err != nil || scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan file DCP3 wajib diisi", map[string]string{"schedule_id": "Jadwal wajib dipilih", "file": "File DCP3 wajib dipilih"})
		return
	}
	defer file.Close()
	headerRow := 1
	if raw := strings.TrimSpace(r.FormValue("header_row")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Baris header tidak valid", map[string]string{"header_row": "Gunakan angka baris header"})
			return
		}
		headerRow = parsed
	}
	if strings.ToLower(filepath.Ext(header.Filename)) != ".xlsx" {
		writeError(w, http.StatusUnsupportedMediaType, "workbook_type_invalid", "Gunakan file Excel berformat .xlsx")
		return
	}
	signature := make([]byte, 4)
	if _, err := io.ReadFull(file, signature); err != nil || string(signature) != "PK\x03\x04" {
		writeError(w, http.StatusUnsupportedMediaType, "workbook_type_invalid", "Isi file bukan workbook .xlsx")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusBadRequest, "workbook_invalid", "File DCP3 tidak dapat dibaca")
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.Preview(r.Context(), rc.principal, scheduleID, filepath.Base(header.Filename), file, clientMeta(r), scope, headerRow)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) handleDCP3RawPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.deps.DCP3 == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "dcp3.import") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDCP3RequestBody)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "workbook_too_large", "File DCP3 melebihi batas 10 MiB")
		} else {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Form upload DCP3 tidak valid")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "File DCP3 wajib dipilih", map[string]string{"file": "File DCP3 wajib dipilih"})
		return
	}
	defer file.Close()
	rows, err := h.deps.DCP3.RawPreview(r.Context(), file)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, rows)
}

func (h *Handler) handleDCP3Preview(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.DCP3 == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "dcp3.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.GetPreview(r.Context(), strings.TrimSpace(id), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDCP3Import(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.deps.DCP3 == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "dcp3.import") {
		return
	}
	var input struct {
		BatchID string       `json:"batch_id"`
		Mapping dcp3.Mapping `json:"mapping"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DCP3.Commit(r.Context(), rc.principal, strings.TrimSpace(input.BatchID), input.Mapping, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}
