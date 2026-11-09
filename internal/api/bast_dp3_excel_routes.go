package api

import (
	"io"
	"mime"
	"net/http"
	"strings"
)

// Excel DP3 (data penerima satu jadwal, disinkronkan berkala ke Drive):
//
//	GET  bast/dp3/excel?schedule_id=          status file (bast.view)
//	GET  bast/dp3/excel/content?schedule_id=  unduh workbook (bast.view)
//	POST bast/dp3/excel/sync {schedule_id}    bangun ulang sekarang (bast.manage)
func (h *Handler) handleDP3Excel(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.DP3Excel == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "":
		h.handleDP3ExcelStatus(w, r, rc)
	case "content":
		h.handleDP3ExcelContent(w, r, rc)
	case "sync":
		h.handleDP3ExcelSync(w, r, rc)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) dp3ExcelScheduleQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return "", false
	}
	return scheduleID, true
}

func (h *Handler) handleDP3ExcelStatus(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID, ok := h.dp3ExcelScheduleQuery(w, r)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3Excel.Status(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDP3ExcelContent(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID, ok := h.dp3ExcelScheduleQuery(w, r)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	content, err := h.deps.DP3Excel.Open(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

func (h *Handler) handleDP3ExcelSync(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	var input struct {
		ScheduleID string `json:"schedule_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	if input.ScheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3Excel.Sync(r.Context(), input.ScheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
