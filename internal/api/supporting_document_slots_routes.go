package api

import (
	"net/http"
	"strings"
)

func (h *Handler) handleSupportingDocumentSlots(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "activities.view") {
			return
		}
		scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
		if scheduleID == "" {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "schedule_id wajib diisi", map[string]string{"schedule_id": "wajib diisi"})
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		items, err := h.deps.Activities.ListSlots(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, items)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		var input struct {
			ScheduleID string `json:"schedule_id"`
			Name       string `json:"name"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		item, err := h.deps.Activities.CreateSlot(r.Context(), rc.principal, input.ScheduleID, input.Name, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleSupportingDocumentSlotItem(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	if !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	item, err := h.deps.Activities.PatchSlot(r.Context(), rc.principal, id, input.Name, input.IsActive, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, item)
}
