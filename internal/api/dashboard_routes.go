package api

import (
	"context"
	"net/http"

	"konkit/internal/auth"
	"konkit/internal/monitoring"
)

// DashboardService is the super admin's charts of a program.
type DashboardService interface {
	Dashboard(ctx context.Context, programID string, scope auth.RegencyScope) (monitoring.Dashboard, error)
}

// handleDashboardProgress serves GET /api/v1/dashboard/progress?program_id=
// (progress_dashboard.view): each stage per kabupaten, zone and day, and the
// activity documentation.
func (h *Handler) handleDashboardProgress(w http.ResponseWriter, r *http.Request, rc requestContext) {
	service := h.deps.Dashboard
	if service == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if !h.authorize(w, r, rc.principal, "progress_dashboard.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := service.Dashboard(r.Context(), r.URL.Query().Get("program_id"), scope)
	writeMonitoringResult(w, result, err)
}
