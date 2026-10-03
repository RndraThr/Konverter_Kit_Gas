package api

import (
	"bytes"
	"net/http"
	"strings"

	"konkit/internal/bast"
)

func (h *Handler) handleBASTDP3(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.DP3 == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "summary":
		h.handleDP3Summary(w, r, rc)
	case "recipients":
		h.handleDP3Recipients(w, r, rc)
	case "preview":
		h.handleDP3Preview(w, r, rc)
	case "finalize":
		h.handleDP3Finalize(w, r, rc)
	case "documents":
		h.handleDP3Documents(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleDP3Content(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleDP3Summary(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3.Summary(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDP3Recipients(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3.Recipients(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.DP3Recipient{}
	}
	writeData(w, http.StatusOK, result)
}

type dp3DocumentRequest struct {
	ScheduleID   string `json:"schedule_id"`
	DocumentDate string `json:"document_date"`
}

func decodeDP3DocumentRequest(w http.ResponseWriter, r *http.Request, rc requestContext) (dp3DocumentRequest, bool) {
	var input dp3DocumentRequest
	if !decodeJSON(w, r, &input) {
		return dp3DocumentRequest{}, false
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.DocumentDate = strings.TrimSpace(input.DocumentDate)
	if input.ScheduleID == "" || !validISODate(input.DocumentDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return dp3DocumentRequest{}, false
	}
	return input, true
}

func (h *Handler) handleDP3Preview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodeDP3DocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3.Preview(r.Context(), input.ScheduleID, input.DocumentDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleDP3Finalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodeDP3DocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3.Finalize(r.Context(), rc.principal, input.ScheduleID, input.DocumentDate, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDP3Documents(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DP3.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDP3Content(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.DP3.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}

func (h *Handler) handleBASTDailyRecap(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.DailyRecap == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "dates":
		h.handleDailyRecapDates(w, r, rc)
	case "recipients":
		h.handleDailyRecapRecipients(w, r, rc)
	case "preview":
		h.handleDailyRecapPreview(w, r, rc)
	case "finalize":
		h.handleDailyRecapFinalize(w, r, rc)
	case "documents":
		h.handleDailyRecapDocuments(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleDailyRecapContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleDailyRecapDates(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DailyRecap.Dates(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.DailyRecapDate{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDailyRecapRecipients(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DailyRecap.Recipients(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.DailyRecapRecipient{}
	}
	writeData(w, http.StatusOK, result)
}

type dailyRecapDocumentRequest struct {
	ScheduleID string `json:"schedule_id"`
	LocalDate  string `json:"local_date"`
}

func decodeDailyRecapDocumentRequest(w http.ResponseWriter, r *http.Request, rc requestContext) (dailyRecapDocumentRequest, bool) {
	var input dailyRecapDocumentRequest
	if !decodeJSON(w, r, &input) {
		return dailyRecapDocumentRequest{}, false
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.LocalDate = strings.TrimSpace(input.LocalDate)
	if input.ScheduleID == "" || !validISODate(input.LocalDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return dailyRecapDocumentRequest{}, false
	}
	return input, true
}

func (h *Handler) handleDailyRecapPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodeDailyRecapDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DailyRecap.Preview(r.Context(), input.ScheduleID, input.LocalDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleDailyRecapFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodeDailyRecapDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DailyRecap.Finalize(r.Context(), rc.principal, input.ScheduleID, input.LocalDate, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDailyRecapDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.DailyRecap.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDailyRecapContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.DailyRecap.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}

func (h *Handler) handleBASTClosingTitikSerah(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.ClosingTitikSerah == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "rows":
		h.handleClosingRows(w, r, rc)
	case "preview":
		h.handleClosingPreview(w, r, rc)
	case "finalize":
		h.handleClosingFinalize(w, r, rc)
	case "documents":
		h.handleClosingDocuments(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleClosingContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleClosingRows(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingTitikSerah.Rows(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.ClosingRow{}
	}
	writeData(w, http.StatusOK, result)
}

type closingDocumentRequest struct {
	ScheduleID   string `json:"schedule_id"`
	DocumentDate string `json:"document_date"`
}

func decodeClosingDocumentRequest(w http.ResponseWriter, r *http.Request, rc requestContext) (closingDocumentRequest, bool) {
	var input closingDocumentRequest
	if !decodeJSON(w, r, &input) {
		return closingDocumentRequest{}, false
	}
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.DocumentDate = strings.TrimSpace(input.DocumentDate)
	if input.ScheduleID == "" || !validISODate(input.DocumentDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return closingDocumentRequest{}, false
	}
	return input, true
}

func (h *Handler) handleClosingPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodeClosingDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingTitikSerah.Preview(r.Context(), input.ScheduleID, input.DocumentDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleClosingFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodeClosingDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingTitikSerah.Finalize(r.Context(), rc.principal, input.ScheduleID, input.DocumentDate, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleClosingDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingTitikSerah.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleClosingContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.ClosingTitikSerah.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}

func (h *Handler) handleBASTClosingKabupaten(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.ClosingKabupaten == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "rows":
		h.handleClosingKabupatenRows(w, r, rc)
	case "preview":
		h.handleClosingKabupatenPreview(w, r, rc)
	case "finalize":
		h.handleClosingKabupatenFinalize(w, r, rc)
	case "documents":
		h.handleClosingKabupatenDocuments(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "documents/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "documents/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleClosingKabupatenContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleClosingKabupatenRows(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingKabupaten.Rows(r.Context(), scheduleID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.ClosingKabupatenRow{}
	}
	writeData(w, http.StatusOK, result)
}

func decodeClosingKabupatenDocumentRequest(w http.ResponseWriter, r *http.Request, rc requestContext) (closingDocumentRequest, bool) {
	return decodeClosingDocumentRequest(w, r, rc)
}

func (h *Handler) handleClosingKabupatenPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, ok := decodeClosingKabupatenDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingKabupaten.Preview(r.Context(), input.ScheduleID, input.DocumentDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleClosingKabupatenFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, ok := decodeClosingKabupatenDocumentRequest(w, r, rc)
	if !ok {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingKabupaten.Finalize(r.Context(), rc.principal, input.ScheduleID, input.DocumentDate, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleClosingKabupatenDocuments(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if scheduleID == "" || !validISODate(date) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.ClosingKabupaten.Documents(r.Context(), scheduleID, date, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if result == nil {
		result = []bast.AggregateDocument{}
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleClosingKabupatenContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.ClosingKabupaten.Open(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}
