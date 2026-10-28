// Package distmap computes the distribution map: one point per nomor bagi (distribution slot)
// with its milestone, the photo it is located by, whether that location lies in the schedule's
// kabupaten, and the titik serah (handover site) it belongs to.
package distmap

import (
	"time"

	"konkit/internal/recipients"
)

// Location statuses of a point (spec §4.2).
const (
	LocationInside    = "inside"
	LocationOutside   = "outside"
	LocationUnmatched = "unmatched"
)

// Milestones (spec §3). A slot sits at exactly one stage; StageAt holds one time per reached stage.
const (
	StageScheduled = 1 // Terjadwal
	StageLinked    = 2 // Penerima tertaut
	StageDelivered = 3 // Diserahkan
	StageEvidenced = 4 // Bukti lengkap
	StageSigned    = 5 // BA ditandatangani
)

// Point is one located distribution slot. It deliberately carries no recipient name or NIK.
type Point struct {
	SlotID, Code, ScheduleID, RegencyID, ZoneID, SiteID string
	Lon, Lat                                            float64
	Stage                                               int
	StageAt                                             []time.Time
	LocationStatus                                      string
	// SiteName is the titik serah name for "hl:" sites and empty for "geo:" sites.
	SiteName string
	// SiteOutlier marks a titik serah member more than 1 km from the median of its members.
	SiteOutlier bool
}

// PointsMeta counts the slots matching the filter and stage, regardless of the bounding box:
// Total = Located + Unlocated, Outside ⊆ Located.
type PointsMeta struct {
	Total, Located, Unlocated, Outside int
}

// PointQuery is the Data Penerima filter plus the map-only bbox and stage filters.
type PointQuery struct {
	Filter recipients.Filter
	// BBox is minLon, minLat, maxLon, maxLat; nil means no bounding box.
	BBox *[4]float64
	// Stage limits the result to one milestone (1..5); 0 means all.
	Stage int
}

// ClientConfig is the basemap configuration handed to the browser (GET /api/v1/map/config), so the
// tile provider can change without rebuilding the frontend. APIKey is a public client key.
type ClientConfig struct {
	StyleURL           string   `json:"style_url"`
	SatelliteTiles     string   `json:"satellite_tiles"`
	HybridOverlayTiles []string `json:"hybrid_overlay_tiles"`
	Attribution        string   `json:"attribution"`
	APIKey             string   `json:"api_key"`
}

// UnlocatedSlot is a slot matching the query that has no accepted photo with usable coordinates.
type UnlocatedSlot struct {
	SlotID, Code, RegencyID string
	Stage                   int
}
