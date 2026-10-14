package api

import (
	"context"
	"net/http"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type BASTOverviewService interface {
	Overview(context.Context, string, auth.RegencyScope) (bast.Overview, error)
}

// handleBASTOverview melayani GET /api/v1/bast/overview?schedule_id=:
// status final, perlu dibuat ulang, arsip, dan tanda tangan basah per jenis BA.
func (h *Handler) handleBASTOverview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	service := h.deps.BASTOverview
	if service == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "schedule_id wajib diisi")
		return
	}
	overview, err := service.Overview(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, overview)
}
