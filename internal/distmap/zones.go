package distmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// ErrBoundaryNotFound means the kabupaten has no boundary, does not exist, or lies outside the
// caller's scope; the three are indistinguishable on purpose.
var ErrBoundaryNotFound = errors.New("regency boundary not found")

// ZoneBoundary is the merged boundary of one zone's kabupaten (within the caller's scope).
type ZoneBoundary struct {
	ZoneID        string
	Code          string
	Name          string
	IsPlaceholder bool
	// RegencyCount counts the zone's kabupaten inside the caller's scope.
	RegencyCount int
	// Geometry is a GeoJSON geometry, already simplified.
	Geometry json.RawMessage
}

// zoneCache holds computed zone boundaries per programme and scope. Boundaries are static; only the
// kabupaten-to-zone assignments (and zone names) change, and the programs service reports that
// through InvalidateZones.
type zoneCache struct {
	mu      sync.Mutex
	entries map[string]map[string][]ZoneBoundary // programID -> scopeKey -> zones
	// generation guards against a slow load storing a result computed before an invalidation.
	generation map[string]uint64
}

// maxScopesPerProgram bounds the cache: each distinct scope of a scoped user is one entry.
const maxScopesPerProgram = 64

func (c *zoneCache) get(programID, scopeKey string) ([]ZoneBoundary, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	zones, ok := c.entries[programID][scopeKey]
	return zones, c.generation[programID], ok
}

func (c *zoneCache) put(programID, scopeKey string, generation uint64, zones []ZoneBoundary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation[programID] != generation {
		return // invalidated while loading: the result may already be stale
	}
	if c.entries == nil {
		c.entries = map[string]map[string][]ZoneBoundary{}
	}
	if len(c.entries[programID]) >= maxScopesPerProgram {
		delete(c.entries, programID)
	}
	if c.entries[programID] == nil {
		c.entries[programID] = map[string][]ZoneBoundary{}
	}
	c.entries[programID][scopeKey] = zones
}

// InvalidateZones drops the cached zone boundaries of a programme; call it whenever the programme's
// zones or kabupaten-to-zone assignments change.
func (r *Repository) InvalidateZones(programID string) {
	r.zones.mu.Lock()
	defer r.zones.mu.Unlock()
	delete(r.zones.entries, programID)
	if r.zones.generation == nil {
		r.zones.generation = map[string]uint64{}
	}
	r.zones.generation[programID]++
}

func scopeKey(scope auth.RegencyScope) string {
	if scope.Unrestricted {
		return "*"
	}
	ids := append([]string(nil), scope.RegencyIDs...)
	sort.Strings(ids)
	return "r:" + strings.Join(ids, ",")
}

// ZoneBoundaries returns one merged, simplified boundary per zone of the programme that has at least
// one kabupaten with a boundary inside scope, ordered by zone sort order. Results are cached per
// programme and scope until InvalidateZones.
func (r *Repository) ZoneBoundaries(ctx context.Context, programID string, scope auth.RegencyScope) ([]ZoneBoundary, error) {
	key := scopeKey(scope)
	zones, generation, ok := r.zones.get(programID, key)
	if ok {
		return zones, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT z.id::text, z.code, z.name, z.is_placeholder,
			count(DISTINCT pra.regency_id)::int,
			ST_AsGeoJSON(ST_SimplifyPreserveTopology(ST_Union(rb.geom), 0.005), 5)
		FROM program_zones z
		JOIN program_regency_assignments pra ON pra.zone_id = z.id AND pra.program_id = z.program_id
		JOIN regency_boundaries rb ON rb.regency_id = pra.regency_id
		WHERE z.program_id = $1::uuid AND ($2 OR pra.regency_id::text = ANY($3))
		GROUP BY z.id, z.code, z.name, z.is_placeholder, z.sort_order
		ORDER BY z.sort_order, z.code
	`, programID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("query zone boundaries: %w", err)
	}
	defer rows.Close()
	zones = []ZoneBoundary{}
	for rows.Next() {
		var zone ZoneBoundary
		var geometry *string
		if err := rows.Scan(&zone.ZoneID, &zone.Code, &zone.Name, &zone.IsPlaceholder, &zone.RegencyCount, &geometry); err != nil {
			return nil, fmt.Errorf("scan zone boundary: %w", err)
		}
		if geometry == nil {
			continue
		}
		zone.Geometry = json.RawMessage(*geometry)
		zones = append(zones, zone)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate zone boundaries: %w", err)
	}
	r.zones.put(programID, key, generation, zones)
	return zones, nil
}

// RegencyBoundary returns one kabupaten's boundary as a GeoJSON Feature (properties regency_id and
// name). A kabupaten outside scope answers like one without a boundary.
func (r *Repository) RegencyBoundary(ctx context.Context, regencyID string, scope auth.RegencyScope) (json.RawMessage, error) {
	if !scope.Allows(regencyID) {
		return nil, ErrBoundaryNotFound
	}
	var name string
	var geometry *string
	err := r.pool.QueryRow(ctx, `
		SELECT r.name, ST_AsGeoJSON(ST_Union(rb.geom), 5)
		FROM regency_boundaries rb
		JOIN regencies r ON r.id = rb.regency_id
		WHERE rb.regency_id::text = $1
		GROUP BY r.name
	`, regencyID).Scan(&name, &geometry)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && geometry == nil) {
		return nil, ErrBoundaryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query regency boundary: %w", err)
	}
	feature, err := json.Marshal(struct {
		Type       string          `json:"type"`
		Geometry   json.RawMessage `json:"geometry"`
		Properties map[string]any  `json:"properties"`
	}{"Feature", json.RawMessage(*geometry), map[string]any{"regency_id": regencyID, "name": name}})
	if err != nil {
		return nil, fmt.Errorf("encode regency boundary: %w", err)
	}
	return feature, nil
}
