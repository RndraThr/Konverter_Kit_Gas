package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

// handleGeoReverse resolves a coordinate to an address for the camera watermark.
func (h *Handler) handleGeoReverse(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.Geo == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorizeAny(w, r, rc.principal, fieldReadPermissions...) {
		return
	}
	lat, latOK := parseCoordinate(r.URL.Query().Get("lat"), 90)
	lng, lngOK := parseCoordinate(r.URL.Query().Get("lng"), 180)
	if !latOK || !lngOK {
		fields := map[string]string{}
		if !latOK {
			fields["lat"] = "harus berupa angka antara -90 dan 90"
		}
		if !lngOK {
			fields["lng"] = "harus berupa angka antara -180 dan 180"
		}
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Koordinat tidak valid", fields)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"address": h.deps.Geo.Reverse(r.Context(), lat, lng)})
}

func parseCoordinate(raw string, limit float64) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.Abs(value) > limit {
		return 0, false
	}
	return value, true
}
