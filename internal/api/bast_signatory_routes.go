package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type BASTSignatoryService interface {
	Settings(context.Context, string, string, auth.RegencyScope) (bast.SignatureSettingsView, error)
	SaveRules(context.Context, auth.Principal, string, string, bool, bast.SignatureRules, bool, auth.RegencyScope, auth.ClientMeta) (bast.SignatureSettingsView, error)
	SaveName(context.Context, auth.Principal, string, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	PutAsset(context.Context, auth.Principal, string, string, string, []byte, auth.RegencyScope, auth.ClientMeta) error
	DeleteAsset(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	OpenAsset(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
}

type BASTSignatureLayoutService interface {
	Layout(context.Context, string, string, string, auth.RegencyScope) (bast.SignatureLayout, error)
}

// handleBASTSignatures melayani /api/v1/bast/signatures:
//
//	GET  ?schedule_id=&document=dp3                  isi panel (bast.view)
//	GET  layout?schedule_id=&document=&date=         editor: PDF halaman tanda
//	     tangan tanpa tanda dan letak kotak per pihak (bast.view)
//	PUT  {schedule_id, document, override, rules}    aturan cetak (bast.manage)
//	PUT  signatories/{role}?schedule_id=  {name, nip} nama/NIP penandatangan
//	GET|PUT|DELETE signatories/{role}/{signature|stamp}?schedule_id=
//	     gambar PNG (PUT {"image_png": base64})
func (h *Handler) handleBASTSignatures(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	service := h.deps.BASTSignatory
	if service == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if suffix == "" {
		h.handleBASTSignatureRules(w, r, rc, service, scope)
		return
	}
	if suffix == "layout" {
		h.handleBASTSignatureLayout(w, r, rc, scope)
		return
	}
	parts := strings.Split(strings.TrimPrefix(suffix, "signatories/"), "/")
	if !strings.HasPrefix(suffix, "signatories/") || len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	role := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			Name string `json:"name"`
			NIP  string `json:"nip"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := service.SaveName(r.Context(), rc.principal, scheduleID, role, input.Name, input.NIP, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"role": role})
		return
	}
	kind := parts[1]
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		image, err := service.OpenAsset(r.Context(), scheduleID, role, kind, scope)
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
		// PNG maksimal 2 MB; base64 menambah sekitar sepertiganya.
		if !decodeJSONLimit(w, r, &input, 3<<20) {
			return
		}
		if err := service.PutAsset(r.Context(), rc.principal, scheduleID, role, kind, input.ImagePNG, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"role": role, "kind": kind})
	case http.MethodDelete:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		if err := service.DeleteAsset(r.Context(), rc.principal, scheduleID, role, kind, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"role": role, "kind": kind})
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
	}
}

func (h *Handler) handleBASTSignatureRules(w http.ResponseWriter, r *http.Request, rc requestContext, service BASTSignatoryService, scope auth.RegencyScope) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		view, err := service.Settings(r.Context(), r.URL.Query().Get("schedule_id"), r.URL.Query().Get("document"), scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, view)
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			ScheduleID string              `json:"schedule_id"`
			Document   string              `json:"document"`
			Override   bool                `json:"override"`
			Rules      bast.SignatureRules `json:"rules"`
			Wet        bool                `json:"wet_signature"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		view, err := service.SaveRules(r.Context(), rc.principal, input.ScheduleID, input.Document, input.Override, input.Rules, input.Wet, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, view)
	default:
		methodNotAllowed(w, "GET, PUT")
	}
}

func (h *Handler) handleBASTSignatureLayout(w http.ResponseWriter, r *http.Request, rc requestContext, scope auth.RegencyScope) {
	service := h.deps.BASTSignatureLayout
	if service == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorize(w, r, rc.principal, "bast.view") {
		return
	}
	query := r.URL.Query()
	layout, err := service.Layout(r.Context(), query.Get("document"), query.Get("schedule_id"), query.Get("date"), scope)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeData(w, http.StatusOK, layout)
}
