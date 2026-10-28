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
	Save(context.Context, auth.Principal, string, []bast.HandoverLocationInput, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
	SetOverride(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) (bast.HandoverView, error)
}

// handleBASTHandoverLocations melayani /api/v1/bast/handover-locations:
//
//	GET  ?schedule_id=  daftar titik serah, penempatan penerima, rekap (bast.view)
//	PUT                 {"schedule_id","locations":[{"id","name","areas"}]} (bast.manage)
//	PUT  override       {"schedule_id","slot_id","location_id"} (bast.manage)
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
	switch suffix {
	case "":
		switch r.Method {
		case http.MethodGet:
			if !h.authorize(w, r, rc.principal, "bast.view") {
				return
			}
			scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
			if scheduleID == "" {
				writeFieldError(w, http.StatusBadRequest, "validation_failed", "Jadwal wajib dipilih", map[string]string{"schedule_id": "wajib diisi"})
				return
			}
			view, err := service.View(r.Context(), scheduleID, scope)
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
				ScheduleID string                       `json:"schedule_id"`
				Locations  []bast.HandoverLocationInput `json:"locations"`
			}
			if !decodeJSON(w, r, &input) {
				return
			}
			view, err := service.Save(r.Context(), rc.principal, input.ScheduleID, input.Locations, scope, clientMeta(r))
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeData(w, http.StatusOK, view)
		default:
			methodNotAllowed(w, "GET, PUT")
		}
	case "override":
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		var input struct {
			ScheduleID string `json:"schedule_id"`
			SlotID     string `json:"slot_id"`
			LocationID string `json:"location_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		view, err := service.SetOverride(r.Context(), rc.principal, input.ScheduleID, input.SlotID, input.LocationID, scope, clientMeta(r))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, view)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}
