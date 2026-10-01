package api

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

func (h *Handler) handleBASTIndividual(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.BAST == nil {
		writeUnavailable(w)
		return
	}
	switch suffix {
	case "dates":
		h.handleBASTDates(w, r, rc)
	case "recipients":
		h.handleBASTRecipients(w, r, rc)
	case "lock-total":
		h.handleBASTLockTotal(w, r, rc)
	case "bundles/preview":
		h.handleBASTPreview(w, r, rc)
	case "bundles/finalize":
		h.handleBASTFinalize(w, r, rc)
	default:
		if strings.HasPrefix(suffix, "bundles/") && strings.HasSuffix(suffix, "/content") {
			id := strings.TrimSuffix(strings.TrimPrefix(suffix, "bundles/"), "/content")
			if id != "" && !strings.Contains(id, "/") {
				h.handleBASTContent(w, r, rc, id)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleBASTDates(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	programID, regencyID := r.URL.Query().Get("program_id"), r.URL.Query().Get("regency_id")
	if strings.TrimSpace(programID) == "" || strings.TrimSpace(regencyID) == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program dan kabupaten wajib dipilih", map[string]string{"program_id": "wajib diisi", "regency_id": "wajib diisi"})
		return
	}
	result, err := h.deps.BAST.ListDates(r.Context(), programID, regencyID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleBASTRecipients(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	programID, regencyID, localDate := r.URL.Query().Get("program_id"), r.URL.Query().Get("regency_id"), r.URL.Query().Get("date")
	if strings.TrimSpace(programID) == "" || strings.TrimSpace(regencyID) == "" || !validISODate(localDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program, kabupaten, dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return
	}
	result, err := h.deps.BAST.ListRecipients(r.Context(), programID, regencyID, localDate, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleBASTLockTotal(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	var input struct {
		ProgramID string `json:"program_id"`
		RegencyID string `json:"regency_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.ProgramID) == "" || strings.TrimSpace(input.RegencyID) == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program dan kabupaten wajib dipilih", map[string]string{"program_id": "wajib diisi", "regency_id": "wajib diisi"})
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.BAST.LockRegencyTotal(r.Context(), rc.principal, input.ProgramID, input.RegencyID, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleBASTPreview(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	input, scope, ok := h.decodeBundleRequest(w, r, rc)
	if !ok {
		return
	}
	result, err := h.deps.BAST.PreviewBundle(r.Context(), input, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writePDF(w, "inline", result.Filename, bytes.NewReader(result.PDF))
}

func (h *Handler) handleBASTFinalize(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	input, scope, ok := h.decodeBundleRequest(w, r, rc)
	if !ok {
		return
	}
	result, err := h.deps.BAST.FinalizeBundle(r.Context(), rc.principal, input, scope, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleBASTContent(w http.ResponseWriter, r *http.Request, rc requestContext, id string) {
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
	content, err := h.deps.BAST.OpenBundle(r.Context(), id, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	writePDF(w, "attachment", content.Filename, content.Reader)
}

func (h *Handler) handleBASTBranding(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.BASTBranding == nil {
		writeUnavailable(w)
		return
	}
	switch {
	case suffix == "":
		h.handleBASTBrandingList(w, r, rc)
	case suffix == "logos":
		h.handleBASTBrandingUpload(w, r, rc)
	case strings.HasPrefix(suffix, "logos/"):
		rest := strings.Split(strings.TrimPrefix(suffix, "logos/"), "/")
		logoID := rest[0]
		if logoID == "" {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		if len(rest) == 2 && rest[1] == "content" {
			h.handleBASTBrandingContent(w, r, rc, logoID)
			return
		}
		if len(rest) == 1 {
			h.handleBASTBrandingPatch(w, r, rc, logoID)
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleBASTBrandingList(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	programID := strings.TrimSpace(r.URL.Query().Get("program_id"))
	if programID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program wajib dipilih", map[string]string{"program_id": "wajib diisi"})
		return
	}
	items, err := h.deps.BASTBranding.ListBranding(r.Context(), programID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (h *Handler) handleBASTBrandingUpload(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Logo melebihi batas 10 MiB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	programID := strings.TrimSpace(r.FormValue("program_id"))
	if programID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program wajib dipilih", map[string]string{"program_id": "wajib diisi"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Logo wajib dipilih", map[string]string{"file": "Logo wajib dipilih"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil || len(data) > 10<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Logo melebihi batas 10 MiB")
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	maxWidth, _ := strconv.ParseFloat(r.FormValue("max_width_mm"), 64)
	maxHeight, _ := strconv.ParseFloat(r.FormValue("max_height_mm"), 64)
	item, err := h.deps.BASTBranding.UploadLogo(r.Context(), rc.principal, bast.LogoUploadInput{ProgramID: programID, SlotCode: r.FormValue("slot_code"), OriginalFilename: header.Filename, Data: data, SortOrder: sortOrder, MaxWidthMM: maxWidth, MaxHeightMM: maxHeight}, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, item)
}

func (h *Handler) handleBASTBrandingPatch(w http.ResponseWriter, r *http.Request, rc requestContext, logoID string) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.manage") {
		return
	}
	var input struct {
		ProgramID   string  `json:"program_id"`
		SortOrder   int     `json:"sort_order"`
		MaxWidthMM  float64 `json:"max_width_mm"`
		MaxHeightMM float64 `json:"max_height_mm"`
		IsVisible   bool    `json:"is_visible"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.deps.BASTBranding.PatchLogo(r.Context(), rc.principal, bast.LogoPatchInput{ID: logoID, ProgramID: input.ProgramID, SortOrder: input.SortOrder, MaxWidthMM: input.MaxWidthMM, MaxHeightMM: input.MaxHeightMM, IsVisible: input.IsVisible}, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, item)
}

func (h *Handler) handleBASTBrandingContent(w http.ResponseWriter, r *http.Request, rc requestContext, logoID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	programID := strings.TrimSpace(r.URL.Query().Get("program_id"))
	if programID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program wajib dipilih", map[string]string{"program_id": "wajib diisi"})
		return
	}
	content, err := h.deps.BASTBranding.OpenLogo(r.Context(), programID, logoID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Content-Type", content.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

func (h *Handler) decodeBundleRequest(w http.ResponseWriter, r *http.Request, rc requestContext) (bast.BundleRequest, auth.RegencyScope, bool) {
	var input bast.BundleRequest
	if !decodeJSON(w, r, &input) {
		return bast.BundleRequest{}, auth.RegencyScope{}, false
	}
	if strings.TrimSpace(input.ProgramID) == "" || strings.TrimSpace(input.RegencyID) == "" || !validISODate(input.LocalDate) {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Program, kabupaten, dan tanggal valid wajib dipilih", map[string]string{"request": "gunakan tanggal YYYY-MM-DD"})
		return bast.BundleRequest{}, auth.RegencyScope{}, false
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return bast.BundleRequest{}, auth.RegencyScope{}, false
	}
	return input, scope, true
}

func validISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func writePDF(w http.ResponseWriter, disposition, filename string, source io.Reader) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, source)
}
