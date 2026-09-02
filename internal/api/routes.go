package api

import (
	"errors"
	"net/http"
	"strings"

	"konkit/internal/administration"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/profile"
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
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"id": rc.principal.UserID, "full_name": rc.principal.FullName,
				"username": rc.principal.Username, "email": rc.principal.Email,
				"roles": rc.principal.Roles, "permissions": permissions,
			},
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
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
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

func (h *Handler) handleRole(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
	if h.deps.Administration == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "roles.manage") {
		return
	}
	switch r.Method {
	case http.MethodPut:
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
		methodNotAllowed(w, http.MethodPut+", "+http.MethodDelete)
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
	writeData(w, http.StatusOK, h.deps.Health.Check(r.Context()))
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
	case errors.Is(err, profile.ErrNotFound), errors.Is(err, administration.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	case errors.Is(err, profile.ErrIdentityInUse), errors.Is(err, administration.ErrIdentityInUse), errors.Is(err, administration.ErrRoleCodeInUse):
		writeError(w, http.StatusConflict, "conflict", "Data sudah digunakan")
	case errors.Is(err, administration.ErrLastSuperAdmin), errors.Is(err, administration.ErrSelfDeactivation), errors.Is(err, administration.ErrRoleInUse), errors.Is(err, administration.ErrSystemRole):
		writeError(w, http.StatusConflict, "operation_rejected", err.Error())
	case errors.Is(err, profile.ErrFullNameInvalid), errors.Is(err, profile.ErrUsernameInvalid), errors.Is(err, profile.ErrEmailInvalid),
		errors.Is(err, profile.ErrCurrentPassword), errors.Is(err, profile.ErrPasswordTooShort), errors.Is(err, profile.ErrPasswordUnchanged),
		errors.Is(err, administration.ErrInvalidInput), errors.Is(err, administration.ErrPasswordTooShort), errors.Is(err, administration.ErrRoleNotFound),
		errors.Is(err, administration.ErrPermissionNotFound), errors.Is(err, administration.ErrRoleCodeInvalid), errors.Is(err, settings.ErrInvalidSetting):
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan pada server")
	}
}
