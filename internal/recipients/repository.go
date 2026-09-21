package recipients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"

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
	r.id::text, r.name, r.document_code,
	ps.id::text, ps.name,
	COALESCE(evidence.evidence_slots, '[]'::jsonb),
	pa.created_at, pa.updated_at` + recipientFrom

const recipientFrom = `
FROM package_allocations pa
JOIN candidate_nominations cn ON cn.id = pa.nomination_id
JOIN program_schedules ps ON ps.id = pa.schedule_id
JOIN programs prog ON prog.id = ps.program_id
JOIN regencies r ON r.id = ps.regency_id
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
) evidence ON true`

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
  AND ($10 OR ps.regency_id::text = ANY($11))`

func recipientOrder(filter Filter) string {
	columns := map[string]string{
		"created_at":          "pa.created_at",
		"distribution_number": "pa.distribution_number",
		"full_name":           "p.full_name",
		"nik":                 "p.nik",
		"district":            "p.district",
		"regency":             "r.name",
		"program":             "prog.name",
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
	}
}

func scanRecipient(row pgx.Row) (Recipient, error) {
	var item Recipient
	var evidenceJSON []byte
	if err := row.Scan(
		&item.AllocationID, &item.DistributionNumber, &item.AllocationStatus,
		&item.DistributionStatus,
		&item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier,
		&item.Address, &item.Village, &item.District, &item.PhoneNumber,
		&item.ProgramID, &item.ProgramName, &item.ProgramType,
		&item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode,
		&item.ScheduleID, &item.ScheduleName,
		&evidenceJSON,
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
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	args := recipientFilterArgs(filter, scope)

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) "+recipientFrom+recipientWhere, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count recipients: %w", err)
	}

	rows, err := r.pool.Query(ctx, recipientSelect+recipientWhere+recipientOrder(filter)+" LIMIT $12 OFFSET $13",
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
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
	return Page{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
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
	err = tx.QueryRow(ctx, `
		SELECT prog.program_type FROM program_schedules ps JOIN programs prog ON prog.id = ps.program_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		FOR UPDATE OF ps
	`, input.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&programType)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipient{}, ErrScheduleNotFound
	}
	if err != nil {
		return Recipient{}, fmt.Errorf("lock schedule for create: %w", err)
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

	var distributionNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(distribution_number),0)+1 FROM package_allocations WHERE schedule_id = $1`, input.ScheduleID).Scan(&distributionNumber); err != nil {
		return Recipient{}, fmt.Errorf("compute distribution number: %w", err)
	}

	var allocationID string
	if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,$3,$4,'ready','{}'::jsonb) RETURNING id::text`,
		input.ScheduleID, nominationID, personID, distributionNumber).Scan(&allocationID); err != nil {
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
