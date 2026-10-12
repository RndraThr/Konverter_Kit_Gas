package api

import (
	"context"
	"net/http"
	"strconv"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

// distributionSignatureService is the recipient-signature part of the
// distribution service (POS Dokumen).
type distributionSignatureService interface {
	SaveRecipientSignature(context.Context, auth.Principal, distribution.SaveSignatureInput, auth.ClientMeta, auth.RegencyScope) (distribution.DistributionSlot, error)
	DeleteRecipientSignature(context.Context, auth.Principal, string, int, auth.ClientMeta, auth.RegencyScope) (distribution.DistributionSlot, error)
	OpenRecipientSignature(context.Context, string, int, auth.RegencyScope) (distribution.SignatureContent, error)
}

// handleDistributionSignature serves /api/v1/distribution/slots/{n}/signature:
// GET the PNG (distribution.view), PUT {"image_png": base64} to sign or
// re-sign, DELETE to clear (both distribution.pos_dokumen).
func (h *Handler) handleDistributionSignature(w http.ResponseWriter, r *http.Request, rc requestContext, slotNumber int) {
	service, ok := h.deps.Distribution.(distributionSignatureService)
	if !ok {
		writeUnavailable(w)
		return
	}
	permission := "distribution.pos_dokumen"
	switch r.Method {
	case http.MethodGet:
		permission = "distribution.view"
	case http.MethodPut, http.MethodDelete:
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
		return
	}
	if !h.authorize(w, r, rc.principal, permission) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	scheduleID := r.URL.Query().Get("schedule_id")
	switch r.Method {
	case http.MethodGet:
		content, err := service.OpenRecipientSignature(r.Context(), scheduleID, slotNumber, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(content.ImagePNG)))
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content.ImagePNG)
	case http.MethodPut:
		var input struct {
			ImagePNG []byte `json:"image_png"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveRecipientSignature(r.Context(), rc.principal, distribution.SaveSignatureInput{ScheduleID: scheduleID, SlotNumber: slotNumber, ImagePNG: input.ImagePNG}, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := service.DeleteRecipientSignature(r.Context(), rc.principal, scheduleID, slotNumber, clientMeta(r), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	}
}
