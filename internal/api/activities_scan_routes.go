package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"konkit/internal/activities"
)

const maxScanRequestBody = activities.MaxScanRequestBytes + (1 << 20) // small margin over the service-level cap for multipart framing overhead

func (h *Handler) handleActivitiesSupportingDocumentsScan(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Activities == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.authorize(w, r, rc.principal, "activities.manage") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxScanRequestBody)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		return
	}
	var programID, regencyID, slotID, customName string
	var pages []io.Reader
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(nextErr, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
			} else {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			}
			return
		}
		switch part.FormName() {
		case "program_id":
			data, readErr := io.ReadAll(io.LimitReader(part, 129))
			if readErr != nil || len(data) > 128 {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			programID = strings.TrimSpace(string(data))
		case "regency_id":
			data, readErr := io.ReadAll(io.LimitReader(part, 129))
			if readErr != nil || len(data) > 128 {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			regencyID = strings.TrimSpace(string(data))
		case "slot_id":
			data, readErr := io.ReadAll(io.LimitReader(part, 129))
			if readErr != nil || len(data) > 128 {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			slotID = strings.TrimSpace(string(data))
		case "custom_name":
			data, readErr := io.ReadAll(io.LimitReader(part, 257))
			if readErr != nil || len(data) > 256 {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			customName = strings.TrimSpace(string(data))
		case "page":
			buf, err := io.ReadAll(io.LimitReader(part, activities.MaxScanPageBytes+1))
			if err != nil {
				writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
				return
			}
			if int64(len(buf)) > activities.MaxScanPageBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "media_too_large", "Berkas melebihi batas unggahan")
				return
			}
			pages = append(pages, bytes.NewReader(buf))
		default:
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			return
		}
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	result, err := h.deps.Activities.ScanUpload(r.Context(), rc.principal, activities.ScanUploadInput{
		ProgramID: programID, RegencyID: regencyID, SlotID: slotID, CustomName: customName, Source: "camera", Pages: pages,
	}, clientMeta(r), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}
