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
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if parts[0] == "programs" && len(parts) > 2 {
		h.handleProgramNested(w, r, rc, parts[1:])
		return
	}
	if len(parts) > 2 {
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
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Programs.ListRegencies(r.Context(), scope)
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
		if !h.authorizeAny(w, r, rc.principal, "programs.view", "bast.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Programs.ListSchedules(r.Context(), scope)
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
	mutationScope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Programs.SaveSchedule(r.Context(), rc.principal, input, mutationScope, clientMeta(r))
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

// handleProgramNested routes the sub-paths nested under programs/{programID}/...
// (zones, assignments) that don't fit the generic maximum-two-parts resource parser.
func (h *Handler) handleProgramNested(w http.ResponseWriter, r *http.Request, rc requestContext, parts []string) {
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	programID := parts[0]
	switch parts[1] {
	case "zones":
		zoneID := ""
		switch len(parts) {
		case 2:
		case 3:
			if parts[2] == "" {
				writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
				return
			}
			zoneID = parts[2]
		default:
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		h.handleProgramZones(w, r, rc, programID, zoneID)
	case "assignments":
		if len(parts) != 3 || parts[2] == "" {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		h.handleRegencyAssignment(w, r, rc, programID, parts[2])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleProgramZones(w http.ResponseWriter, r *http.Request, rc requestContext, programID, zoneID string) {
	if r.Method == http.MethodGet && zoneID == "" {
		if !h.authorize(w, r, rc.principal, "programs.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		result, err := h.deps.Programs.ListZones(r.Context(), programID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !programMutationMethod(w, r, zoneID) || !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	var input programs.ZoneInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ProgramID = programID
	input.ID = zoneID
	result, err := h.deps.Programs.SaveZone(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, mutationStatus(r), result)
}

func (h *Handler) handleRegencyAssignment(w http.ResponseWriter, r *http.Request, rc requestContext, programID, regencyID string) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	if !h.authorize(w, r, rc.principal, "programs.manage") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	var input programs.RegencyAssignmentInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ProgramID = programID
	input.RegencyID = regencyID
	result, err := h.deps.Programs.AssignRegency(r.Context(), rc.principal, input, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
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
