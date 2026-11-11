package api

import (
	"mime"
	"net/http"
	"strings"
)

// Laporan DP3 (penerima selesai satu jadwal, tanpa status):
//
//	GET  reports/schedule/{id}/summary       total penerima jadwal, dipakai aplikasi HP (distribution.view)
//	GET  reports/schedule/{id}/rows          isi laporan (distribution.view)
//	GET  reports/schedule/{id}/export.xlsx   unduh Excel (distribution.view)
//	GET  reports/schedule/{id}/export.pdf    unduh PDF (distribution.view)
//	GET  reports/schedule/{id}/drive         status file di Drive (distribution.view)
//	POST reports/schedule/{id}/drive/sync    perbarui file di Drive sekarang (distribution.manage)
func (h *Handler) handleReportsSchedule(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Reports == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	scheduleID, action := parts[0], strings.Join(parts[1:], "/")
	if action == "drive/sync" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if !h.authorize(w, r, rc.principal, "distribution.manage") {
			return
		}
	} else {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		if !h.authorize(w, r, rc.principal, "distribution.view") {
			return
		}
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	switch action {
	case "summary":
		result, err := h.deps.Reports.Summary(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "rows":
		result, err := h.deps.Reports.Report(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "export.xlsx":
		file, err := h.deps.Reports.ExportExcel(r.Context(), rc.principal, scheduleID, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, file.Filename, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file.Data)
	case "export.pdf":
		file, err := h.deps.Reports.ExportPDF(r.Context(), rc.principal, scheduleID, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeAttachment(w, file.Filename, "application/pdf", file.Data)
	case "drive":
		result, err := h.deps.Reports.DriveStatus(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case "drive/sync":
		result, err := h.deps.Reports.SyncDrive(r.Context(), scheduleID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
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
