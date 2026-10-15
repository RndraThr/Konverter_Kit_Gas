package api

import (
	"net/http"
	"strings"

	"konkit/internal/finalreport"
)

func (h *Handler) handleFinalReportSettings(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "final_report.view") {
			return
		}
		programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		settings, err := h.deps.FinalReport.GetSettings(r.Context(), programID, regencyID, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, settings)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "final_report.manage") {
			return
		}
		var input finalreport.Settings
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		settings, err := h.deps.FinalReport.PutSettings(r.Context(), rc.principal, input, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleFinalReportPhotoPicks(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	programID, regencyID, slideKey := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id")), strings.TrimSpace(r.URL.Query().Get("slide_key"))
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "final_report.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		picks, err := h.deps.FinalReport.ListPhotoPicks(r.Context(), programID, regencyID, slideKey, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, picks)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "final_report.manage") {
			return
		}
		var input struct {
			ProgramID        string   `json:"program_id"`
			RegencyID        string   `json:"regency_id"`
			SlideKey         string   `json:"slide_key"`
			ActivityMediaIDs []string `json:"activity_media_ids"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		picks, err := h.deps.FinalReport.ReplacePhotoPicks(r.Context(), input.ProgramID, input.RegencyID, input.SlideKey, input.ActivityMediaIDs, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, picks)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPut)
	}
}

func (h *Handler) handleFinalReportPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.view") {
		return
	}
	programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	preview, err := h.deps.FinalReport.Preview(r.Context(), programID, regencyID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, preview)
}

func (h *Handler) handleFinalReportGenerate(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.manage") {
		return
	}
	var body struct {
		ProgramID string `json:"program_id"`
		RegencyID string `json:"regency_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	doc, err := h.deps.FinalReport.Generate(r.Context(), rc.principal, body.ProgramID, body.RegencyID, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, doc)
}

func (h *Handler) handleFinalReportDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.FinalReport == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "final_report.view") {
		return
	}
	programID, regencyID := strings.TrimSpace(r.URL.Query().Get("program_id")), strings.TrimSpace(r.URL.Query().Get("regency_id"))
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	docs, err := h.deps.FinalReport.ListDocuments(r.Context(), programID, regencyID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, docs)
}
