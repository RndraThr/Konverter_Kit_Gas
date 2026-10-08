package api

import (
	"net/http"
	"strings"

	"konkit/internal/bast"
)

// handleBASTItems melayani merk & No. PO per jadwal (/bast/items/schedule)
// dan No. PO per zona (/bast/items/zone-po).
func (h *Handler) handleBASTItems(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.Items == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "schedule":
		h.handleScheduleItems(w, r, rc)
	case "zone-po":
		h.handleZonePO(w, r, rc)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleZonePO(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
		result, err := h.deps.Items.ProgramZonePO(r.Context(), programID)
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
			ProgramID string             `json:"program_id"`
			ZoneID    string             `json:"zone_id"`
			PONumbers []bast.ZonePOInput `json:"po_numbers"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Items.SaveZonePO(r.Context(), rc.principal, input.ProgramID, input.ZoneID, input.PONumbers, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleScheduleItems(w http.ResponseWriter, r *http.Request, rc requestContext) {
	switch r.Method {
	case http.MethodGet:
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
		result, err := h.deps.Items.ScheduleItems(r.Context(), scheduleID, scope)
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
			ScheduleID string            `json:"schedule_id"`
			Selection  map[string]string `json:"selection"`
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
		result, err := h.deps.Items.SaveScheduleSelection(r.Context(), rc.principal, input.ScheduleID, input.Selection, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}
