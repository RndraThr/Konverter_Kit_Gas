package api

import (
	"bytes"
	"net/http"
	"strings"

	"konkit/internal/bast"
)

type pemeriksaanDocumentRequest struct {
	ScheduleID   string   `json:"schedule_id"`
	DocumentDate string   `json:"document_date"`
	FormCodes    []string `json:"form_codes"`
}

func (h *Handler) handleBASTPemeriksaan(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.Pemeriksaan == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "profile":
		h.handlePemeriksaanProfile(w, r, rc)
	case "summary":
		h.handlePemeriksaanSummary(w, r, rc)
	case "preview":
		h.handlePemeriksaanPreview(w, r, rc)
	case "finalize":
		h.handlePemeriksaanFinalize(w, r, rc)
	case "documents":
		h.handlePemeriksaanDocuments(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handlePemeriksaanContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handlePemeriksaanProfile(w http.ResponseWriter, r *http.Request, rc requestContext) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		programID := strings.TrimSpace(r.URL.Query().Get("program_id"))
		if programID == "" {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program wajib dipilih", map[string]string{"program_id": "wajib diisi"})
			return
		}
		result, err := h.deps.Pemeriksaan.Profile(r.Context(), programID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			ProgramID string                 `json:"program_id"`
			Forms     []bast.PemeriksaanForm `json:"forms"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Pemeriksaan.SaveProfile(r.Context(), rc.principal, bast.PemeriksaanProfile{ProgramID: input.ProgramID, Forms: input.Forms}, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handlePemeriksaanSummary(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.Pemeriksaan.Summary(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func decodePemeriksaanDocumentRequest(w http.ResponseWriter, r *http.Request) (pemeriksaanDocumentRequest, bool) {
	var input pemeriksaanDocumentRequest
	if !decodeJSON(w, r, &input) {
		return pemeriksaanDocumentRequest{}, false
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.DocumentDate = strings.TrimSpace(input.DocumentDate)
	if input.ScheduleID == "" || !validISODate(input.DocumentDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return pemeriksaanDocumentRequest{}, false
	}
	return input, true
}

func (h *Handler) handlePemeriksaanPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodePemeriksaanDocumentRequest(w, r)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Pemeriksaan.Preview(r.Context(), input.ScheduleID, input.DocumentDate, input.FormCodes, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handlePemeriksaanFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodePemeriksaanDocumentRequest(w, r)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Pemeriksaan.Finalize(r.Context(), rc.principal, input.ScheduleID, input.DocumentDate, input.FormCodes, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handlePemeriksaanDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Pemeriksaan.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handlePemeriksaanContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.Pemeriksaan.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}
