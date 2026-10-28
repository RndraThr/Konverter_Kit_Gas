package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/distmap"
)

// DistributionMapService serves the distribution map: located slots, unlocated slots, zone and
// kabupaten boundaries.
type DistributionMapService interface {
	Points(context.Context, distmap.PointQuery, auth.RegencyScope) ([]distmap.Point, distmap.PointsMeta, error)
	UnlocatedSlots(context.Context, distmap.PointQuery, auth.RegencyScope) ([]distmap.UnlocatedSlot, int, error)
	ZoneBoundaries(context.Context, string, auth.RegencyScope) ([]distmap.ZoneBoundary, error)
	RegencyBoundary(context.Context, string, auth.RegencyScope) (json.RawMessage, error)
}

// mapViewPermissions are the permissions of GET /recipients/map; every map endpoint shares them.
var mapViewPermissions = []string{"recipients.view", "distribution.view"}

// mapQueryFromRequest reads the Data Penerima filter plus bbox and stage. On invalid input it has
// already answered 400 validation_failed and returns false.
func mapQueryFromRequest(w http.ResponseWriter, r *http.Request) (distmap.PointQuery, bool) {
	query := distmap.PointQuery{Filter: recipientFilterFromRequest(r)}
	if raw := strings.TrimSpace(r.URL.Query().Get("bbox")); raw != "" {
		bbox, ok := parseBBox(raw)
		if !ok {
			writeMapValidation(w, "bbox", "Batas peta harus minLon,minLat,maxLon,maxLat")
			return query, false
		}
		query.BBox = &bbox
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("stage")); raw != "" {
		stage, ok := parseStage(raw)
		if !ok {
			writeMapValidation(w, "stage", "Tahap harus M1 sampai M5")
			return query, false
		}
		query.Stage = stage
	}
	if err := query.Validate(); err != nil {
		writeMapServiceError(w, err)
		return query, false
	}
	return query, true
}

// parseBBox parses "minLon,minLat,maxLon,maxLat" into four finite numbers.
func parseBBox(raw string) ([4]float64, bool) {
	var bbox [4]float64
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return bbox, false
	}
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return bbox, false
		}
		bbox[i] = value
	}
	return bbox, true
}

// parseStage accepts "M1".."M5" or "1".."5".
func parseStage(raw string) (int, bool) {
	stage, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(raw), "M"))
	if err != nil || stage < distmap.StageScheduled || stage > distmap.StageSigned {
		return 0, false
	}
	return stage, true
}

func writeMapValidation(w http.ResponseWriter, field, message string) {
	writeFieldError(w, http.StatusBadRequest, "validation_failed", message, map[string]string{field: message})
}

func writeMapServiceError(w http.ResponseWriter, err error) {
	var validation *distmap.ValidationError
	if errors.As(err, &validation) {
		writeMapValidation(w, validation.Field, validation.Message)
		return
	}
	writeServiceError(w, err)
}

// authorizeMap runs the checks shared by the map endpoints and returns the caller's regency scope.
func (h *Handler) authorizeMap(w http.ResponseWriter, r *http.Request, rc requestContext) (auth.RegencyScope, bool) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return auth.RegencyScope{}, false
	}
	if !h.authorizeAny(w, r, rc.principal, mapViewPermissions...) {
		return auth.RegencyScope{}, false
	}
	return h.regencyScope(w, r, rc.principal)
}

type mapGeometry struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

// mapPointProperties deliberately has no recipient name or NIK.
type mapPointProperties struct {
	SlotID         string   `json:"slot_id"`
	Code           string   `json:"code"`
	ScheduleID     string   `json:"schedule_id"`
	RegencyID      string   `json:"regency_id"`
	ZoneID         string   `json:"zone_id"`
	SiteID         string   `json:"site_id"`
	SiteName       string   `json:"site_name"`
	SiteOutlier    bool     `json:"site_outlier"`
	Stage          int      `json:"stage"`
	StageAt        []string `json:"stage_at"`
	LocationStatus string   `json:"location_status"`
}

type mapPointFeature struct {
	Type       string             `json:"type"`
	Geometry   mapGeometry        `json:"geometry"`
	Properties mapPointProperties `json:"properties"`
}

type mapPointsMeta struct {
	Total     int `json:"total"`
	Located   int `json:"located"`
	Unlocated int `json:"unlocated"`
	Outside   int `json:"outside"`
}

type mapPointsResponse struct {
	Type     string            `json:"type"`
	Features []mapPointFeature `json:"features"`
	Meta     mapPointsMeta     `json:"meta"`
}

func (h *Handler) handleMapPoints(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.DistributionMap == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.authorizeMap(w, r, rc)
	if !ok {
		return
	}
	query, ok := mapQueryFromRequest(w, r)
	if !ok {
		return
	}
	points, meta, err := h.deps.DistributionMap.Points(r.Context(), query, scope)
	if err != nil {
		writeMapServiceError(w, err)
		return
	}
	response := mapPointsResponse{
		Type:     "FeatureCollection",
		Features: make([]mapPointFeature, 0, len(points)),
		Meta:     mapPointsMeta{Total: meta.Total, Located: meta.Located, Unlocated: meta.Unlocated, Outside: meta.Outside},
	}
	for _, p := range points {
		stageAt := make([]string, 0, len(p.StageAt))
		for _, at := range p.StageAt {
			stageAt = append(stageAt, at.UTC().Format(time.RFC3339))
		}
		response.Features = append(response.Features, mapPointFeature{
			Type:     "Feature",
			Geometry: mapGeometry{Type: "Point", Coordinates: [2]float64{p.Lon, p.Lat}},
			Properties: mapPointProperties{
				SlotID: p.SlotID, Code: p.Code, ScheduleID: p.ScheduleID, RegencyID: p.RegencyID, ZoneID: p.ZoneID,
				SiteID: p.SiteID, SiteName: p.SiteName, SiteOutlier: p.SiteOutlier,
				Stage: p.Stage, StageAt: stageAt, LocationStatus: p.LocationStatus,
			},
		})
	}
	writeData(w, http.StatusOK, response)
}

type mapUnlocatedItem struct {
	SlotID    string `json:"slot_id"`
	Code      string `json:"code"`
	RegencyID string `json:"regency_id"`
	Stage     int    `json:"stage"`
}

func (h *Handler) handleMapUnlocated(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.DistributionMap == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.authorizeMap(w, r, rc)
	if !ok {
		return
	}
	query, ok := mapQueryFromRequest(w, r)
	if !ok {
		return
	}
	slots, total, err := h.deps.DistributionMap.UnlocatedSlots(r.Context(), query, scope)
	if err != nil {
		writeMapServiceError(w, err)
		return
	}
	items := make([]mapUnlocatedItem, 0, len(slots))
	for _, s := range slots {
		items = append(items, mapUnlocatedItem{SlotID: s.SlotID, Code: s.Code, RegencyID: s.RegencyID, Stage: s.Stage})
	}
	writeData(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type mapZoneProperties struct {
	ZoneID        string `json:"zone_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	IsPlaceholder bool   `json:"is_placeholder"`
	RegencyCount  int    `json:"regency_count"`
}

type mapZoneFeature struct {
	Type       string            `json:"type"`
	Geometry   json.RawMessage   `json:"geometry"`
	Properties mapZoneProperties `json:"properties"`
}

func (h *Handler) handleMapZones(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.DistributionMap == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.authorizeMap(w, r, rc)
	if !ok {
		return
	}
	programID := strings.TrimSpace(r.URL.Query().Get("program_id"))
	if programID == "" {
		writeMapValidation(w, "program_id", "Program wajib dipilih")
		return
	}
	zones, err := h.deps.DistributionMap.ZoneBoundaries(r.Context(), programID, scope)
	if err != nil {
		writeMapServiceError(w, err)
		return
	}
	features := make([]mapZoneFeature, 0, len(zones))
	for _, z := range zones {
		features = append(features, mapZoneFeature{
			Type: "Feature", Geometry: z.Geometry,
			Properties: mapZoneProperties{ZoneID: z.ZoneID, Code: z.Code, Name: z.Name, IsPlaceholder: z.IsPlaceholder, RegencyCount: z.RegencyCount},
		})
	}
	writeData(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": features})
}

// handleMapRegency serves GET /map/regencies/{regency_id}/boundary. A kabupaten without a boundary
// and one outside the caller's scope both answer 404.
func (h *Handler) handleMapRegency(w http.ResponseWriter, r *http.Request, rc requestContext, rest string) {
	if h.deps.DistributionMap == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.authorizeMap(w, r, rc)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "boundary" {
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
		return
	}
	feature, err := h.deps.DistributionMap.RegencyBoundary(r.Context(), parts[0], scope)
	if errors.Is(err, distmap.ErrBoundaryNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Batas wilayah tidak ditemukan")
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, feature)
}

// handleMapConfig serves the basemap configuration. Tile keys are public client keys, so the
// response is safe for any user who may view the map.
func (h *Handler) handleMapConfig(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if _, ok := h.authorizeMap(w, r, rc); !ok {
		return
	}
	config := h.deps.MapConfig
	if config.HybridOverlayTiles == nil {
		config.HybridOverlayTiles = []string{}
	}
	writeData(w, http.StatusOK, config)
}
