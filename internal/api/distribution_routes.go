package api

import (
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/distribution"
)

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
	result, err := h.deps.Distribution.Search(r.Context(), scheduleID, query, limit)
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
		result, err := h.deps.Distribution.GetWorkspace(r.Context(), parts[0])
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
		result, err := h.deps.Distribution.SaveDraft(r.Context(), rc.principal, parts[0], input, clientMeta(r))
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
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}
