package api

import (
	"errors"
	"net/http"
	"strings"

	"konkit/internal/administration"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/dcp3"
	"konkit/internal/distribution"
	apphealth "konkit/internal/health"
	"konkit/internal/profile"
	"konkit/internal/programs"
	"konkit/internal/reports"
	"konkit/internal/settings"
)

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request, rc requestContext) {
	switch r.Method {
	case http.MethodGet:
		permissions, err := h.deps.Auth.Permissions(r.Context(), rc.principal)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		data := map[string]any{
			"id": rc.principal.UserID, "full_name": rc.principal.FullName,
			"username": rc.principal.Username, "email": rc.principal.Email,
			"roles": rc.principal.Roles, "permissions": permissions,
		}
		if h.deps.Profile != nil {
			result, profileErr := h.deps.Profile.Get(r.Context(), rc.principal.UserID)
			if profileErr != nil {
				writeServiceError(w, profileErr)
				return
			}
			data["full_name"], data["username"], data["email"] = result.FullName, result.Username, result.Email
			data["roles"], data["is_active"], data["last_login_at"] = result.Roles, result.IsActive, result.LastLoginAt
			data["created_at"], data["updated_at"] = result.CreatedAt, result.UpdatedAt
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": data,
			"meta": map[string]string{"csrf_token": csrfToken(h, rc.token)},
		})
	case http.MethodPatch:
		if h.deps.Profile == nil {
			writeUnavailable(w)
			return
		}
		var input profile.UpdateInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Profile.Update(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
	}
}

func (h *Handler) handleMyPassword(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	if h.deps.Profile == nil {
		writeUnavailable(w)
		return
	}
	var input profile.PasswordInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.deps.Profile.ChangePassword(r.Context(), rc.principal, rc.token, input, clientMeta(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleUsers(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "users.view") {
			return
		}
		page, err := h.deps.Administration.ListUsers(r.Context(), administration.UserFilter{
			Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 20), Search: r.URL.Query().Get("search"),
			Active: boolQuery(r, "active"), RoleCode: r.URL.Query().Get("role"),
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, page)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "users.manage") {
			return
		}
		var input administration.CreateUserInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Administration.CreateUser(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleDashboardSummary(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "dashboard.view") {
		return
	}
	counts, err := h.deps.Administration.UserCounts(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	roles, err := h.deps.Administration.ListRoles(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	summary := map[string]any{"users": counts.Total, "active_users": counts.Active, "inactive_users": counts.Inactive, "roles": len(roles)}
	if h.deps.Health != nil {
		report := h.deps.Health.Check(r.Context())
		summary["system"] = map[string]any{"status": report.Status, "database": map[string]any{"status": report.Database.Status}}
	}
	if h.deps.Profile != nil {
		if current, profileErr := h.deps.Profile.Get(r.Context(), rc.principal.UserID); profileErr == nil {
			summary["current_user"] = map[string]any{"full_name": current.FullName, "last_login_at": current.LastLoginAt}
		}
	}
	if h.deps.Audit != nil {
		if recent, auditErr := h.deps.Audit.List(r.Context(), audit.Filter{Page: 1, PageSize: 5}); auditErr == nil {
			items := make([]map[string]any, 0, len(recent.Items))
			for _, entry := range recent.Items {
				items = append(items, map[string]any{"id": entry.ID, "action": entry.Action, "actor_name": entry.ActorName, "created_at": entry.CreatedAt})
			}
			summary["recent_activity"] = items
		}
	}
	writeData(w, http.StatusOK, summary)
}

func (h *Handler) handleUser(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) == 2 && parts[1] == "password" {
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		if !h.authorize(w, r, rc.principal, "users.manage") {
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := h.deps.Administration.SetPassword(r.Context(), rc.principal, parts[0], input.Password, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "users.view") {
			return
		}
		result, err := h.deps.Administration.GetUser(r.Context(), parts[0])
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
		return
	}
	if !h.authorize(w, r, rc.principal, "users.manage") {
		return
	}
	var input administration.UpdateUserInput
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.deps.Administration.UpdateUser(r.Context(), rc.principal, parts[0], input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleRoles(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "roles.view") {
			return
		}
		result, err := h.deps.Administration.ListRoles(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "roles.manage") {
			return
		}
		var input administration.RoleInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Administration.CreateRole(r.Context(), rc.principal, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRoleOptions(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorizeAny(w, r, rc.principal, "users.view", "users.manage") {
		return
	}
	roles, err := h.deps.Administration.ListRoles(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	options := make([]administration.RoleRef, 0, len(roles))
	for _, role := range roles {
		options = append(options, administration.RoleRef{ID: role.ID, Code: role.Code, Name: role.Name})
	}
	writeData(w, http.StatusOK, options)
}

func (h *Handler) handleRole(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	if r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "roles.view") {
			return
		}
		result, err := h.deps.Administration.GetRole(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
		return
	}
	if !h.authorize(w, r, rc.principal, "roles.manage") {
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var input administration.RoleInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Administration.UpdateRole(r.Context(), rc.principal, id, input, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodDelete:
		if err := h.deps.Administration.DeleteRole(r.Context(), rc.principal, id, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) handlePermissions(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "roles.view") {
		return
	}
	result, err := h.deps.Administration.ListPermissions(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Settings == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "settings.view") {
			return
		}
		result, err := h.deps.Settings.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodPatch:
		if !h.authorize(w, r, rc.principal, "settings.manage") {
			return
		}
		var input struct {
			Values map[string]string `json:"values"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Settings.Update(r.Context(), rc.principal, input.Values, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
	}
}

func (h *Handler) handleSystemHealth(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Health == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "health.view") {
		return
	}
	report := h.deps.Health.Check(r.Context())
	status := http.StatusOK
	if report.Status != apphealth.StatusHealthy {
		status = http.StatusServiceUnavailable
	}
	writeData(w, status, report)
}

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if h.deps.Audit == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "audit.view") {
		return
	}
	result, err := h.deps.Audit.List(r.Context(), audit.Filter{
		Page: intQuery(r, "page", 1), PageSize: intQuery(r, "page_size", 20), Action: r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resource_type"), ActorUserID: r.URL.Query().Get("actor_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func csrfToken(h *Handler, rawToken string) string {
	return auth.CSRFToken(h.deps.SessionSecret, rawToken)
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Metode tidak didukung")
}

func writeUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Layanan belum siap")
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, profile.ErrNotFound), errors.Is(err, administration.ErrNotFound), errors.Is(err, programs.ErrNotFound), errors.Is(err, dcp3.ErrPreviewNotFound), errors.Is(err, distribution.ErrAllocationNotFound), errors.Is(err, distribution.ErrMediaNotFound), errors.Is(err, reports.ErrScheduleNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	case errors.Is(err, profile.ErrIdentityInUse), errors.Is(err, administration.ErrIdentityInUse):
		writeFieldError(w, http.StatusConflict, "conflict", "Data sudah digunakan", map[string]string{"username": err.Error(), "email": err.Error()})
	case errors.Is(err, administration.ErrRoleCodeInUse):
		writeFieldError(w, http.StatusConflict, "conflict", "Data sudah digunakan", map[string]string{"code": err.Error()})
	case errors.Is(err, programs.ErrDocumentCodeInUse), errors.Is(err, programs.ErrCodeInUse):
		writeFieldError(w, http.StatusConflict, "conflict", "Kode sudah digunakan", map[string]string{"code": err.Error()})
	case errors.Is(err, dcp3.ErrDuplicateImport):
		writeError(w, http.StatusConflict, "duplicate_import", "File DCP3 ini sudah pernah diunggah pada jadwal yang sama")
	case errors.Is(err, dcp3.ErrImportState):
		writeError(w, http.StatusConflict, "import_state_invalid", "Batch DCP3 tidak dapat diimport pada status saat ini")
	case errors.Is(err, distribution.ErrIdentifierConflict):
		writeError(w, http.StatusConflict, "identifier_conflict", "NIK atau nomor kartu sudah digunakan penerima lain")
	case errors.Is(err, distribution.ErrIdentityIncomplete):
		writeError(w, http.StatusConflict, "identity_incomplete", "Identitas wajib penerima belum lengkap")
	case errors.Is(err, distribution.ErrDocumentationIncomplete):
		writeError(w, http.StatusConflict, "documentation_incomplete", "Dokumentasi wajib belum lengkap")
	case errors.Is(err, distribution.ErrPreviouslyReceived):
		writeError(w, http.StatusConflict, "previously_received", "Penerima tercatat sudah menerima paket")
	case errors.Is(err, distribution.ErrAlreadyCompleted):
		writeError(w, http.StatusConflict, "already_completed", "Distribusi sudah pernah diselesaikan")
	case errors.Is(err, dcp3.ErrWorkbookTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "workbook_too_large", err.Error())
	case errors.Is(err, distribution.ErrMediaTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", err.Error())
	case errors.Is(err, administration.ErrLastSuperAdmin), errors.Is(err, administration.ErrSelfDeactivation), errors.Is(err, administration.ErrRoleInUse), errors.Is(err, administration.ErrSystemRole):
		writeError(w, http.StatusConflict, "operation_rejected", err.Error())
	case errors.Is(err, profile.ErrFullNameInvalid), errors.Is(err, profile.ErrUsernameInvalid), errors.Is(err, profile.ErrEmailInvalid),
		errors.Is(err, profile.ErrCurrentPassword), errors.Is(err, profile.ErrPasswordTooShort), errors.Is(err, profile.ErrPasswordUnchanged),
		errors.Is(err, administration.ErrInvalidInput), errors.Is(err, administration.ErrPasswordTooShort), errors.Is(err, administration.ErrRoleNotFound),
		errors.Is(err, administration.ErrPermissionNotFound), errors.Is(err, administration.ErrRoleCodeInvalid), errors.Is(err, settings.ErrInvalidSetting),
		errors.Is(err, programs.ErrInvalidInput), errors.Is(err, programs.ErrDocumentCodeInvalid), errors.Is(err, programs.ErrProgramTypeInvalid),
		errors.Is(err, programs.ErrScheduleDatesInvalid), errors.Is(err, programs.ErrTemplateSlotInvalid):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), validationFields(err))
	case errors.Is(err, distribution.ErrScheduleRequired), errors.Is(err, distribution.ErrQueryRequired), errors.Is(err, distribution.ErrQueryTooShort),
		errors.Is(err, distribution.ErrNIKInvalid), errors.Is(err, distribution.ErrIdentityChangeReasonRequired):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), validationFields(err))
	case errors.Is(err, reports.ErrScheduleRequired), errors.Is(err, reports.ErrFilterInvalid):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", err.Error(), map[string]string{"request": err.Error()})
	case errors.Is(err, distribution.ErrMediaTypeInvalid), errors.Is(err, distribution.ErrMediaSourceInvalid), errors.Is(err, distribution.ErrMediaLocationRequired),
		errors.Is(err, distribution.ErrMediaCapturedAtRequired), errors.Is(err, distribution.ErrMediaLimitReached):
		writeFieldError(w, http.StatusBadRequest, "media_invalid", err.Error(), map[string]string{"file": err.Error()})
	case errors.Is(err, dcp3.ErrMappingInvalid), errors.Is(err, dcp3.ErrTooManyRows), errors.Is(err, dcp3.ErrTooManyColumns),
		errors.Is(err, dcp3.ErrHeadersInvalid), errors.Is(err, dcp3.ErrWorkbookInvalid):
		writeFieldError(w, http.StatusBadRequest, "dcp3_invalid", err.Error(), map[string]string{"file": err.Error()})
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan pada server")
	}
}

func validationFields(err error) map[string]string {
	switch {
	case errors.Is(err, profile.ErrFullNameInvalid):
		return map[string]string{"full_name": err.Error()}
	case errors.Is(err, profile.ErrUsernameInvalid):
		return map[string]string{"username": err.Error()}
	case errors.Is(err, profile.ErrEmailInvalid):
		return map[string]string{"email": err.Error()}
	case errors.Is(err, profile.ErrCurrentPassword):
		return map[string]string{"current_password": err.Error()}
	case errors.Is(err, profile.ErrPasswordTooShort), errors.Is(err, profile.ErrPasswordUnchanged):
		return map[string]string{"new_password": err.Error()}
	case errors.Is(err, administration.ErrPasswordTooShort):
		return map[string]string{"password": err.Error()}
	case errors.Is(err, administration.ErrRoleCodeInvalid):
		return map[string]string{"code": err.Error()}
	case errors.Is(err, administration.ErrRoleNotFound):
		return map[string]string{"role_ids": err.Error()}
	case errors.Is(err, administration.ErrPermissionNotFound):
		return map[string]string{"permission_codes": err.Error()}
	case errors.Is(err, settings.ErrInvalidSetting):
		return map[string]string{"values": err.Error()}
	default:
		return map[string]string{"request": err.Error()}
	}
}
