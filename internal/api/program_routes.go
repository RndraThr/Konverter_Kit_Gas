package api

import (
	"io"
	"mime"
	"net/http"
	"strconv"
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
	case "document-profiles":
		h.handleDocumentProfiles(w, r, rc, programID, parts[2:])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleDocumentProfiles(w http.ResponseWriter, r *http.Request, rc requestContext, programID string, rest []string) {
	service, ok := h.deps.Programs.(DocumentProfileService)
	if !ok {
		writeUnavailable(w)
		return
	}
	if len(rest) == 0 {
		if r.Method == http.MethodGet {
			if !h.authorizeAny(w, r, rc.principal, "programs.view", "bast.view") {
				return
			}
			items, err := service.ListDocumentProfiles(r.Context(), programID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeData(w, http.StatusOK, items)
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
			return
		}
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		var input programs.DocumentProfileInput
		if !decodeJSON(w, r, &input) {
			return
		}
		input.ProgramID = programID
		item, err := service.SaveDocumentProfile(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
		return
	}
	profileID := rest[0]
	if len(rest) == 1 {
		if r.Method != http.MethodPatch {
			methodNotAllowed(w, http.MethodPatch)
			return
		}
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		var input programs.DocumentProfileInput
		if !decodeJSON(w, r, &input) {
			return
		}
		input.ID = profileID
		input.ProgramID = programID
		item, err := service.SaveDocumentProfile(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, item)
		return
	}
	if len(rest) == 2 && rest[1] == "publish" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		item, err := service.PublishDocumentProfile(r.Context(), rc.principal, programID, profileID, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, item)
		return
	}
	if len(rest) >= 2 && rest[1] == "logos" {
		h.handleDocumentLogos(w, r, rc, service, profileID, rest[2:])
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func (h *Handler) handleDocumentLogos(w http.ResponseWriter, r *http.Request, rc requestContext, service DocumentProfileService, profileID string, rest []string) {
	if len(rest) == 0 {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Logo melebihi batas 10 MiB")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Logo wajib dipilih", map[string]string{"file": "Logo wajib dipilih"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
		if err != nil || len(data) > 10<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Logo melebihi batas 10 MiB")
			return
		}
		sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
		maxWidth, _ := strconv.ParseFloat(r.FormValue("max_width_mm"), 64)
		maxHeight, _ := strconv.ParseFloat(r.FormValue("max_height_mm"), 64)
		item, err := service.UploadDocumentLogo(r.Context(), rc.principal, programs.DocumentLogoInput{ProfileVersionID: profileID, SlotCode: r.FormValue("slot_code"), OriginalFilename: header.Filename, Data: data, SortOrder: sortOrder, MaxWidthMM: maxWidth, MaxHeightMM: maxHeight}, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, item)
		return
	}
	logoID := rest[0]
	if len(rest) == 2 && rest[1] == "content" && r.Method == http.MethodGet {
		if !h.authorizeAny(w, r, rc.principal, "programs.view", "bast.view") {
			return
		}
		content, err := service.OpenDocumentLogo(r.Context(), profileID, logoID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", content.MimeType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
		return
	}
	if len(rest) == 1 && r.Method == http.MethodPatch {
		if !h.authorize(w, r, rc.principal, "programs.manage") {
			return
		}
		var input programs.DocumentLogoUpdateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		input.ID = logoID
		input.ProfileVersionID = profileID
		item, err := service.UpdateDocumentLogo(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, item)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
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
