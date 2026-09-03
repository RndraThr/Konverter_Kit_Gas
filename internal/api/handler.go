package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/administration"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/dcp3"
	apphealth "konkit/internal/health"
	"konkit/internal/profile"
	"konkit/internal/programs"
	"konkit/internal/settings"
)

const maxRequestBody = 1 << 20

type AuthService interface {
	Authenticate(context.Context, string) (auth.Principal, error)
	Can(context.Context, auth.Principal, string) (bool, error)
	Permissions(context.Context, auth.Principal) ([]string, error)
}

type ProfileService interface {
	Get(context.Context, string) (profile.Profile, error)
	Update(context.Context, auth.Principal, profile.UpdateInput, auth.ClientMeta) (profile.Profile, error)
	ChangePassword(context.Context, auth.Principal, string, profile.PasswordInput, auth.ClientMeta) error
}

type AdministrationService interface {
	ListUsers(context.Context, administration.UserFilter) (administration.UserPage, error)
	UserCounts(context.Context) (administration.UserCounts, error)
	GetUser(context.Context, string) (administration.UserDetail, error)
	CreateUser(context.Context, auth.Principal, administration.CreateUserInput, auth.ClientMeta) (administration.UserDetail, error)
	UpdateUser(context.Context, auth.Principal, string, administration.UpdateUserInput, auth.ClientMeta) (administration.UserDetail, error)
	SetPassword(context.Context, auth.Principal, string, string, auth.ClientMeta) error
	ListRoles(context.Context) ([]administration.Role, error)
	GetRole(context.Context, string) (administration.Role, error)
	ListPermissions(context.Context) ([]administration.PermissionGroup, error)
	CreateRole(context.Context, auth.Principal, administration.RoleInput, auth.ClientMeta) (administration.Role, error)
	UpdateRole(context.Context, auth.Principal, string, administration.RoleInput, auth.ClientMeta) (administration.Role, error)
	DeleteRole(context.Context, auth.Principal, string, auth.ClientMeta) error
}

type SettingsService interface {
	List(context.Context) ([]settings.Setting, error)
	Update(context.Context, auth.Principal, map[string]string, auth.ClientMeta) ([]settings.Setting, error)
}

type HealthService interface {
	Check(context.Context) apphealth.Report
}

type AuditService interface {
	List(context.Context, audit.Filter) (audit.Page, error)
}

type ProgramSetupService interface {
	ListRegencies(context.Context) ([]programs.Regency, error)
	SaveRegency(context.Context, auth.Principal, programs.RegencyInput, auth.ClientMeta) (programs.Regency, error)
	ListPrograms(context.Context) ([]programs.Program, error)
	SaveProgram(context.Context, auth.Principal, programs.ProgramInput, auth.ClientMeta) (programs.Program, error)
	ListSchedules(context.Context) ([]programs.Schedule, error)
	SaveSchedule(context.Context, auth.Principal, programs.ScheduleInput, auth.ClientMeta) (programs.Schedule, error)
	ListPackageTemplates(context.Context) ([]programs.PackageTemplate, error)
	SavePackageTemplate(context.Context, auth.Principal, programs.PackageTemplateInput, auth.ClientMeta) (programs.PackageTemplate, error)
	ListDocumentationTemplates(context.Context) ([]programs.DocumentationTemplate, error)
	SaveDocumentationTemplate(context.Context, auth.Principal, programs.DocumentationTemplateInput, auth.ClientMeta) (programs.DocumentationTemplate, error)
}

type DCP3Service interface {
	Preview(context.Context, auth.Principal, string, string, io.Reader, auth.ClientMeta) (dcp3.ImportPreview, error)
	GetPreview(context.Context, string) (dcp3.ImportPreview, error)
	Commit(context.Context, auth.Principal, string, dcp3.Mapping, auth.ClientMeta) (dcp3.ImportResult, error)
}

type Dependencies struct {
	Auth           AuthService
	Profile        ProfileService
	Administration AdministrationService
	Settings       SettingsService
	Health         HealthService
	Audit          AuditService
	Programs       ProgramSetupService
	DCP3           DCP3Service
	SessionSecret  []byte
}

type Handler struct {
	deps Dependencies
}

type requestContext struct {
	principal auth.Principal
	token     string
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func NewHandler(deps Dependencies) http.Handler {
	return &Handler{deps: deps}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/health" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Metode tidak didukung")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}

	rc, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if isMutation(r.Method) && !auth.VerifyCSRF(h.deps.SessionSecret, rc.token, r.Header.Get("X-CSRF-Token")) {
		writeError(w, http.StatusForbidden, "csrf_invalid", "Token keamanan tidak valid")
		return
	}
	h.routeProtected(w, r, rc)
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (requestContext, bool) {
	if h.deps.Auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Layanan autentikasi belum siap")
		return requestContext{}, false
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sesi login diperlukan")
		return requestContext{}, false
	}
	principal, err := h.deps.Auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sesi login tidak valid")
		return requestContext{}, false
	}
	return requestContext{principal: principal, token: cookie.Value}, true
}

func (h *Handler) routeProtected(w http.ResponseWriter, r *http.Request, rc requestContext) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	switch {
	case path == "me":
		h.handleMe(w, r, rc)
	case path == "me/password":
		h.handleMyPassword(w, r, rc)
	case path == "dashboard/summary":
		h.handleDashboardSummary(w, r, rc)
	case path == "admin/users":
		h.handleUsers(w, r, rc)
	case strings.HasPrefix(path, "admin/users/"):
		h.handleUser(w, r, rc, strings.TrimPrefix(path, "admin/users/"))
	case path == "admin/roles":
		h.handleRoles(w, r, rc)
	case path == "admin/role-options":
		h.handleRoleOptions(w, r, rc)
	case strings.HasPrefix(path, "admin/roles/"):
		h.handleRole(w, r, rc, strings.TrimPrefix(path, "admin/roles/"))
	case path == "admin/permissions":
		h.handlePermissions(w, r, rc)
	case path == "system/settings":
		h.handleSettings(w, r, rc)
	case path == "system/health":
		h.handleSystemHealth(w, r, rc)
	case path == "system/audit-logs":
		h.handleAudit(w, r, rc)
	case strings.HasPrefix(path, "program-setup/"):
		h.handleProgramSetup(w, r, rc, strings.TrimPrefix(path, "program-setup/"))
	case path == "dcp3/previews":
		h.handleDCP3PreviewCreate(w, r, rc)
	case strings.HasPrefix(path, "dcp3/previews/"):
		h.handleDCP3Preview(w, r, rc, strings.TrimPrefix(path, "dcp3/previews/"))
	case path == "dcp3/imports":
		h.handleDCP3Import(w, r, rc)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, principal auth.Principal, permission string) bool {
	allowed, err := h.deps.Auth.Can(r.Context(), principal, permission)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Tidak dapat memeriksa akses")
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
		return false
	}
	return true
}

func (h *Handler) authorizeAny(w http.ResponseWriter, r *http.Request, principal auth.Principal, permissions ...string) bool {
	for _, permission := range permissions {
		allowed, err := h.deps.Auth.Can(r.Context(), principal, permission)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Tidak dapat memeriksa akses")
			return false
		}
		if allowed {
			return true
		}
	}
	writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_invalid", "Gunakan Content-Type application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Isi permintaan JSON tidak valid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Hanya satu objek JSON yang diizinkan")
		return false
	}
	return true
}

func clientMeta(r *http.Request) auth.ClientMeta {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return auth.ClientMeta{IPAddress: host, UserAgent: r.UserAgent()}
}

func intQuery(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func boolQuery(r *http.Request, key string) *bool {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil
	}
	return &value
}

func isMutation(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": errorBody{Code: code, Message: message}})
}

func writeFieldError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeJSON(w, status, map[string]any{"error": errorBody{Code: code, Message: message, Fields: fields}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
