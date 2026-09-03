package api

import (
	"net/http"
	"strings"

	"konkit/internal/programs"
)

func (h *Handler) handleProgramSetup(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.Programs == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	id := ""
	if len(parts) == 2 {
		id = parts[1]
		if id == "" {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
	}
	switch parts[0] {
	case "regencies":
		h.handleRegencies(w, r, rc, id)
	case "programs":
		h.handlePrograms(w, r, rc, id)
	case "schedules":
		h.handleSchedules(w, r, rc, id)
	case "package-templates":
		h.handlePackageTemplates(w, r, rc, id)
	case "documentation-templates":
		h.handleDocumentationTemplates(w, r, rc, id)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleRegencies(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method == http.MethodGet && id == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		result, err := h.deps.Programs.ListRegencies(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, id) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.RegencyInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ID = id
	result, err := h.deps.Programs.SaveRegency(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func (h *Handler) handlePrograms(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method == http.MethodGet && id == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		result, err := h.deps.Programs.ListPrograms(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, id) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.ProgramInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ID = id
	result, err := h.deps.Programs.SaveProgram(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func (h *Handler) handleSchedules(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method == http.MethodGet && id == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		result, err := h.deps.Programs.ListSchedules(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, id) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.ScheduleInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ID = id
	result, err := h.deps.Programs.SaveSchedule(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func (h *Handler) handlePackageTemplates(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method == http.MethodGet && id == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		result, err := h.deps.Programs.ListPackageTemplates(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, id) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.PackageTemplateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ID = id
	result, err := h.deps.Programs.SavePackageTemplate(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func (h *Handler) handleDocumentationTemplates(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if r.Method == http.MethodGet && id == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		result, err := h.deps.Programs.ListDocumentationTemplates(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, id) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.DocumentationTemplateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ID = id
	result, err := h.deps.Programs.SaveDocumentationTemplate(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func programMutationMethod(w http.ResponseWriter, r *http.Request, id string) bool {
	if r.Method == http.MethodPost && id == "" {
		return true
	}
	if r.Method == http.MethodPatch && id != "" {
		return true
	}
	methodNotAllowed(w, http.MethodGet+", "+http.MethodPost+", "+http.MethodPatch)
	return false
}

func mutationStatus(r *http.Request) int {
	if r.Method == http.MethodPost {
		return http.StatusCreated
	}
	return http.StatusOK
}
