package api

import (
	"bytes"
	"net/http"
	"strings"

	"konkit/internal/bast"
)

// handleBASTGasElpiji melayani /api/v1/bast/gas-elpiji/*: penerimaan LPG per
// tanggal (summary, preview, draft, finalize, documents, cancel) dan BA gabungan
// (merged/preview, merged/finalize, merged/documents).
func (h *Handler) handleBASTGasElpiji(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	service := h.deps.GasElpiji
	if service == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "summary":
		if !h.gasElpijiGuard(w, r, rc, http.MethodGet, "bast.view") {
			return
		}
		scheduleID, ok := requiredScheduleQuery(w, r)
		if !ok {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := service.Summary(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "preview", "finalize":
		permission := "bast.view"
		if suffix == "finalize" {
			permission = "bast.manage"
		}
		if !h.gasElpijiGuard(w, r, rc, http.MethodPost, permission) {
			return
		}
		var input bast.GasElpijiInput
		if !decodeJSON(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.ScheduleID) == "" || !validISODate(strings.TrimSpace(input.DocumentDate)) {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if suffix == "preview" {
			result, err := service.Preview(r.Context(), input, scope)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
			return
		}
		result, err := service.Finalize(r.Context(), rc.principal, input, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "draft":
		if !h.gasElpijiGuard(w, r, rc, http.MethodPut, "bast.manage") {
			return
		}
		var input bast.GasElpijiInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := service.SaveDraft(r.Context(), rc.principal, input, scope); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]bool{"saved": true})
	case "documents":
		if !h.gasElpijiGuard(w, r, rc, http.MethodGet, "bast.view") {
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
		result, err := service.Documents(r.Context(), scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAggregateList(w, result)
	case "merged/preview", "merged/finalize":
		permission := "bast.view"
		if suffix == "merged/finalize" {
			permission = "bast.manage"
		}
		if !h.gasElpijiGuard(w, r, rc, http.MethodPost, permission) {
			return
		}
		var input struct {
			ScheduleID string `json:"schedule_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.ScheduleID) == "" {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if suffix == "merged/preview" {
			result, err := service.MergedPreview(r.Context(), input.ScheduleID, scope)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
			return
		}
		result, err := service.FinalizeMerged(r.Context(), rc.principal, input.ScheduleID, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "merged/documents":
		if !h.gasElpijiGuard(w, r, rc, http.MethodGet, "bast.view") {
			return
		}
		scheduleID, ok := requiredScheduleQuery(w, r)
		if !ok {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := service.MergedDocuments(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAggregateList(w, result)
	default:
		id, action, found := strings.Cut(strings.TrimPrefix(suffix, "documents/"), "/")
		if !strings.HasPrefix(suffix, "documents/") || !found || id == "" || strings.Contains(action, "/") {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		switch action {
		case "content":
			if !h.gasElpijiGuard(w, r, rc, http.MethodGet, "bast.view") {
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
			writePDF(w, "attachment", content.Filename, content.Reader)
		case "cancel":
			if !h.gasElpijiGuard(w, r, rc, http.MethodPost, "bast.manage") {
				return
			}
			scope, ok := h.regencyScope(w, r, rc.principal)
			if !ok {
				return
			}
			if err := service.Cancel(r.Context(), rc.principal, id, scope, clientMeta(r)); err != nil {
				writeServiceError(w, err)
				return
			}
			writeData(w, http.StatusOK, map[string]bool{"cancelled": true})
		default:
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		}
	}
}

func (h *Handler) gasElpijiGuard(w http.ResponseWriter, r *http.Request, rc requestContext, method, permission string) bool {
	if r.Method != method {
		methodNotAllowed(w, method)
		return false
	}
	return h.authorize(w, r, rc.principal, permission)
}

func requiredScheduleQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return "", false
	}
	return scheduleID, true
}

func writeAggregateList(w http.ResponseWriter, result []bast.AggregateDocument) {
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}
