package recipients

import (
	"context"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
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
