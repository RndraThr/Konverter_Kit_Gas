package api

import (
	"context"
	"net/http"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type BASTHandoverService interface {
	View(context.Context, string, auth.RegencyScope) (bast.HandoverView, error)
	Rename(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
	Merge(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
	Delete(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
	Assign(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
}

// handleBASTHandoverLocations melayani /api/v1/bast/handover-locations
// (?schedule_id=):
//
//	GET              rekap per titik serah dan penerima tanpa titik serah (bast.view)
//	PUT    assign    {"slot_id","location_id"} koreksi titik serah satu penerima (bast.manage)
//	PATCH  {id}      {"name"} ubah nama (bast.manage)
//	POST   {id}/merge {"target_id"} gabungkan ke lokasi lain (bast.manage)
//	DELETE {id}      hapus lokasi tanpa penerima (bast.manage)
func (h *Handler) handleBASTHandoverLocations(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	service := h.deps.BASTHandover
	if service == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
		return
	}
	respond := func(view bast.HandoverView, err error) {
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, view)
	}
	parts := strings.Split(suffix, "/")
	switch {
	case suffix == "" && r.Method == http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		respond(service.View(r.Context(), scheduleID, scope))
	case suffix == "assign" && r.Method == http.MethodPut:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			SlotID     string `json:"slot_id"`
			LocationID string `json:"location_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		respond(service.Assign(r.Context(), rc.principal, scheduleID, input.SlotID, input.LocationID, scope, clientMeta(r)))
	case len(parts) == 1 && parts[0] != "" && r.Method == http.MethodPatch:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		respond(service.Rename(r.Context(), rc.principal, scheduleID, parts[0], input.Name, scope, clientMeta(r)))
	case len(parts) == 1 && parts[0] != "" && r.Method == http.MethodDelete:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		respond(service.Delete(r.Context(), rc.principal, scheduleID, parts[0], scope, clientMeta(r)))
	case len(parts) == 2 && parts[1] == "merge" && r.Method == http.MethodPost:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			TargetID string `json:"target_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		respond(service.Merge(r.Context(), rc.principal, scheduleID, parts[0], input.TargetID, scope, clientMeta(r)))
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}
