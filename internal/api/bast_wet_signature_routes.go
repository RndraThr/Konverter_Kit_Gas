package api

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type BASTWetSignatureService interface {
	Status(context.Context, string, string, string, auth.RegencyScope) (bast.WetSignatureStatus, error)
	Page(context.Context, string, string, string, auth.RegencyScope) (bast.AggregatePreview, string, error)
	Upload(context.Context, auth.Principal, string, string, string, []byte, auth.RegencyScope, auth.ClientMeta) error
	Delete(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	Open(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
}

// handleBASTWetSignature melayani /api/v1/bast/wet-signature
// (?schedule_id=&document=&date=):
//
//	GET          status mode tanda tangan basah (bast.view)
//	GET  page    PDF halaman tanda tangan untuk dicetak (bast.view)
//	GET  scan    gambar PNG pindaian (bast.view)
//	PUT  scan    unggah pindaian {"image_png": base64} (bast.manage)
//	DELETE scan  hapus pindaian (bast.manage)
func (h *Handler) handleBASTWetSignature(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	service := h.deps.BASTWetSignature
	if service == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	query := r.URL.Query()
	scheduleID, document, date := strings.TrimSpace(query.Get("schedule_id")), strings.TrimSpace(query.Get("document")), strings.TrimSpace(query.Get("date"))
	switch suffix {
	case "":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		status, err := service.Status(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, status)
	case "page":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		page, code, err := service.Page(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("X-Document-Code", code)
		writePDF(w, "inline", "halaman-tanda-tangan-"+page.Filename, bytes.NewReader(page.PDF))
	case "scan":
		h.handleBASTWetScan(w, r, rc, service, scope, document, scheduleID, date)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleBASTWetScan(w http.ResponseWriter, r *http.Request, rc requestContext, service BASTWetSignatureService, scope auth.RegencyScope, document, scheduleID, date string) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		image, err := service.Open(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(image)))
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(image)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			ImagePNG []byte `json:"image_png"`
		}
		// Base64 menambah sekitar sepertiga dari batas gambar 8 MB.
		if !decodeJSONLimit(w, r, &input, 12<<20) {
			return
		}
		if err := service.Upload(r.Context(), rc.principal, document, scheduleID, date, input.ImagePNG, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		status, err := service.Status(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, status)
	case http.MethodDelete:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		if err := service.Delete(r.Context(), rc.principal, document, scheduleID, date, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"document": document})
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
	}
}
