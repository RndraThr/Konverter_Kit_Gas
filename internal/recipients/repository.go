package recipients

import (
	"context"
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

const recipientSelect = `
SELECT
	pa.id::text, pa.distribution_number, pa.status,
	dr.status,
	p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
	COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''),
	prog.id::text, prog.name, prog.program_type,
	r.id::text, r.name, r.document_code,
	ps.id::text, ps.name,
	pa.created_at, pa.updated_at
FROM package_allocations pa
JOIN candidate_nominations cn ON cn.id = pa.nomination_id
JOIN program_schedules ps ON ps.id = pa.schedule_id
JOIN programs prog ON prog.id = ps.program_id
JOIN regencies r ON r.id = ps.regency_id
JOIN people p ON p.id = COALESCE(pa.actual_recipient_person_id, pa.intended_person_id, cn.person_id)
LEFT JOIN distribution_records dr ON dr.allocation_id = pa.id
LEFT JOIN person_sector_identifiers psi ON psi.person_id = p.id
	AND psi.identifier_type = CASE prog.program_type WHEN 'farmer' THEN 'farmer_card' ELSE 'kusuka' END`

const recipientWhere = `
WHERE ($1 = '%%' OR p.full_name ILIKE $1 OR p.nik ILIKE $1 OR psi.normalized_value ILIKE $1)
  AND ($2 = '' OR r.id::text = $2)
  AND ($3 = '' OR prog.id::text = $3)
  AND ($4 = '' OR prog.program_type = $4)
  AND (($5 = '' AND pa.status != 'cancelled') OR ($5 != '' AND pa.status = $5))
  AND ($6 = '' OR dr.status = $6)
  AND ($7 OR ps.regency_id::text = ANY($8))`

func scanRecipient(row pgx.Row) (Recipient, error) {
	var item Recipient
	if err := row.Scan(
		&item.AllocationID, &item.DistributionNumber, &item.AllocationStatus,
		&item.DistributionStatus,
		&item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier,
		&item.Address, &item.Village, &item.District, &item.PhoneNumber,
		&item.ProgramID, &item.ProgramName, &item.ProgramType,
		&item.RegencyID, &item.RegencyName, &item.RegencyDocumentCode,
		&item.ScheduleID, &item.ScheduleName,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Recipient{}, fmt.Errorf("scan recipient: %w", err)
	}
	return item, nil
}

func (r *Repository) List(ctx context.Context, filter Filter, scope auth.RegencyScope) (Page, error) {
	search := "%" + filter.Search + "%"
	args := []any{search, filter.RegencyID, filter.ProgramID, filter.ProgramType, filter.AllocationStatus, filter.DistributionStatus, scope.Unrestricted, scope.RegencyIDs}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM package_allocations pa JOIN candidate_nominations cn ON cn.id=pa.nomination_id JOIN program_schedules ps ON ps.id=pa.schedule_id JOIN programs prog ON prog.id=ps.program_id JOIN regencies r ON r.id=ps.regency_id JOIN people p ON p.id=COALESCE(pa.actual_recipient_person_id,pa.intended_person_id,cn.person_id) LEFT JOIN distribution_records dr ON dr.allocation_id=pa.id LEFT JOIN person_sector_identifiers psi ON psi.person_id=p.id AND psi.identifier_type = CASE prog.program_type WHEN 'farmer' THEN 'farmer_card' ELSE 'kusuka' END "+recipientWhere, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count recipients: %w", err)
	}

	rows, err := r.pool.Query(ctx, recipientSelect+recipientWhere+" ORDER BY pa.created_at DESC, pa.id DESC LIMIT $9 OFFSET $10",
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

func (r *Repository) Stats(ctx context.Context, scope auth.RegencyScope) (Stats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.status, count(*)
		FROM package_allocations pa JOIN program_schedules ps ON ps.id = pa.schedule_id
		WHERE ($1 OR ps.regency_id::text = ANY($2))
		GROUP BY pa.status`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return Stats{}, fmt.Errorf("stats recipients: %w", err)
	}
	defer rows.Close()
	stats := Stats{ByAllocationStatus: map[string]int64{}}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return Stats{}, fmt.Errorf("scan recipient stats: %w", err)
		}
		stats.ByAllocationStatus[status] = count
		if status != "cancelled" {
			stats.Total += count
		}
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

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
	if isUniqueViolation(err) {
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
	if isUniqueViolation(err) {
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
