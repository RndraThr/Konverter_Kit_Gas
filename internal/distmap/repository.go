package distmap

import (
	"context"
	"fmt"
	"time"

	"konkit/internal/auth"
	"konkit/internal/recipients"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool  *pgxpool.Pool
	zones zoneCache
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// slotsSQL selects every non-cancelled slot matching the filter with its milestone, the five
// milestone times, the photo location it is drawn at and the location status. recipientFrom and
// recipientWhere ($1..$12) come from recipients.RecipientFilterSQL; $13..$20 are filled by
// slotArgs. The result is the CTE chain up to "staged"; callers append their final SELECT.
func slotsSQL(recipientFrom, recipientWhere string) string {
	return `
WITH allocated AS (
	-- Slots holding a recipient, selected by the Data Penerima filter itself so a filter means
	-- the same thing on the map as in the list. A replaced recipient's old allocation no longer
	-- owns the slot (allocation_id moved to the successor), so each slot appears once.
	SELECT dr.id AS slot_id, z.id AS zone_id, (` + recipients.EvidenceStatusSQL + `) AS evidence_status
	` + recipientFrom + recipientWhere + `
	  AND dr.id IS NOT NULL AND ps.status <> 'cancelled'
),
unallocated AS (
	-- Open slots have no allocation, so the recipient filter cannot see them. As in the slot
	-- counts of MapRegions they are kept when their schedule passes the schedule-level filters;
	-- $13 drops them when a recipient-level filter (search, status, district, evidence) is set.
	SELECT ds.id AS slot_id, z.id AS zone_id, 'not-configured' AS evidence_status
	FROM distribution_slots ds
	JOIN program_schedules s ON s.id = ds.schedule_id
	JOIN programs prog ON prog.id = s.program_id
	LEFT JOIN program_regency_assignments pra ON pra.program_id = prog.id AND pra.regency_id = s.regency_id
	LEFT JOIN program_zones z ON z.id = pra.zone_id AND z.program_id = prog.id
	WHERE $13 AND ds.allocation_id IS NULL AND s.status <> 'cancelled'
	  AND ($14 = '' OR s.regency_id::text = $14)
	  AND ($15 = '' OR prog.id::text = $15)
	  AND ($16 = '' OR prog.program_type = $16)
	  AND ($17 = '' OR s.id::text = $17)
	  AND ($18 = '' OR z.id::text = $18)
	  AND ($19 OR s.regency_id::text = ANY($20))
),
slots AS (
	SELECT ds.id, ds.slot_number, ds.schedule_id, s.regency_id, picked.zone_id, picked.evidence_status,
		ds.status, ds.handover_location_id, hl.name AS site_name,
		ds.created_at, ds.linked_at, ds.distributed_at, ds.completed_at, ds.updated_at
	FROM (SELECT * FROM allocated UNION ALL SELECT * FROM unallocated) picked
	JOIN distribution_slots ds ON ds.id = picked.slot_id
	JOIN program_schedules s ON s.id = ds.schedule_id
	LEFT JOIN bast_handover_locations hl ON hl.id = ds.handover_location_id
	WHERE ds.status <> 'cancelled'
),
staged AS (
	SELECT slots.*,
		-- Milestones require each other, so the highest one reached is the stage.
		CASE
			WHEN slots.status = 'completed' AND slots.evidence_status = 'complete' AND ba.created_at IS NOT NULL THEN 5
			WHEN slots.status = 'completed' AND slots.evidence_status = 'complete' THEN 4
			WHEN slots.status = 'completed' THEN 3
			WHEN slots.status = 'linked' THEN 2
			ELSE 1
		END AS stage,
		slots.created_at AS stage1_at,
		COALESCE(slots.linked_at, slots.distributed_at, slots.completed_at, slots.updated_at) AS stage2_at,
		COALESCE(slots.completed_at, slots.distributed_at, slots.updated_at) AS stage3_at,
		COALESCE(evidence.last_upload, slots.completed_at, slots.updated_at) AS stage4_at,
		ba.created_at AS stage5_at,
		loc.geom,
		CASE
			WHEN loc.geom IS NULL THEN ''
			WHEN NOT EXISTS (SELECT 1 FROM regency_boundaries rb WHERE rb.regency_id = slots.regency_id) THEN 'unmatched'
			-- ST_Covers answers the common case cheaply; the 500 m geography buffer absorbs boundary
			-- simplification and GPS error near the border.
			WHEN EXISTS (
				SELECT 1 FROM regency_boundaries rb
				WHERE rb.regency_id = slots.regency_id
				  AND (ST_Covers(rb.geom, loc.geom) OR ST_DWithin(rb.geom::geography, loc.geom::geography, 500))
			) THEN 'inside'
			ELSE 'outside'
		END AS location_status
	FROM slots
	LEFT JOIN LATERAL (
		SELECT max(m.uploaded_at) AS last_upload
		FROM documentation_slots d
		JOIN media_files m ON m.documentation_slot_id = d.id AND m.status = 'accepted'
		WHERE d.distribution_slot_id = slots.id AND d.is_required
	) evidence ON true
	-- Only the current final BA perorangan counts; superseded or stale revisions do not.
	LEFT JOIN bast_individual_documents ba
		ON ba.distribution_slot_id = slots.id AND ba.document_type = 'individual' AND ba.status = 'final'
	-- The photo the slot is drawn at (spec §4.1): handover-stage photos first, then the documentation
	-- slot with the smallest sort_order, then the earliest capture. media_files.geom is already NULL
	-- for missing or 0,0 coordinates; a longitude in -11..6 is an Indonesian latitude typed into the
	-- longitude field (swapped pair) and is treated as no location too.
	LEFT JOIN LATERAL (
		SELECT m.geom
		FROM documentation_slots d
		JOIN media_files m ON m.documentation_slot_id = d.id
		WHERE d.distribution_slot_id = slots.id
		  AND m.status = 'accepted' AND m.geom IS NOT NULL
		  AND NOT ST_X(m.geom) BETWEEN -11 AND 6
		ORDER BY (d.stage = 'penyerahan') DESC, d.sort_order, m.captured_at NULLS LAST, m.uploaded_at, m.id
		LIMIT 1
	) loc ON true
)`
}

func slotArgs(q PointQuery, scope auth.RegencyScope) (from, where string, args []any) {
	from, where, args = recipients.RecipientFilterSQL(q.Filter, scope)
	f := q.Filter
	includeOpen := f.Search == "" && f.AllocationStatus == "" && f.DistributionStatus == "" && f.District == "" && f.EvidenceStatus == ""
	args = append(args, includeOpen, f.RegencyID, f.ProgramID, f.ProgramType, f.ScheduleID, f.ZoneID, scope.Unrestricted, scope.RegencyIDs)
	return from, where, args
}

// pointsSelect groups located slots into sites and returns every staged slot (located or not) so
// the caller can count meta; bbox and stage are applied afterwards so that site membership and the
// titik serah median never depend on the viewport or the stage filter.
const pointsSelect = `,
located AS (SELECT * FROM staged WHERE geom IS NOT NULL),
geo_sites AS (
	-- Slots without a titik serah: photos within 30 m of each other in the same schedule form one
	-- unnamed site. EPSG:3857 metres stretch by 1/cos(lat), about 1% at Indonesian latitudes.
	SELECT id, ST_ClusterDBSCAN(ST_Transform(geom, 3857), eps := 30, minpoints := 1)
		OVER (PARTITION BY schedule_id) AS cluster
	FROM located WHERE handover_location_id IS NULL
),
hl_centers AS (
	-- A titik serah has no coordinates of its own; its position is the median of its members.
	SELECT handover_location_id,
		ST_SetSRID(ST_MakePoint(
			percentile_cont(0.5) WITHIN GROUP (ORDER BY ST_X(geom)),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY ST_Y(geom))), 4326) AS center
	FROM located WHERE handover_location_id IS NOT NULL
	GROUP BY handover_location_id
)
SELECT st.id::text, st.slot_number::text, st.schedule_id::text, st.regency_id::text, COALESCE(st.zone_id::text, ''),
	CASE
		WHEN st.geom IS NULL THEN ''
		WHEN st.handover_location_id IS NOT NULL THEN 'hl:' || st.handover_location_id::text
		ELSE 'geo:' || st.schedule_id::text || ':' || gs.cluster::text
	END,
	COALESCE(st.site_name, ''),
	COALESCE(ST_Distance(st.geom::geography, hc.center::geography) > 1000, false),
	ST_X(st.geom), ST_Y(st.geom),
	st.stage, st.stage1_at, st.stage2_at, st.stage3_at, st.stage4_at, st.stage5_at,
	st.location_status
FROM staged st
LEFT JOIN geo_sites gs ON gs.id = st.id
LEFT JOIN hl_centers hc ON hc.handover_location_id = st.handover_location_id
ORDER BY st.schedule_id, st.slot_number`

// Points returns the located slots matching q within q.BBox, and counts for everything matching
// the filter and stage (located or not, inside the bbox or not).
func (r *Repository) Points(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]Point, PointsMeta, error) {
	from, where, args := slotArgs(q, scope)
	rows, err := r.pool.Query(ctx, slotsSQL(from, where)+pointsSelect, args...)
	if err != nil {
		return nil, PointsMeta{}, fmt.Errorf("query map points: %w", err)
	}
	defer rows.Close()

	points := []Point{}
	var meta PointsMeta
	for rows.Next() {
		var p Point
		var lon, lat *float64
		var at [5]*time.Time
		if err := rows.Scan(&p.SlotID, &p.Code, &p.ScheduleID, &p.RegencyID, &p.ZoneID, &p.SiteID, &p.SiteName, &p.SiteOutlier,
			&lon, &lat, &p.Stage, &at[0], &at[1], &at[2], &at[3], &at[4], &p.LocationStatus); err != nil {
			return nil, PointsMeta{}, fmt.Errorf("scan map point: %w", err)
		}
		if q.Stage != 0 && p.Stage != q.Stage {
			continue
		}
		meta.Total++
		if lon == nil || lat == nil {
			meta.Unlocated++
			continue
		}
		meta.Located++
		if p.LocationStatus == LocationOutside {
			meta.Outside++
		}
		p.Lon, p.Lat = *lon, *lat
		if b := q.BBox; b != nil && (p.Lon < b[0] || p.Lat < b[1] || p.Lon > b[2] || p.Lat > b[3]) {
			continue
		}
		p.StageAt = make([]time.Time, 0, p.Stage)
		for _, t := range at[:p.Stage] {
			if t != nil {
				p.StageAt = append(p.StageAt, t.UTC())
			}
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, PointsMeta{}, fmt.Errorf("iterate map points: %w", err)
	}
	return points, meta, nil
}

// UnlocatedLimit caps the rows UnlocatedSlots returns; the total says how many there are in all.
const UnlocatedLimit = 500

// UnlocatedSlots lists the first UnlocatedLimit slots matching the filter and stage that have no
// usable photo location, ordered by kabupaten name, schedule and slot number, plus the number of
// such slots in all. The bounding box does not apply because they have no position.
func (r *Repository) UnlocatedSlots(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]UnlocatedSlot, int, error) {
	from, where, args := slotArgs(q, scope)
	rows, err := r.pool.Query(ctx, slotsSQL(from, where)+`
SELECT st.id::text, st.slot_number::text, st.regency_id::text, st.stage, count(*) OVER ()
FROM staged st
JOIN regencies rg ON rg.id = st.regency_id
WHERE st.geom IS NULL AND ($21 = 0 OR st.stage = $21)
ORDER BY rg.name, st.regency_id, st.schedule_id, st.slot_number
LIMIT $22`, append(args, q.Stage, UnlocatedLimit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query unlocated slots: %w", err)
	}
	defer rows.Close()
	items := []UnlocatedSlot{}
	total := 0
	for rows.Next() {
		var item UnlocatedSlot
		if err := rows.Scan(&item.SlotID, &item.Code, &item.RegencyID, &item.Stage, &total); err != nil {
			return nil, 0, fmt.Errorf("scan unlocated slot: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate unlocated slots: %w", err)
	}
	return items, total, nil
}
