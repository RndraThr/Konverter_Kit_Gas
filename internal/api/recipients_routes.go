package api

import (
	"net/http"
	"strings"

	"konkit/internal/recipients"
)

func (h *Handler) handleRecipients(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "recipients.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		page, err := h.deps.Recipients.List(r.Context(), recipients.Filter{
			Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 20),
			Search: r.URL.Query().Get("search"), RegencyID: r.URL.Query().Get("regency_id"),
			ProgramID: r.URL.Query().Get("program_id"), ProgramType: r.URL.Query().Get("program_type"),
			AllocationStatus: r.URL.Query().Get("allocation_status"), DistributionStatus: r.URL.Query().Get("distribution_status"),
		}, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, page)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		var input recipients.CreateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Recipients.Create(r.Context(), rc.principal, input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRecipientStats(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "recipients.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	stats, err := h.deps.Recipients.Stats(r.Context(), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, stats)
}

func (h *Handler) handleRecipient(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Recipients == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodPatch {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		var input recipients.UpdateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Recipients.Update(r.Context(), rc.principal, parts[0], input, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Recipients.Cancel(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "restore" && r.Method == http.MethodPost {
		if !h.authorize(w, r, rc.principal, "recipients.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Recipients.Restore(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 1 {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}
