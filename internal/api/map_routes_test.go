package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/distmap"
)

type fakeDistributionMap struct {
	points     []distmap.Point
	unlocated  []distmap.UnlocatedSlot
	unlocTotal int
	zones      []distmap.ZoneBoundary
	boundary   json.RawMessage
	boundaryOK bool

	seenQuery     distmap.PointQuery
	seenScope     auth.RegencyScope
	seenProgram   string
	seenRegencyID string
	calls         int
}

func (f *fakeDistributionMap) Points(_ context.Context, q distmap.PointQuery, scope auth.RegencyScope) ([]distmap.Point, distmap.PointsMeta, error) {
	f.calls++
	f.seenQuery, f.seenScope = q, scope
	return f.points, distmap.PointsMeta{Total: 3, Located: 2, Unlocated: 1, Outside: 1}, nil
}

func (f *fakeDistributionMap) UnlocatedSlots(_ context.Context, q distmap.PointQuery, scope auth.RegencyScope) ([]distmap.UnlocatedSlot, int, error) {
	f.calls++
	f.seenQuery, f.seenScope = q, scope
	return f.unlocated, f.unlocTotal, nil
}

func (f *fakeDistributionMap) ZoneBoundaries(_ context.Context, programID string, scope auth.RegencyScope) ([]distmap.ZoneBoundary, error) {
	f.calls++
	f.seenProgram, f.seenScope = programID, scope
	return f.zones, nil
}

func (f *fakeDistributionMap) RegencyBoundary(_ context.Context, regencyID string, scope auth.RegencyScope) (json.RawMessage, error) {
	f.calls++
	f.seenRegencyID, f.seenScope = regencyID, scope
	if !scope.Allows(regencyID) || !f.boundaryOK {
		return nil, distmap.ErrBoundaryNotFound
	}
	return f.boundary, nil
}

func mapGet(t *testing.T, service *fakeDistributionMap, authService *fakeAuthService, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: validSessionToken})
	rec := httptest.NewRecorder()
	NewHandler(Dependencies{Auth: authService, DistributionMap: service, MapConfig: distmap.ClientConfig{
		StyleURL: "https://style.test/s", SatelliteTiles: "https://sat.test/{z}/{y}/{x}",
		HybridOverlayTiles: []string{"https://ov1.test/{z}/{y}/{x}"}, Attribution: "attr",
	}}).ServeHTTP(rec, req)
	return rec
}

func mapViewer(scope auth.RegencyScope) *fakeAuthService {
	return &fakeAuthService{principal: auth.Principal{UserID: "user-1"}, allowedPermissions: map[string]bool{"distribution.view": true}, regencyScope: scope}
}

func TestMapEndpointsRequireAViewPermission(t *testing.T) {
	denied := &fakeAuthService{principal: auth.Principal{UserID: "user-2"}}
	for _, target := range []string{
		"/api/v1/map/points", "/api/v1/map/unlocated", "/api/v1/map/zones?program_id=p",
		"/api/v1/map/regencies/r-1/boundary", "/api/v1/map/config",
	} {
		service := &fakeDistributionMap{}
		if rec := mapGet(t, service, denied, target); rec.Code != http.StatusForbidden || service.calls != 0 {
			t.Fatalf("%s: want 403 without a view permission and no service call, got %d calls=%d", target, rec.Code, service.calls)
		}
	}
}

func TestMapPointsRejectsInvalidBBoxAndStage(t *testing.T) {
	cases := []struct{ query, field string }{
		{"bbox=a,b", "bbox"},
		{"bbox=1,2,3", "bbox"},
		{"bbox=1,2,3,x", "bbox"},
		{"bbox=NaN,0,1,1", "bbox"},
		{"bbox=0,0,Inf,1", "bbox"},
		{"bbox=0,-Inf,1,1", "bbox"},
		{"bbox=109,-6,108,-5", "bbox"},
		{"bbox=108,-5,109,-6", "bbox"},
		{"stage=9", "stage"},
		{"stage=M6", "stage"},
		{"stage=0", "stage"},
		{"stage=x", "stage"},
	}
	for _, tc := range cases {
		service := &fakeDistributionMap{}
		rec := mapGet(t, service, mapViewer(auth.RegencyScope{Unrestricted: true}), "/api/v1/map/points?"+tc.query)
		var body struct {
			Error struct {
				Code   string            `json:"code"`
				Fields map[string]string `json:"fields"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusBadRequest || body.Error.Code != "validation_failed" || body.Error.Fields[tc.field] == "" || service.calls != 0 {
			t.Fatalf("%s: want 400 validation_failed on %s without a service call, got %d %s", tc.query, tc.field, rec.Code, rec.Body.String())
		}
	}
	// The same validation guards /map/unlocated.
	rec := mapGet(t, &fakeDistributionMap{}, mapViewer(auth.RegencyScope{Unrestricted: true}), "/api/v1/map/unlocated?stage=9")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unlocated stage=9: want 400, got %d", rec.Code)
	}
}

func TestMapPointsReturnsGeoJSONWithoutPersonalData(t *testing.T) {
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	service := &fakeDistributionMap{points: []distmap.Point{{
		SlotID: "slot-1", Code: "12", ScheduleID: "sch-1", RegencyID: "reg-1", ZoneID: "zone-1", SiteID: "hl:abc", SiteName: "Balai Desa",
		Lon: 108.19533, Lat: -6.45012, Stage: 2, StageAt: []time.Time{at, at.Add(time.Hour)}, LocationStatus: "inside", SiteOutlier: true,
	}}}
	viewer := mapViewer(auth.RegencyScope{RegencyIDs: []string{"reg-1"}})
	rec := mapGet(t, service, viewer, "/api/v1/map/points?regency_id=reg-1&program_id=prog-1&zone_id=zone-1&schedule_id=sch-1&search=Siti&bbox=108,-7,109,-6&stage=M2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.seenQuery.Filter.RegencyID != "reg-1" || service.seenQuery.Filter.ProgramID != "prog-1" || service.seenQuery.Filter.ZoneID != "zone-1" || service.seenQuery.Filter.ScheduleID != "sch-1" {
		t.Fatalf("filter not forwarded: %+v", service.seenQuery.Filter)
	}
	if b := service.seenQuery.BBox; b == nil || *b != [4]float64{108, -7, 109, -6} || service.seenQuery.Stage != 2 {
		t.Fatalf("bbox/stage not forwarded: %+v", service.seenQuery)
	}
	if len(service.seenScope.RegencyIDs) != 1 || service.seenScope.RegencyIDs[0] != "reg-1" {
		t.Fatalf("principal scope not forwarded: %+v", service.seenScope)
	}
	raw := rec.Body.String()
	for _, forbidden := range []string{"full_name", "nik", "Siti"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("point response must not carry %q: %s", forbidden, raw)
		}
	}
	var body struct {
		Data struct {
			Type     string `json:"type"`
			Features []struct {
				Type     string `json:"type"`
				Geometry struct {
					Type        string    `json:"type"`
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
				Properties map[string]any `json:"properties"`
			} `json:"features"`
			Meta map[string]int `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Type != "FeatureCollection" || len(body.Data.Features) != 1 {
		t.Fatalf("unexpected collection: %s", raw)
	}
	f := body.Data.Features[0]
	if f.Type != "Feature" || f.Geometry.Type != "Point" || f.Geometry.Coordinates[0] != 108.19533 || f.Geometry.Coordinates[1] != -6.45012 {
		t.Fatalf("unexpected geometry: %+v", f.Geometry)
	}
	p := f.Properties
	for key, want := range map[string]any{"slot_id": "slot-1", "code": "12", "schedule_id": "sch-1", "regency_id": "reg-1", "zone_id": "zone-1", "site_id": "hl:abc", "site_name": "Balai Desa", "site_outlier": true, "stage": float64(2), "location_status": "inside"} {
		if p[key] != want {
			t.Fatalf("property %s=%v, want %v", key, p[key], want)
		}
	}
	stageAt, _ := p["stage_at"].([]any)
	if len(stageAt) != 2 || stageAt[0] != "2026-11-02T03:00:00Z" {
		t.Fatalf("stage_at must be RFC 3339 UTC, got %v", p["stage_at"])
	}
	if len(p) != 11 {
		t.Fatalf("unexpected extra properties: %v", p)
	}
	if body.Data.Meta["total"] != 3 || body.Data.Meta["located"] != 2 || body.Data.Meta["unlocated"] != 1 || body.Data.Meta["outside"] != 1 {
		t.Fatalf("unexpected meta: %v", body.Data.Meta)
	}
}

func TestMapPointsEmptyIsAnEmptyFeatureArray(t *testing.T) {
	rec := mapGet(t, &fakeDistributionMap{}, mapViewer(auth.RegencyScope{Unrestricted: true}), "/api/v1/map/points")
	if !strings.Contains(rec.Body.String(), `"features":[]`) {
		t.Fatalf("no points must serialize as an empty array, got %s", rec.Body.String())
	}
}

func TestMapUnlocatedReturnsItemsAndTotal(t *testing.T) {
	service := &fakeDistributionMap{unlocated: []distmap.UnlocatedSlot{{SlotID: "s1", Code: "7", RegencyID: "r1", Stage: 3}}, unlocTotal: 812}
	rec := mapGet(t, service, mapViewer(auth.RegencyScope{Unrestricted: true}), "/api/v1/map/unlocated?program_id=p1&stage=M3")
	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
	}
	if body.Data.Total != 812 || len(body.Data.Items) != 1 || body.Data.Items[0]["slot_id"] != "s1" || body.Data.Items[0]["code"] != "7" || body.Data.Items[0]["regency_id"] != "r1" || body.Data.Items[0]["stage"] != float64(3) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if service.seenQuery.Filter.ProgramID != "p1" || service.seenQuery.Stage != 3 {
		t.Fatalf("query not forwarded: %+v", service.seenQuery)
	}
}

func TestMapZonesRequiresProgramAndRespectsScope(t *testing.T) {
	service := &fakeDistributionMap{zones: []distmap.ZoneBoundary{{ZoneID: "z1", Code: "A", Name: "Zona A", IsPlaceholder: false, RegencyCount: 2, Geometry: json.RawMessage(`{"type":"MultiPolygon","coordinates":[]}`)}}}
	viewer := mapViewer(auth.RegencyScope{RegencyIDs: []string{"r1"}})
	if rec := mapGet(t, service, viewer, "/api/v1/map/zones"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "program_id") {
		t.Fatalf("missing program_id: want 400, got %d %s", rec.Code, rec.Body.String())
	}
	rec := mapGet(t, service, viewer, "/api/v1/map/zones?program_id=p1")
	if rec.Code != http.StatusOK || service.seenProgram != "p1" || len(service.seenScope.RegencyIDs) != 1 {
		t.Fatalf("status=%d program=%q scope=%+v", rec.Code, service.seenProgram, service.seenScope)
	}
	var body struct {
		Data struct {
			Type     string `json:"type"`
			Features []struct {
				Type       string          `json:"type"`
				Geometry   json.RawMessage `json:"geometry"`
				Properties map[string]any  `json:"properties"`
			} `json:"features"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Type != "FeatureCollection" || len(body.Data.Features) != 1 {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	p := body.Data.Features[0].Properties
	if p["zone_id"] != "z1" || p["code"] != "A" || p["name"] != "Zona A" || p["is_placeholder"] != false || p["regency_count"] != float64(2) {
		t.Fatalf("unexpected zone properties: %v", p)
	}
	if !strings.Contains(string(body.Data.Features[0].Geometry), "MultiPolygon") {
		t.Fatalf("geometry missing: %s", body.Data.Features[0].Geometry)
	}
}

func TestMapRegencyBoundary(t *testing.T) {
	service := &fakeDistributionMap{boundaryOK: true, boundary: json.RawMessage(`{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[]},"properties":{"regency_id":"r1"}}`)}
	viewer := mapViewer(auth.RegencyScope{RegencyIDs: []string{"r1"}})
	rec := mapGet(t, service, viewer, "/api/v1/map/regencies/r1/boundary")
	if rec.Code != http.StatusOK || service.seenRegencyID != "r1" || !strings.Contains(rec.Body.String(), `"type":"Feature"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := mapGet(t, service, viewer, "/api/v1/map/regencies/r2/boundary"); rec.Code != http.StatusNotFound {
		t.Fatalf("out-of-scope regency: want 404, got %d", rec.Code)
	}
	service.boundaryOK = false
	if rec := mapGet(t, service, viewer, "/api/v1/map/regencies/r1/boundary"); rec.Code != http.StatusNotFound {
		t.Fatalf("regency without a boundary: want 404, got %d", rec.Code)
	}
	if rec := mapGet(t, service, viewer, "/api/v1/map/regencies/r1/other"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown sub-path: want 404, got %d", rec.Code)
	}
}

func TestMapConfig(t *testing.T) {
	rec := mapGet(t, &fakeDistributionMap{}, mapViewer(auth.RegencyScope{}), "/api/v1/map/config")
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
	}
	d := body.Data
	overlays, _ := d["hybrid_overlay_tiles"].([]any)
	if d["style_url"] != "https://style.test/s" || d["satellite_tiles"] != "https://sat.test/{z}/{y}/{x}" || len(overlays) != 1 || d["attribution"] != "attr" || d["api_key"] != "" || len(d) != 5 {
		t.Fatalf("unexpected config: %v", d)
	}
}
