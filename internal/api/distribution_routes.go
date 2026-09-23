package api

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"konkit/internal/distribution"
)

const maxMediaRequestBody = 11 << 20

func (h *Handler) handleDistributionSlots(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.pos_mesin") {
		return
	}
	var input distribution.CreateSlotInput
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.deps.Distribution.CreateSlot(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) handleDistributionCandidates(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SearchCandidate(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("nik"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotLink(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	var input distribution.LinkSlotInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.SlotNumber = slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.LinkSlot(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotSearch(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Distribution == nil {
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
	result, err := h.deps.Distribution.SearchSlot(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("q"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotComplete(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_penyerahan") {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.CompleteSlot(r.Context(), rc.principal, distribution.CompleteSlotInput{ScheduleID: r.URL.Query().Get("schedule_id"), SlotNumber: slotNumber}, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlot(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "search" && r.Method == http.MethodGet {
		h.handleDistributionSlotSearch(w, r, rc)
		return
	}
	if len(parts) == 2 && parts[1] == "media" && r.Method == http.MethodPost {
		h.handleDistributionSlotMediaUpload(w, r, rc, parts[0])
		return
	}
	if len(parts) == 2 {
		slotNumber, err := strconv.Atoi(parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
			return
		}
		switch {
		case parts[1] == "link" && r.Method == http.MethodPost:
			h.handleDistributionSlotLink(w, r, rc, slotNumber)
			return
		case parts[1] == "complete" && r.Method == http.MethodPost:
			h.handleDistributionSlotComplete(w, r, rc, slotNumber)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func (h *Handler) handleDistributionSlotMediaUpload(w http.ResponseWriter, r *http.Request, rc requestContext, slotID string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	if !h.authorize(w, r, rc.principal, "documentation.manage") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaRequestBody)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Foto melebihi batas 10 MiB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Foto wajib dipilih", map[string]string{"file": "Foto wajib dipilih"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "media_invalid", "Foto tidak dapat dibaca")
		return
	}
	if len(data) > 10<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Foto melebihi batas 10 MiB")
		return
	}
	input := distribution.UploadMediaInput{SlotID: slotID, OriginalFilename: header.Filename, Source: strings.TrimSpace(r.FormValue("source")), Data: data}
	if raw := strings.TrimSpace(r.FormValue("captured_at")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Waktu pengambilan tidak valid", map[string]string{"captured_at": "Gunakan waktu RFC3339"})
			return
		}
		input.CapturedAt = &value
	}
	latitude, err := optionalFloat(r.FormValue("latitude"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"latitude": "Latitude tidak valid"})
		return
	}
	input.Latitude = latitude
	longitude, err := optionalFloat(r.FormValue("longitude"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"longitude": "Longitude tidak valid"})
		return
	}
	input.Longitude = longitude
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.UploadMedia(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) handleDistributionMedia(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet {
		if !h.authorize(w, r, rc.principal, "distribution.view") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		content, err := h.deps.Distribution.OpenMedia(r.Context(), parts[0], scope)
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
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if !h.authorize(w, r, rc.principal, "documentation.manage") {
			return
		}
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		if err := h.deps.Distribution.DeleteMedia(r.Context(), rc.principal, parts[0], clientMeta(r), scope); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func optionalFloat(raw string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
