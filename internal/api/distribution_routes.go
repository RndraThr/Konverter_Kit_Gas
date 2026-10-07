package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"konkit/internal/distribution"
	"konkit/internal/media"
)

const maxMediaRequestBody = 501 << 20

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
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.CreateSlot(r.Context(), rc.principal, input, scope, clientMeta(r))
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

func (h *Handler) handleDistributionCandidateSuggestions(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.Distribution.SuggestCandidates(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("nik_prefix"), scope)
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

func (h *Handler) handleDistributionSlotCatalog(w http.ResponseWriter, r *http.Request, rc requestContext) {
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
	result, err := h.deps.Distribution.ListSlotCatalog(r.Context(), r.URL.Query().Get("schedule_id"), scope)
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
	if len(parts) == 1 && parts[0] == "catalog" && r.Method == http.MethodGet {
		h.handleDistributionSlotCatalog(w, r, rc)
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
		case parts[1] == "date" && r.Method == http.MethodPatch:
			h.handleDistributionDateUpdate(w, r, rc, slotNumber)
			return
		case parts[1] == "equipment" && r.Method == http.MethodPatch:
			h.handleDistributionEquipmentUpdate(w, r, rc, slotNumber)
			return
		case parts[1] == "recipient" && r.Method == http.MethodPatch:
			h.handleDistributionRecipientUpdate(w, r, rc, slotNumber)
			return
		case parts[1] == "replace-recipient" && r.Method == http.MethodPost:
			h.handleDistributionRecipientReplace(w, r, rc, slotNumber)
			return
		case parts[1] == "reopen" && r.Method == http.MethodPost:
			h.handleDistributionSlotReopen(w, r, rc, slotNumber)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
}

func (h *Handler) handleDistributionDateUpdate(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_mesin") {
		return
	}
	var input distribution.SetDistributionDateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ScheduleID = r.URL.Query().Get("schedule_id")
	input.SlotNumber = slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.SetDistributionDate(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionEquipmentUpdate(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	var input distribution.UpdateEquipmentInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ScheduleID = r.URL.Query().Get("schedule_id")
	input.SlotNumber = slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.UpdateEquipment(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionRecipientUpdate(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	var input distribution.UpdateRecipientInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ScheduleID, input.SlotNumber = r.URL.Query().Get("schedule_id"), slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.UpdateRecipient(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionRecipientReplace(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	if !h.authorize(w, r, rc.principal, "distribution.pos_dokumen") {
		return
	}
	var input distribution.ReplaceRecipientInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ScheduleID, input.SlotNumber = r.URL.Query().Get("schedule_id"), slotNumber
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Distribution.ReplaceRecipient(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) handleDistributionSlotReopen(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	var input distribution.ReopenSlotInput
	if !decodeJSON(w, r, &input) {
		return
	}
	permission, ok := distributionStagePermission(strings.TrimSpace(input.Stage))
	if !ok {
		writeServiceError(w, distribution.ErrRevisionStageInvalid)
		return
	}
	if !h.authorize(w, r, rc.principal, permission) {
		return
	}
	input.ScheduleID, input.SlotNumber = r.URL.Query().Get("schedule_id"), slotNumber
	scope, allowed := h.regencyScope(w, r, rc.principal)
	if !allowed {
		return
	}
	result, err := h.deps.Distribution.ReopenSlot(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func distributionStagePermission(stage string) (string, bool) {
	permissions := map[string]string{"mesin": "distribution.pos_mesin", "dokumen": "distribution.pos_dokumen", "penyerahan": "distribution.pos_penyerahan"}
	permission, ok := permissions[stage]
	return permission, ok
}

func (h *Handler) handleDistributionSlotMediaUpload(w http.ResponseWriter, r *http.Request, rc requestContext, slotID string) {
	if h.deps.Distribution == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	stage, err := h.deps.Distribution.DocumentationSlotStage(r.Context(), slotID, scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	permission, validStage := distributionStagePermission(stage)
	if !validStage {
		writeServiceError(w, distribution.ErrRevisionStageInvalid)
		return
	}
	if !h.authorize(w, r, rc.principal, permission) {
		return
	}
	started := time.Now()
	kind := media.Kind("unknown")
	loggedSize := int64(0)
	tracked := &statusTrackingResponseWriter{ResponseWriter: w, status: http.StatusOK}
	w = tracked
	defer func() { logMediaUpload("distribution", kind, loggedSize, tracked.status, started) }()

	upload, err := openMediaMultipart(w, r, maxMediaRequestBody, map[string]int64{
		"source": 16, "file_size": 20, "captured_at": 64, "latitude": 32, "longitude": 32,
	}, []string{"source", "file_size"})
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
		} else {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		}
		return
	}
	loggedSize = upload.DeclaredSize
	input := distribution.UploadMediaInput{SlotID: slotID, OriginalFilename: upload.Filename, Source: strings.TrimSpace(upload.Fields["source"]), Data: upload.File, DeclaredSize: upload.DeclaredSize}
	if raw := strings.TrimSpace(upload.Fields["captured_at"]); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "validation_failed", "Waktu pengambilan tidak valid", map[string]string{"captured_at": "Gunakan waktu RFC3339"})
			return
		}
		input.CapturedAt = &value
	}
	latitude, err := optionalFloat(upload.Fields["latitude"])
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"latitude": "Latitude tidak valid"})
		return
	}
	input.Latitude = latitude
	longitude, err := optionalFloat(upload.Fields["longitude"])
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", map[string]string{"longitude": "Longitude tidak valid"})
		return
	}
	input.Longitude = longitude
	result, err := h.deps.Distribution.UploadMedia(r.Context(), rc.principal, input, clientMeta(r), scope)
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.Is(err, errMultipartTrailingPart):
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		case errors.As(err, &tooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
		default:
			writeServiceError(w, err)
		}
		return
	}
	kind = mediaKindFromMIME(result.MimeType)
	loggedSize = result.ByteSize
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
		scope, ok := h.regencyScope(w, r, rc.principal)
		if !ok {
			return
		}
		stage, err := h.deps.Distribution.MediaStage(r.Context(), parts[0], scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		permission, validStage := distributionStagePermission(stage)
		if !validStage {
			writeServiceError(w, distribution.ErrRevisionStageInvalid)
			return
		}
		if !h.authorize(w, r, rc.principal, permission) {
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
