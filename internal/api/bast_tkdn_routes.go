package api

import (
	"bytes"
	"net/http"
	"strings"

	"konkit/internal/bast"
)

func (h *Handler) handleBASTTKDN(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.TKDN == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "profile":
		h.handleTKDNProfile(w, r, rc)
	case "summary":
		h.handleTKDNSummary(w, r, rc)
	case "preview":
		h.handleTKDNPreview(w, r, rc)
	case "finalize":
		h.handleTKDNFinalize(w, r, rc)
	case "documents":
		h.handleTKDNDocuments(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleTKDNContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

// handleTKDNProfile membaca (GET ?program_id=) atau menyimpan (PUT) daftar
// TKDN satu program/tender.
func (h *Handler) handleTKDNProfile(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
		result, err := h.deps.TKDN.Profile(r.Context(), programID)
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
			ProgramID string         `json:"program_id"`
			Rows      []bast.TKDNRow `json:"rows"`
			TotalTKDN float64        `json:"total_tkdn"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.TKDN.SaveProfile(r.Context(), rc.principal, bast.TKDNProfile{ProgramID: input.ProgramID, Rows: input.Rows, TotalTKDN: input.TotalTKDN}, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleTKDNSummary(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.TKDN.Summary(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleTKDNPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodeClosingDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.TKDN.Preview(r.Context(), input.ScheduleID, input.DocumentDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleTKDNFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodeClosingDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.TKDN.Finalize(r.Context(), rc.principal, input.ScheduleID, input.DocumentDate, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleTKDNDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.TKDN.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleTKDNContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.TKDN.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}
