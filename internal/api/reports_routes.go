package api

import (
	"mime"
	"net/http"
	"strings"

	"konkit/internal/reports"
)

func (h *Handler) handleReportsSchedule(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Reports == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.view") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	scheduleID := parts[0]
	filter := reports.Filter{
		AllocationStatus:    strings.TrimSpace(r.URL.Query().Get("allocation_status")),
		DistributionStatus:  strings.TrimSpace(r.URL.Query().Get("distribution_status")),
		DocumentationStatus: strings.TrimSpace(r.URL.Query().Get("documentation_status")),
	}
	switch parts[1] {
	case "summary":
		result, err := h.deps.Reports.Summary(r.Context(), scheduleID, filter, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "rows":
		result, err := h.deps.Reports.Rows(r.Context(), scheduleID, filter, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "export.xlsx":
		data, err := h.deps.Reports.ExportExcel(r.Context(), rc.principal, scheduleID, filter, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, "laporan-distribusi.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
	case "export.pdf":
		data, err := h.deps.Reports.ExportPDF(r.Context(), rc.principal, scheduleID, filter, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, "laporan-distribusi.pdf", "application/pdf", data)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func writeAttachment(w http.ResponseWriter, filename, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
