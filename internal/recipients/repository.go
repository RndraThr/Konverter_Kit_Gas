package recipients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/programs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const evidenceStatusSQL = `CASE
	WHEN COALESCE(evidence.total_slots, 0) = 0 THEN 'not-configured'
	WHEN COALESCE(evidence.required_complete, 0) = COALESCE(evidence.required_total, 0) THEN 'complete'
	WHEN COALESCE(evidence.accepted_files, 0) = 0 THEN 'empty'
	ELSE 'partial'
END`

const recipientSelect = `
SELECT
	pa.id::text, pa.distribution_number, pa.status,
	CASE WHEN dr.status IN ('completed','cancelled') THEN dr.status WHEN dr.status IS NOT NULL THEN 'draft' ELSE NULL END,
	p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
	COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''),
	prog.id::text, prog.name, prog.program_type,
	COALESCE(z.id::text,''), COALESCE(z.code,''), COALESCE(z.name,''),
	r.id::text, r.name, r.document_code,
	ps.id::text, ps.name,
	COALESCE(evidence.evidence_slots, '[]'::jsonb),
	replaced_by.payload, replaces.payload,
	pa.created_at, pa.updated_at` + recipientFrom

const recipientFrom = `
FROM package_allocations pa
JOIN candidate_nominations cn ON cn.id = pa.nomination_id
JOIN program_schedules ps ON ps.id = pa.schedule_id
JOIN programs prog ON prog.id = ps.program_id
JOIN regencies r ON r.id = ps.regency_id
LEFT JOIN program_regency_assignments pra ON pra.program_id = prog.id AND pra.regency_id = r.id
LEFT JOIN program_zones z ON z.id = pra.zone_id AND z.program_id = prog.id
JOIN people p ON p.id = COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)
LEFT JOIN distribution_slots dr ON dr.allocation_id = pa.id
LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id
	AND psi.identifier_type = CASE prog.program_type WHEN 'farmer' THEN 'farmer_card' ELSE 'kusuka' END
LEFT JOIN LATERAL (
	SELECT
	jsonb_agg(
		jsonb_build_object(
			'slot_code', slot.slot_code,
			'label', slot.label_snapshot,
			'is_required', slot.is_required,
			'min_files', slot.min_files,
			'accepted_files', slot.accepted_files,
			'complete', slot.accepted_files >= slot.min_files
		)
		ORDER BY slot.sort_order, slot.slot_code
	) AS evidence_slots,
	count(*)::int AS total_slots,
	count(*) FILTER (WHERE slot.is_required)::int AS required_total,
	count(*) FILTER (WHERE slot.is_required AND slot.accepted_files >= slot.min_files)::int AS required_complete,
	COALESCE(sum(slot.accepted_files), 0)::int AS accepted_files
	FROM (
		SELECT s.slot_code, s.label_snapshot, s.is_required, s.min_files, s.sort_order,
			count(m.id) FILTER (WHERE m.status = 'accepted')::int AS accepted_files
		FROM documentation_slots s
		LEFT JOIN media_files m ON m.documentation_slot_id = s.id
		WHERE s.distribution_slot_id = dr.id
		GROUP BY s.id, s.slot_code, s.label_snapshot, s.is_required, s.min_files, s.sort_order
	) slot
) evidence ON true
LEFT JOIN LATERAL (
	SELECT jsonb_build_object(
		'full_name', successor.full_name,
		'nik', COALESCE(successor.nik, ''),
		'reason', rr.reason,
		'replaced_at', rr.replaced_at
	) AS payload
	FROM recipient_replacements rr
	JOIN people successor ON successor.id = rr.new_person_id
	WHERE rr.old_person_id = p.id
	ORDER BY rr.replaced_at DESC, rr.id DESC
	LIMIT 1
) replaced_by ON true
LEFT JOIN LATERAL (
	SELECT jsonb_build_object(
		'full_name', predecessor.full_name,
		'nik', COALESCE(predecessor.nik, ''),
		'reason', rr.reason,
		'replaced_at', rr.replaced_at
	) AS payload
	FROM recipient_replacements rr
	JOIN people predecessor ON predecessor.id = rr.old_person_id
	WHERE rr.new_person_id = p.id
	ORDER BY rr.replaced_at ASC, rr.id ASC
	LIMIT 1
) replaces ON true`

const recipientWhere = `
WHERE ($1 = '%%' OR p.full_name ILIKE $1 OR p.nik ILIKE $1 OR psi.normalized_value ILIKE $1)
  AND ($2 = '' OR r.id::text = $2)
  AND ($3 = '' OR prog.id::text = $3)
  AND ($4 = '' OR prog.program_type = $4)
  AND (($5 = '' AND pa.status != 'cancelled') OR ($5 != '' AND pa.status = $5))
  AND ($6 = '' OR ($6 = 'draft' AND (dr.status IS NULL OR dr.status NOT IN ('completed','cancelled'))) OR dr.status = $6)
  AND ($7 = '' OR ps.id::text = $7)
  AND ($8 = '' OR p.district ILIKE $8)
  AND ($9 = '' OR (` + evidenceStatusSQL + `) = $9)
  AND ($10 OR ps.regency_id::text = ANY($11))
  AND ($12 = '' OR z.id::text = $12)`

func recipientOrder(filter Filter) string {
	columns := map[string]string{
		"created_at":          "pa.created_at",
		"distribution_number": "pa.distribution_number",
		"full_name":           "p.full_name",
		"nik":                 "p.nik",
		"district":            "p.district",
		"regency":             "r.name",
		"program":             "prog.name",
		"zone":                "z.sort_order",
		"schedule":            "ps.name",
		"allocation_status":   "pa.status",
		"distribution_status": "dr.status",
		"evidence": `CASE (` + evidenceStatusSQL + `)
			WHEN 'not-configured' THEN 0 WHEN 'empty' THEN 1
			WHEN 'partial' THEN 2 WHEN 'complete' THEN 3 ELSE 0 END`,
	}
	column, ok := columns[filter.SortBy]
	if !ok {
		column = columns["created_at"]
	}
	direction := "DESC"
	if strings.EqualFold(filter.SortDirection, "asc") {
		direction = "ASC"
	}
	return " ORDER BY " + column + " " + direction + " NULLS LAST, pa.id DESC"
}

func recipientFilterArgs(filter Filter, scope auth.RegencyScope) []any {
	return []any{
		"%" + filter.Search + "%",
		filter.RegencyID,
		filter.ProgramID,
		filter.ProgramType,
		filter.AllocationStatus,
		filter.DistributionStatus,
		filter.ScheduleID,
		filter.District,
		filter.EvidenceStatus,
		scope.Unrestricted,
		scope.RegencyIDs,
		filter.ZoneID,
	}
}

func scanRecipient(row pgx.Row) (Recipient, error) {
	var item Recipient
	var evidenceJSON, replacedByJSON, replacesJSON []byte
	if err := row.Scan(
		&item.AllocationID, &item.DistributionNumber, &item.AllocationStatus,
		&item.DistributionStatus,
		&item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier,
		&item.Address, &item.Village, &item.District, &item.PhoneNumber,
		&item.ProgramID, &item.ProgramName, &item.ProgramType,
		&item.ZoneID, &item.ZoneCode, &item.ZoneName,
		&item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode,
		&item.ScheduleID, &item.ScheduleName,
		&evidenceJSON,
		&replacedByJSON, &replacesJSON,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Recipient{}, fmt.Errorf("scan recipient: %w", err)
	}
	if err := json.Unmarshal(evidenceJSON, &item.EvidenceSlots); err != nil {
		return Recipient{}, fmt.Errorf("decode recipient evidence: %w", err)
	}
	if item.EvidenceSlots == nil {
		item.EvidenceSlots = []EvidenceSlotSummary{}
	}
	if len(replacedByJSON) > 0 {
		var summary ReplacementSummary
		if err := json.Unmarshal(replacedByJSON, &summary); err != nil {
			return Recipient{}, fmt.Errorf("decode replaced_by summary: %w", err)
		}
		item.ReplacedBy = &summary
	}
	if len(replacesJSON) > 0 {
		var summary ReplacementSummary
		if err := json.Unmarshal(replacesJSON, &summary); err != nil {
			return Recipient{}, fmt.Errorf("decode replaces summary: %w", err)
		}
		item.Replaces = &summary
	}
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	args := recipientFilterArgs(filter, scope)

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) "+recipientFrom+recipientWhere, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count recipients: %w", err)
	}

	query := recipientSelect + recipientWhere + recipientOrder(filter)
	queryArgs := args
	if !filter.All {
		query += " LIMIT $13 OFFSET $14"
		queryArgs = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	}
	rows, err := r.pool.Query(ctx, query, queryArgs...)
	if err != nil {
		return Page{}, fmt.Errorf("list recipients: %w", err)
	}
	defer rows.Close()
	items := []Recipient{}
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate recipients: %w", err)
	}
	pageSize := filter.PageSize
	if filter.All {
		pageSize = len(items)
	}
	return Page{Items: items, Page: filter.Page, PageSize: pageSize, All: filter.All, Total: total}, nil
}

func (r *Repository) Stats(ctx context.Context, filter Filter, scope auth.RegencyScope) (Stats, error) {
	rows, err := r.pool.Query(ctx, `SELECT pa.status, (`+evidenceStatusSQL+`) AS evidence_status, count(*) `+
		recipientFrom+recipientWhere+` GROUP BY pa.status, evidence_status`, recipientFilterArgs(filter, scope)...)
	if err != nil {
		return Stats{}, fmt.Errorf("stats recipients: %w", err)
	}
	defer rows.Close()
	stats := Stats{ByAllocationStatus: map[string]int64{}, ByEvidenceStatus: map[string]int64{}}
	for rows.Next() {
		var status, evidenceStatus string
		var count int64
		if err := rows.Scan(&status, &evidenceStatus, &count); err != nil {
			return Stats{}, fmt.Errorf("scan recipient stats: %w", err)
		}
		stats.ByAllocationStatus[status] = count
		stats.ByEvidenceStatus[evidenceStatus] += count
		stats.Total += count
	}
	return stats, rows.Err()
}

// MapRegions aggregates recipients, evidence completeness, and distribution progress per
// kabupaten/kota. Recipients and evidence reuse recipientFrom/recipientWhere, so a filter means
// exactly the same thing on the map as it does in the Data Penerima list. Slots are counted
// separately because an open slot has no allocation yet and would otherwise be invisible.
func (r *Repository) MapRegions(ctx context.Context, filter Filter, scope auth.RegencyScope) (MapData, error) {
	regions := map[string]*MapRegion{}
	order := []string{}

	recipientRows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.name, r.province_name, r.document_code,
			count(*),
			count(*) FILTER (WHERE pa.status = 'candidate'),
			count(*) FILTER (WHERE pa.status = 'ready'),
			count(*) FILTER (WHERE pa.status = 'distributed'),
			count(*) FILTER (WHERE pa.status = 'needs_review'),
			count(*) FILTER (WHERE pa.status = 'replaced'),
			count(*) FILTER (WHERE (`+evidenceStatusSQL+`) = 'complete'),
			count(*) FILTER (WHERE (`+evidenceStatusSQL+`) = 'partial'),
			count(*) FILTER (WHERE (`+evidenceStatusSQL+`) = 'empty'),
			count(*) FILTER (WHERE (`+evidenceStatusSQL+`) = 'not-configured')
		`+recipientFrom+recipientWhere+`
		GROUP BY r.id, r.name, r.province_name, r.document_code
		ORDER BY r.name
	`, recipientFilterArgs(filter, scope)...)
	if err != nil {
		return MapData{}, fmt.Errorf("aggregate map recipients: %w", err)
	}
	for recipientRows.Next() {
		var item MapRegion
		if err := recipientRows.Scan(
			&item.RegencyID, &item.RegencyName, &item.ProvinceName, &item.DocumentCode,
			&item.Recipients, &item.Candidate, &item.Ready, &item.Distributed, &item.NeedsReview, &item.Replaced,
			&item.EvidenceComplete, &item.EvidencePartial, &item.EvidenceEmpty, &item.EvidenceNotConfigured,
		); err != nil {
			recipientRows.Close()
			return MapData{}, fmt.Errorf("scan map recipient aggregate: %w", err)
		}
		copied := item
		regions[item.RegencyID] = &copied
		order = append(order, item.RegencyID)
	}
	if err := recipientRows.Err(); err != nil {
		recipientRows.Close()
		return MapData{}, fmt.Errorf("iterate map recipients: %w", err)
	}
	recipientRows.Close()

	// Slot progress is scoped by the same programme/schedule/zone/regency filters. Quota is summed
	// from the schedules themselves, before the join, so it is never multiplied by slot count.
	slotRows, err := r.pool.Query(ctx, `
		WITH scoped AS (
			SELECT s.id, s.regency_id, s.slot_quota
			FROM program_schedules s
			JOIN programs prog ON prog.id = s.program_id
			LEFT JOIN program_regency_assignments pra ON pra.program_id = prog.id AND pra.regency_id = s.regency_id
			LEFT JOIN program_zones z ON z.id = pra.zone_id AND z.program_id = prog.id
			WHERE s.status <> 'cancelled'
			  AND ($1 = '' OR prog.program_type = $1)
			  AND ($2 = '' OR s.id::text = $2)
			  AND ($3 = '' OR s.program_id::text = $3)
			  AND ($4 = '' OR z.id::text = $4)
			  AND ($5 = '' OR s.regency_id::text = $5)
			  AND ($6 OR s.regency_id::text = ANY($7))
		)
		SELECT scoped.regency_id::text, r.name, r.province_name, r.document_code,
			COALESCE(sum(scoped.slot_quota), 0),
			COALESCE(sum(counted.slots), 0),
			COALESCE(sum(counted.open), 0),
			COALESCE(sum(counted.linked), 0),
			COALESCE(sum(counted.completed), 0),
			COALESCE(sum(counted.cancelled), 0)
		FROM scoped
		JOIN regencies r ON r.id = scoped.regency_id
		-- Counted per schedule in a LATERAL so each schedule contributes exactly one row. Counting
		-- after the join would multiply each schedule's quota by its number of slots.
		LEFT JOIN LATERAL (
			SELECT
				count(*) AS slots,
				count(*) FILTER (WHERE ds.status = 'open') AS open,
				count(*) FILTER (WHERE ds.status = 'linked') AS linked,
				count(*) FILTER (WHERE ds.status = 'completed') AS completed,
				count(*) FILTER (WHERE ds.status = 'cancelled') AS cancelled
			FROM distribution_slots ds WHERE ds.schedule_id = scoped.id
		) counted ON true
		GROUP BY scoped.regency_id, r.name, r.province_name, r.document_code
	`, filter.ProgramType, filter.ScheduleID, filter.ProgramID, filter.ZoneID, filter.RegencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return MapData{}, fmt.Errorf("aggregate map slots: %w", err)
	}
	defer slotRows.Close()
	for slotRows.Next() {
		var regencyID, regencyName, provinceName, documentCode string
		var slots, quota, open, linked, completed, cancelled int64
		if err := slotRows.Scan(&regencyID, &regencyName, &provinceName, &documentCode, &quota, &slots, &open, &linked, &completed, &cancelled); err != nil {
			return MapData{}, fmt.Errorf("scan map slot aggregate: %w", err)
		}
		region, ok := regions[regencyID]
		if !ok {
			// A regency can hold schedules and quota before any recipient row exists; it must still
			// appear on the map so unfilled quota is visible rather than silently missing.
			region = &MapRegion{RegencyID: regencyID, RegencyName: regencyName, ProvinceName: provinceName, DocumentCode: documentCode}
			regions[regencyID] = region
			order = append(order, regencyID)
		}
		region.SlotQuota, region.SlotsOpen, region.SlotsLinked, region.SlotsCompleted, region.SlotsCancelled = quota, open, linked, completed, cancelled
	}
	if err := slotRows.Err(); err != nil {
		return MapData{}, fmt.Errorf("iterate map slots: %w", err)
	}

	result := MapData{Regions: make([]MapRegion, 0, len(order))}
	for _, id := range order {
		result.Regions = append(result.Regions, *regions[id])
	}
	sort.Slice(result.Regions, func(i, j int) bool { return result.Regions[i].RegencyName < result.Regions[j].RegencyName })
	for _, region := range result.Regions {
		result.Totals.Recipients += region.Recipients
		result.Totals.Candidate += region.Candidate
		result.Totals.Ready += region.Ready
		result.Totals.Distributed += region.Distributed
		result.Totals.NeedsReview += region.NeedsReview
		result.Totals.Replaced += region.Replaced
		result.Totals.EvidenceComplete += region.EvidenceComplete
		result.Totals.EvidencePartial += region.EvidencePartial
		result.Totals.EvidenceEmpty += region.EvidenceEmpty
		result.Totals.EvidenceNotConfigured += region.EvidenceNotConfigured
		result.Totals.SlotQuota += region.SlotQuota
		result.Totals.SlotsOpen += region.SlotsOpen
		result.Totals.SlotsLinked += region.SlotsLinked
		result.Totals.SlotsCompleted += region.SlotsCompleted
		result.Totals.SlotsCancelled += region.SlotsCancelled
	}
	return result, nil
}

func getRecipientByID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, allocationID string) (Recipient, error) {
	row := q.QueryRow(ctx, recipientSelect+" WHERE pa.id = $1", allocationID)
	item, err := scanRecipient(row)
	if err != nil {
		return Recipient{}, ErrNotFound
	}
	return item, nil
}

// uniqueViolationConstraint reports whether err is a Postgres unique-violation
// and, if so, the name of the constraint/index that was violated.
func uniqueViolationConstraint(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}

const (
	peopleNIKUniqueIndex                    = "people_nik_uq"
	personSectorIdentifierCrossPersonUnique = "person_sector_identifiers_identifier_type_normalized_value_key"
)

func recordRecipientAudit(ctx context.Context, tx pgx.Tx, actor auth.Principal, meta auth.ClientMeta, action, resourceID string, metadata map[string]any) error {
	return audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "recipient." + action, ResourceType: "package_allocation", ResourceID: resourceID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent})
}

func (r *Repository) Create(ctx context.Context, actor auth.Principal, input CreateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Recipient{}, fmt.Errorf("begin create recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var programType string
	var packageValuesJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT prog.program_type, pt.values_json FROM program_schedules ps
		JOIN programs prog ON prog.id = ps.program_id
		JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF ps
	`, input.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&programType, &packageValuesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipient{}, ErrScheduleNotFound
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("lock schedule for create: %w", err)
	}
	allocationSnapshot, err := programs.BuildAllocationSnapshot(packageValuesJSON, input.MachineOptionCode)
	if err != nil {
		return Recipient{}, fmt.Errorf("build allocation snapshot: %w", err)
	}

	var personID string
	err = tx.QueryRow(ctx, `INSERT INTO people(full_name,nik,address,village,district,phone_number) VALUES($1,NULLIF($2,''),$3,$4,$5,$6) RETURNING id::text`,
		input.FullName, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber).Scan(&personID)
	if constraint, ok := uniqueViolationConstraint(err); ok {
		if constraint == peopleNIKUniqueIndex {
			return Recipient{}, ErrNIKInUse
		}
		return Recipient{}, ErrNIKInvalid
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("insert recipient person: %w", err)
	}

	if strings.TrimSpace(input.SectorIdentifier) != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)`,
			personID, identifierType, strings.ToUpper(strings.TrimSpace(input.SectorIdentifier))); err != nil {
			if constraint, ok := uniqueViolationConstraint(err); ok && constraint == personSectorIdentifierCrossPersonUnique {
				return Recipient{}, ErrSectorIdentifierInUse
			}
			return Recipient{}, fmt.Errorf("insert sector identifier: %w", err)
		}
	}

	var nominationID string
	if err := tx.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,$2,'{}'::jsonb,'ready') RETURNING id::text`,
		personID, programType).Scan(&nominationID); err != nil {
		return Recipient{}, fmt.Errorf("insert nomination: %w", err)
	}

	var allocationID string
	if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'ready',$4) RETURNING id::text`,
		input.ScheduleID, nominationID, personID, allocationSnapshot).Scan(&allocationID); err != nil {
		return Recipient{}, fmt.Errorf("insert allocation: %w", err)
	}

	if err := recordRecipientAudit(ctx, tx, actor, meta, "created", allocationID, map[string]any{"full_name": input.FullName, "schedule_id": input.ScheduleID}); err != nil {
		return Recipient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recipient{}, fmt.Errorf("commit create recipient: %w", err)
	}
	return getRecipientByID(ctx, r.pool, allocationID)
}

func (r *Repository) lockAllocationInScope(ctx context.Context, tx pgx.Tx, allocationID string, scope auth.RegencyScope) (personID, programType, currentStatus string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id), prog.program_type, pa.status
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN programs prog ON prog.id = ps.program_id
		WHERE pa.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF pa
	`, allocationID, scope.Unrestricted, scope.RegencyIDs).Scan(&personID, &programType, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("lock allocation: %w", err)
	}
	return personID, programType, currentStatus, nil
}

func (r *Repository) Update(ctx context.Context, actor auth.Principal, allocationID string, input UpdateInput, meta auth.ClientMeta, scope auth.RegencyScope) (Recipient, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Recipient{}, fmt.Errorf("begin update recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	personID, programType, _, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return Recipient{}, err
	}

	_, err = tx.Exec(ctx, `UPDATE people SET full_name=$2,nik=NULLIF($3,''),address=$4,village=$5,district=$6,phone_number=$7,updated_at=now() WHERE id=$1`,
		personID, input.FullName, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber)
	if constraint, ok := uniqueViolationConstraint(err); ok {
		if constraint == peopleNIKUniqueIndex {
			return Recipient{}, ErrNIKInUse
		}
		return Recipient{}, ErrNIKInvalid
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("update recipient person: %w", err)
	}

	if strings.TrimSpace(input.SectorIdentifier) != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		normalized := strings.ToUpper(strings.TrimSpace(input.SectorIdentifier))
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)
			ON CONFLICT (person_id, identifier_type) DO UPDATE SET normalized_value = EXCLUDED.normalized_value, display_value = EXCLUDED.display_value, updated_at = now()
		`, personID, identifierType, normalized); err != nil {
			if constraint, ok := uniqueViolationConstraint(err); ok && constraint == personSectorIdentifierCrossPersonUnique {
				return Recipient{}, ErrSectorIdentifierInUse
			}
			return Recipient{}, fmt.Errorf("upsert sector identifier: %w", err)
		}
	}

	if err := recordRecipientAudit(ctx, tx, actor, meta, "updated", allocationID, map[string]any{"full_name": input.FullName}); err != nil {
		return Recipient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recipient{}, fmt.Errorf("commit update recipient: %w", err)
	}
	return getRecipientByID(ctx, r.pool, allocationID)
}

func (r *Repository) Cancel(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancel recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, _, currentStatus, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return err
	}
	if currentStatus == "cancelled" {
		return ErrAlreadyCancelled
	}
	if currentStatus == "distributed" || currentStatus == "replaced" {
		return ErrCancelNotAllowed
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET status='cancelled', updated_at=now() WHERE id=$1`, allocationID); err != nil {
		return fmt.Errorf("cancel allocation: %w", err)
	}
	if err := recordRecipientAudit(ctx, tx, actor, meta, "cancelled", allocationID, map[string]any{"previous_status": currentStatus}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Restore(ctx context.Context, actor auth.Principal, allocationID string, meta auth.ClientMeta, scope auth.RegencyScope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin restore recipient: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, _, currentStatus, err := r.lockAllocationInScope(ctx, tx, allocationID, scope)
	if err != nil {
		return err
	}
	if currentStatus != "cancelled" {
		return ErrNotCancelled
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET status='ready', updated_at=now() WHERE id=$1`, allocationID); err != nil {
		return fmt.Errorf("restore allocation: %w", err)
	}
	if err := recordRecipientAudit(ctx, tx, actor, meta, "restored", allocationID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
