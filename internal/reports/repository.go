package reports

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const reportsBaseCTE = `
WITH base AS (
	SELECT a.distribution_number, a.status AS allocation_status,
		COALESCE(dr.status,'draft') AS distribution_status,
		COALESCE(p.full_name,'Data perlu ditinjau') AS full_name,
		COALESCE(p.nik,'') AS nik,
		COALESCE(psi.display_value,'') AS sector_identifier,
		COALESCE(p.village,'') AS village,
		COALESCE(p.district,'') AS district,
		dr.completed_at,
		NOT EXISTS (
			SELECT 1 FROM documentation_slots ds
			WHERE ds.distribution_id = dr.id AND ds.is_required AND ds.status <> 'complete'
		) AS documentation_complete
	FROM package_allocations a
	JOIN candidate_nominations n ON n.id = a.nomination_id
	JOIN program_schedules ps ON ps.id = a.schedule_id
	JOIN programs pr ON pr.id = ps.program_id
	LEFT JOIN distribution_records dr ON dr.allocation_id = a.id
	LEFT JOIN people p ON p.id = COALESCE(a.actual_recipient_person_id, a.intended_person_id, n.person_id)
	LEFT JOIN LATERAL (
		SELECT display_value FROM person_sector_identifiers
		WHERE person_id = p.id
			AND identifier_type = CASE WHEN pr.program_type = 'farmer' THEN 'farmer_card' ELSE 'kusuka' END
		LIMIT 1
	) psi ON true
	WHERE a.schedule_id = $1
)
`

const reportsFilterClause = `
WHERE ($2 = '' OR allocation_status = $2)
	AND ($3 = '' OR distribution_status = $3)
	AND ($4 = '' OR ($4 = 'complete' AND documentation_complete) OR ($4 = 'incomplete' AND NOT documentation_complete))
`

func (r *Repository) Summary(ctx context.Context, scheduleID string, filter Filter) (Summary, error) {
	query := reportsBaseCTE + `
SELECT count(*),
	count(*) FILTER (WHERE allocation_status = 'candidate'),
	count(*) FILTER (WHERE allocation_status = 'ready'),
	count(*) FILTER (WHERE allocation_status = 'needs_review'),
	count(*) FILTER (WHERE allocation_status = 'distributed'),
	count(*) FILTER (WHERE allocation_status = 'replaced'),
	count(*) FILTER (WHERE allocation_status = 'cancelled'),
	count(*) FILTER (WHERE distribution_status = 'draft'),
	count(*) FILTER (WHERE distribution_status = 'completed'),
	count(*) FILTER (WHERE distribution_status = 'cancelled'),
	count(*) FILTER (WHERE NOT documentation_complete)
FROM base
` + reportsFilterClause

	var summary Summary
	var candidate, ready, needsReview, distributed, replaced, cancelled int
	var draft, completed, distributionCancelled int
	err := r.pool.QueryRow(ctx, query, scheduleID, filter.AllocationStatus, filter.DistributionStatus, filter.DocumentationStatus).Scan(
		&summary.TotalAllocations, &candidate, &ready, &needsReview, &distributed, &replaced, &cancelled,
		&draft, &completed, &distributionCancelled, &summary.DocumentationIncomplete,
	)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize report: %w", err)
	}
	summary.AllocationStatusCounts = []StatusCount{
		{Status: "candidate", Count: candidate}, {Status: "ready", Count: ready}, {Status: "needs_review", Count: needsReview},
		{Status: "distributed", Count: distributed}, {Status: "replaced", Count: replaced}, {Status: "cancelled", Count: cancelled},
	}
	summary.DistributionStatusCounts = []StatusCount{
		{Status: "draft", Count: draft}, {Status: "completed", Count: completed}, {Status: "cancelled", Count: distributionCancelled},
	}
	return summary, nil
}

func (r *Repository) Rows(ctx context.Context, scheduleID string, filter Filter) ([]Row, error) {
	query := reportsBaseCTE + `
SELECT distribution_number, allocation_status, distribution_status, full_name, nik, sector_identifier, village, district, completed_at, documentation_complete
FROM base
` + reportsFilterClause + `
ORDER BY distribution_number
`
	rows, err := r.pool.Query(ctx, query, scheduleID, filter.AllocationStatus, filter.DistributionStatus, filter.DocumentationStatus)
	if err != nil {
		return nil, fmt.Errorf("list report rows: %w", err)
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var item Row
		if err := rows.Scan(&item.DistributionNumber, &item.AllocationStatus, &item.DistributionStatus, &item.FullName, &item.NIK, &item.SectorIdentifier, &item.Village, &item.District, &item.CompletedAt, &item.DocumentationComplete); err != nil {
			return nil, fmt.Errorf("scan report row: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
