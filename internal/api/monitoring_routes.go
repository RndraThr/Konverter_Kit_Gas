package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/monitoring"
)

// MonitoringService is the central team's live distribution monitoring.
type MonitoringService interface {
	Overview(ctx context.Context, programID, date string, scope auth.RegencyScope) (monitoring.Overview, error)
	SaveCalendar(ctx context.Context, actor auth.Principal, input monitoring.CalendarInput, scope auth.RegencyScope, meta auth.ClientMeta) error
	SaveAssignments(ctx context.Context, actor auth.Principal, input monitoring.AssignmentInput, scope auth.RegencyScope, meta auth.ClientMeta) error
	Queue(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]monitoring.QueueItem, error)
	Slot(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) (monitoring.SlotDetail, error)
	Approve(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, input monitoring.ApproveInput, scope auth.RegencyScope, meta auth.ClientMeta) (monitoring.SlotDetail, error)
	RequestRevision(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, input monitoring.RevisionInput, scope auth.RegencyScope, meta auth.ClientMeta) (monitoring.SlotDetail, error)
}

// handleMonitoring serves /api/v1/monitoring/...:
//
//	GET  overview?program_id=&date=        kabupaten, phase days, assignments, live numbers
//	PUT  calendar                          phase days and Rakornas (monitoring.manage)
//	PUT  assignments                       reviewers and field leaders (monitoring.manage)
//	GET  queue?schedule_id=                recipients to check
//	GET  slots/{n}?schedule_id=            one recipient with photos and latest check
//	POST slots/{n}/approve?schedule_id=    approve photos, default every unchecked one (monitoring.review)
//	POST slots/{n}/revision?schedule_id=   reject photos and tell the field (monitoring.review)
func (h *Handler) handleMonitoring(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	service := h.deps.Monitoring
	if service == nil {
		writeUnavailable(w)
		return
	}
	permission := "monitoring.view"
	switch {
	case path == "calendar" || path == "assignments":
		permission = "monitoring.manage"
	case r.Method == http.MethodPost:
		permission = "monitoring.review"
	}
	if !h.authorize(w, r, rc.principal, permission) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	query := r.URL.Query()
	scheduleID := strings.TrimSpace(query.Get("schedule_id"))
	switch {
	case path == "overview" && r.Method == http.MethodGet:
		result, err := service.Overview(r.Context(), query.Get("program_id"), query.Get("date"), scope)
		writeMonitoringResult(w, result, err)
	case path == "calendar" && r.Method == http.MethodPut:
		var input monitoring.CalendarInput
		if !decodeJSON(w, r, &input) {
			return
		}
		writeMonitoringResult(w, map[string]bool{"saved": true}, service.SaveCalendar(r.Context(), rc.principal, input, scope, clientMeta(r)))
	case path == "assignments" && r.Method == http.MethodPut:
		var input monitoring.AssignmentInput
		if !decodeJSON(w, r, &input) {
			return
		}
		writeMonitoringResult(w, map[string]bool{"saved": true}, service.SaveAssignments(r.Context(), rc.principal, input, scope, clientMeta(r)))
	case path == "queue" && r.Method == http.MethodGet:
		result, err := service.Queue(r.Context(), scheduleID, scope)
		writeMonitoringResult(w, result, err)
	case strings.HasPrefix(path, "slots/"):
		parts := strings.Split(strings.TrimPrefix(path, "slots/"), "/")
		number, err := strconv.Atoi(parts[0])
		if err != nil || number < 1 || len(parts) > 2 {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		action := ""
		if len(parts) == 2 {
			action = parts[1]
		}
		switch {
		case action == "" && r.Method == http.MethodGet:
			result, err := service.Slot(r.Context(), scheduleID, number, scope)
			writeMonitoringResult(w, result, err)
		case action == "approve" && r.Method == http.MethodPost:
			var input monitoring.ApproveInput
			if r.ContentLength != 0 && !decodeJSON(w, r, &input) {
				return
			}
			result, err := service.Approve(r.Context(), rc.principal, scheduleID, number, input, scope, clientMeta(r))
			writeMonitoringResult(w, result, err)
		case action == "revision" && r.Method == http.MethodPost:
			var input monitoring.RevisionInput
			if !decodeJSON(w, r, &input) {
				return
			}
			result, err := service.RequestRevision(r.Context(), rc.principal, scheduleID, number, input, scope, clientMeta(r))
			writeMonitoringResult(w, result, err)
		default:
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		}
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func writeMonitoringResult(w http.ResponseWriter, result any, err error) {
	switch {
	case err == nil:
		writeData(w, http.StatusOK, result)
	case errors.Is(err, monitoring.ErrProgramNotFound), errors.Is(err, monitoring.ErrScheduleNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Program atau jadwal tidak ditemukan")
	case errors.Is(err, monitoring.ErrRakornasForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Jadwal Rakornas hanya dapat diubah akun tanpa batasan kabupaten")
	case errors.Is(err, monitoring.ErrInvalidPhase):
		writeError(w, http.StatusUnprocessableEntity, "monitoring_phase_invalid", "Tanggal atau tahap jadwal tidak valid")
	case errors.Is(err, monitoring.ErrInvalidUser):
		writeError(w, http.StatusUnprocessableEntity, "monitoring_user_invalid", "Pengguna yang dipilih tidak aktif atau tidak ditemukan")
	case errors.Is(err, monitoring.ErrNothingToApprove):
		writeError(w, http.StatusConflict, "monitoring_nothing_to_approve", "Tidak ada foto baru untuk disetujui")
	case errors.Is(err, monitoring.ErrRevisionEmpty):
		writeError(w, http.StatusUnprocessableEntity, "monitoring_revision_empty", "Tandai minimal satu foto yang perlu diperbaiki")
	case errors.Is(err, monitoring.ErrFlagUnknown):
		writeError(w, http.StatusUnprocessableEntity, "monitoring_flag_unknown", "Foto yang ditandai bukan milik pelanggan ini")
	default:
		writeServiceError(w, err)
	}
}
