package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Search(ctx context.Context, scheduleID, query string, limit int) ([]SearchRecord, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	identifier := normalizeIdentifier(query)
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text,a.distribution_number,
			COALESCE(p.full_name,ir.normalized_data_json->>'full_name','Data perlu ditinjau'),COALESCE(p.nik,''),
			concat_ws(', ',NULLIF(p.village,''),NULLIF(p.district,''),rg.name),pr.program_type,a.status,dr.id::text,
			CASE
				WHEN EXISTS(SELECT 1 FROM distribution_records old WHERE old.recipient_person_id=p.id AND old.status='completed' AND old.allocation_id<>a.id) THEN 'previously_received'
				WHEN p.id IS NULL OR a.status='needs_review' THEN 'incomplete'
				ELSE 'eligible'
			END eligibility
		FROM package_allocations a
		JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN dcp3_import_rows ir ON ir.id=n.import_row_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		JOIN programs pr ON pr.id=ps.program_id
		JOIN regencies rg ON rg.id=ps.regency_id
		LEFT JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,a.intended_person_id,n.person_id)
		LEFT JOIN distribution_records dr ON dr.allocation_id=a.id
		WHERE a.schedule_id=$1 AND (
			a.distribution_number::text=$2 OR p.nik=NULLIF($3,'') OR
			EXISTS(SELECT 1 FROM person_sector_identifiers psi WHERE psi.person_id=p.id AND psi.normalized_value=NULLIF($4,'')) OR
			lower(p.full_name) LIKE lower($2)||'%' OR lower(p.full_name) LIKE '%'||lower($2)||'%'
		)
		ORDER BY CASE
			WHEN a.distribution_number::text=$2 THEN 0
			WHEN p.nik=NULLIF($3,'') OR EXISTS(SELECT 1 FROM person_sector_identifiers psi WHERE psi.person_id=p.id AND psi.normalized_value=NULLIF($4,'')) THEN 1
			WHEN lower(p.full_name) LIKE lower($2)||'%' THEN 2 ELSE 3 END,
			lower(COALESCE(p.full_name,'')),a.distribution_number
		LIMIT $5
	`, scheduleID, query, digits, identifier, limit)
	if err != nil {
		return nil, fmt.Errorf("search distribution recipients: %w", err)
	}
	defer rows.Close()
	results := []SearchRecord{}
	for rows.Next() {
		var item SearchRecord
		var distributionID string
		if err := rows.Scan(&item.AllocationID, &item.DistributionNumber, &item.FullName, &item.NIK, &item.Location, &item.ProgramType, &item.AllocationStatus, &distributionID, &item.Eligibility); err != nil {
			return nil, fmt.Errorf("scan distribution recipient: %w", err)
		}
		item.Documentation, err = r.listSlots(ctx, distributionID)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *Repository) GetWorkspace(ctx context.Context, allocationID string) (RecipientWorkspace, error) {
	var result RecipientWorkspace
	var sourceJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT a.id::text,dr.id::text,a.schedule_id::text,a.distribution_number,a.status,dr.status,
			pr.program_type,pr.name,rg.name,COALESCE(p.full_name,''),COALESCE(p.nik,''),
			COALESCE(psi.display_value,''),COALESCE(psi.identifier_type,''),COALESCE(p.address,''),
			COALESCE(p.village,''),COALESCE(p.district,''),COALESCE(p.phone_number,''),n.source_snapshot_json,
			CASE
				WHEN EXISTS(SELECT 1 FROM distribution_records old WHERE old.recipient_person_id=p.id AND old.status='completed' AND old.allocation_id<>a.id) THEN 'previously_received'
				WHEN p.id IS NULL OR a.status='needs_review' THEN 'incomplete'
				ELSE 'eligible'
			END
		FROM package_allocations a
		JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN program_schedules ps ON ps.id=a.schedule_id
		JOIN programs pr ON pr.id=ps.program_id
		JOIN regencies rg ON rg.id=ps.regency_id
		JOIN distribution_records dr ON dr.allocation_id=a.id
		LEFT JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,dr.recipient_person_id,a.intended_person_id,n.person_id)
		LEFT JOIN LATERAL (
			SELECT identifier_type,display_value FROM person_sector_identifiers
			WHERE person_id=p.id AND identifier_type=CASE WHEN pr.program_type='farmer' THEN 'farmer_card' ELSE 'kusuka' END LIMIT 1
		) psi ON true
		WHERE a.id=$1
	`, allocationID).Scan(
		&result.AllocationID, &result.DistributionID, &result.ScheduleID, &result.DistributionNumber,
		&result.AllocationStatus, &result.DistributionStatus, &result.ProgramType, &result.ProgramName,
		&result.RegencyName, &result.FullName, &result.NIK, &result.SectorIdentifier,
		&result.SectorIdentifierType, &result.Address, &result.Village, &result.District,
		&result.PhoneNumber, &sourceJSON, &result.Eligibility,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("get recipient workspace: %w", err)
	}
	result.SourceSnapshot = map[string]any{}
	if err := json.Unmarshal(sourceJSON, &result.SourceSnapshot); err != nil {
		return RecipientWorkspace{}, fmt.Errorf("decode recipient source snapshot: %w", err)
	}
	result.Documentation, err = r.listSlots(ctx, result.DistributionID)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	result.ReceiptHistory, err = r.listReceiptHistory(ctx, allocationID)
	if err != nil {
		return RecipientWorkspace{}, err
	}
	result.EligibilityReasons = []string{}
	if result.Eligibility == "previously_received" {
		result.EligibilityReasons = append(result.EligibilityReasons, "Penerima tercatat sudah menerima paket pada program sebelumnya")
	} else if result.Eligibility == "incomplete" {
		result.EligibilityReasons = append(result.EligibilityReasons, "Data identitas penerima perlu dilengkapi atau ditinjau")
	}
	return result, nil
}

func (r *Repository) SaveDraft(ctx context.Context, actor auth.Principal, allocationID string, input DraftInput, meta auth.ClientMeta) (RecipientWorkspace, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("begin recipient draft: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var personID, programType, oldNIK, oldAddress, oldVillage, oldDistrict, oldPhone string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text,pr.program_type,COALESCE(p.nik,''),COALESCE(p.address,''),COALESCE(p.village,''),COALESCE(p.district,''),COALESCE(p.phone_number,'')
		FROM package_allocations a JOIN candidate_nominations n ON n.id=a.nomination_id
		JOIN program_schedules ps ON ps.id=a.schedule_id JOIN programs pr ON pr.id=ps.program_id
		JOIN distribution_records dr ON dr.allocation_id=a.id
		JOIN people p ON p.id=COALESCE(a.actual_recipient_person_id,dr.recipient_person_id,a.intended_person_id,n.person_id)
		WHERE a.id=$1 FOR UPDATE OF p
	`, allocationID).Scan(&personID, &programType, &oldNIK, &oldAddress, &oldVillage, &oldDistrict, &oldPhone)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecipientWorkspace{}, ErrAllocationNotFound
	}
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("lock recipient draft: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE people SET nik=NULLIF($2,''),address=NULLIF($3,''),village=NULLIF($4,''),district=NULLIF($5,''),phone_number=NULLIF($6,''),updated_at=now() WHERE id=$1`, personID, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber)
	if isUniqueViolation(err) {
		return RecipientWorkspace{}, ErrIdentifierConflict
	}
	if err != nil {
		return RecipientWorkspace{}, fmt.Errorf("update recipient draft: %w", err)
	}
	identifierType := "farmer_card"
	if programType == "fisherman" {
		identifierType = "kusuka"
	}
	if input.SectorIdentifier != "" {
		_, err = tx.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3) ON CONFLICT(person_id,identifier_type) DO UPDATE SET normalized_value=EXCLUDED.normalized_value,display_value=EXCLUDED.display_value,updated_at=now()`, personID, identifierType, input.SectorIdentifier)
		if isUniqueViolation(err) {
			return RecipientWorkspace{}, ErrIdentifierConflict
		}
		if err != nil {
			return RecipientWorkspace{}, fmt.Errorf("update recipient sector identifier: %w", err)
		}
	}
	metadata := map[string]any{
		"before": map[string]any{"nik": maskNIK(oldNIK), "address": oldAddress, "village": oldVillage, "district": oldDistrict, "phone_number": oldPhone},
		"after":  map[string]any{"nik": maskNIK(input.NIK), "address": input.Address, "village": input.Village, "district": input.District, "phone_number": input.PhoneNumber, "sector_identifier": input.SectorIdentifier},
	}
	if input.NIK != oldNIK {
		metadata["identity_change_reason"] = input.IdentityChangeReason
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.draft_updated", ResourceType: "package_allocation", ResourceID: allocationID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return RecipientWorkspace{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecipientWorkspace{}, fmt.Errorf("commit recipient draft: %w", err)
	}
	return r.GetWorkspace(ctx, allocationID)
}

func (r *Repository) listSlots(ctx context.Context, distributionID string) ([]SlotSummary, error) {
	if distributionID == "" {
		return []SlotSummary{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text,slot_code,label_snapshot,status,is_required,min_files,max_files FROM documentation_slots WHERE distribution_id=$1 ORDER BY sort_order,slot_code`, distributionID)
	if err != nil {
		return nil, fmt.Errorf("list documentation slots: %w", err)
	}
	defer rows.Close()
	result := []SlotSummary{}
	for rows.Next() {
		var item SlotSummary
		if err := rows.Scan(&item.ID, &item.Code, &item.Label, &item.Status, &item.Required, &item.MinFiles, &item.MaxFiles); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) listReceiptHistory(ctx context.Context, currentAllocationID string) ([]ReceiptHistory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(d.completed_at,d.distributed_at),rg.name,pr.name
		FROM package_allocations current
		JOIN candidate_nominations cn ON cn.id=current.nomination_id
		JOIN distribution_records d ON d.recipient_person_id=COALESCE(current.actual_recipient_person_id,current.intended_person_id,cn.person_id) AND d.status='completed' AND d.allocation_id<>current.id
		JOIN package_allocations old ON old.id=d.allocation_id JOIN program_schedules ps ON ps.id=old.schedule_id
		JOIN programs pr ON pr.id=ps.program_id JOIN regencies rg ON rg.id=ps.regency_id
		WHERE current.id=$1 ORDER BY COALESCE(d.completed_at,d.distributed_at) DESC
	`, currentAllocationID)
	if err != nil {
		return nil, fmt.Errorf("list receipt history: %w", err)
	}
	defer rows.Close()
	result := []ReceiptHistory{}
	for rows.Next() {
		var item ReceiptHistory
		if err := rows.Scan(&item.CompletedAt, &item.Regency, &item.Program); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
