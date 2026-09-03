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
	rows, err := r.pool.Query(ctx, `SELECT id::text,slot_code,label_snapshot,status,is_required,min_files,max_files,input_source,require_location,require_captured_at FROM documentation_slots WHERE distribution_id=$1 ORDER BY sort_order,slot_code`, distributionID)
	if err != nil {
		return nil, fmt.Errorf("list documentation slots: %w", err)
	}
	defer rows.Close()
	result := []SlotSummary{}
	for rows.Next() {
		var item SlotSummary
		if err := rows.Scan(&item.ID, &item.Code, &item.Label, &item.Status, &item.Required, &item.MinFiles, &item.MaxFiles, &item.InputSource, &item.RequireLocation, &item.RequireCapturedAt); err != nil {
			return nil, err
		}
		item.Files, err = r.listMedia(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetMediaSlot(ctx context.Context, slotID string) (MediaSlot, error) {
	var result MediaSlot
	err := r.pool.QueryRow(ctx, `SELECT s.id::text,s.input_source,s.require_location,s.require_captured_at,s.min_files,s.max_files,count(m.id) FILTER(WHERE m.status='accepted') FROM documentation_slots s LEFT JOIN media_files m ON m.documentation_slot_id=s.id WHERE s.id=$1 GROUP BY s.id`, slotID).Scan(&result.ID, &result.InputSource, &result.RequireLocation, &result.RequireCapturedAt, &result.MinFiles, &result.MaxFiles, &result.AcceptedFiles)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaSlot{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaSlot{}, fmt.Errorf("get documentation slot: %w", err)
	}
	return result, nil
}

func (r *Repository) SaveMedia(ctx context.Context, actor auth.Principal, input MediaFileInput, meta auth.ClientMeta) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media metadata: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result MediaFile
	err = tx.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,captured_at,latitude,longitude,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid) RETURNING id::text,documentation_slot_id::text,storage_key::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,uploaded_at`, input.SlotID, input.StorageKey, input.OriginalFilename, input.MimeType, input.ByteSize, input.Checksum, input.Source, input.CapturedAt, input.Latitude, input.Longitude, actor.UserID).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if err != nil {
		return MediaFile{}, fmt.Errorf("save media metadata: %w", err)
	}
	if err := updateSlotStatus(ctx, tx, input.SlotID); err != nil {
		return MediaFile{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation.media_uploaded", ResourceType: "media_file", ResourceID: result.ID, Metadata: map[string]any{"slot_id": input.SlotID, "mime_type": input.MimeType, "byte_size": input.ByteSize, "source": input.Source}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return MediaFile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MediaFile{}, fmt.Errorf("commit media metadata: %w", err)
	}
	return result, nil
}

func (r *Repository) GetMedia(ctx context.Context, mediaID string) (MediaFile, error) {
	var result MediaFile
	err := r.pool.QueryRow(ctx, `SELECT id::text,documentation_slot_id::text,storage_key::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,uploaded_at FROM media_files WHERE id=$1 AND status='accepted'`, mediaID).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("get documentation media: %w", err)
	}
	result.ContentURL = "/api/v1/distribution/media/" + result.ID + "/content"
	return result, nil
}

func (r *Repository) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result MediaFile
	err = tx.QueryRow(ctx, `UPDATE media_files SET status='deleted',updated_at=now() WHERE id=$1 AND status='accepted' RETURNING id::text,documentation_slot_id::text,storage_key::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,uploaded_at`, mediaID).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("mark media deleted: %w", err)
	}
	if err := updateSlotStatus(ctx, tx, result.SlotID); err != nil {
		return MediaFile{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation.media_deleted", ResourceType: "media_file", ResourceID: result.ID, Metadata: map[string]any{"slot_id": result.SlotID, "mime_type": result.MimeType, "byte_size": result.ByteSize}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return MediaFile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MediaFile{}, fmt.Errorf("commit media delete: %w", err)
	}
	return result, nil
}

func (r *Repository) RestoreMedia(ctx context.Context, mediaID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var slotID string
	if err := tx.QueryRow(ctx, `UPDATE media_files SET status='accepted',updated_at=now() WHERE id=$1 AND status='deleted' RETURNING documentation_slot_id::text`, mediaID).Scan(&slotID); err != nil {
		return err
	}
	if err := updateSlotStatus(ctx, tx, slotID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) listMedia(ctx context.Context, slotID string) ([]MediaFile, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,documentation_slot_id::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,uploaded_at FROM media_files WHERE documentation_slot_id=$1 AND status='accepted' ORDER BY uploaded_at,id`, slotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []MediaFile{}
	for rows.Next() {
		var item MediaFile
		if err := rows.Scan(&item.ID, &item.SlotID, &item.OriginalFilename, &item.MimeType, &item.ByteSize, &item.Source, &item.CapturedAt, &item.Latitude, &item.Longitude, &item.Status, &item.UploadedAt); err != nil {
			return nil, err
		}
		item.ContentURL = "/api/v1/distribution/media/" + item.ID + "/content"
		result = append(result, item)
	}
	return result, rows.Err()
}

func updateSlotStatus(ctx context.Context, tx pgx.Tx, slotID string) error {
	_, err := tx.Exec(ctx, `UPDATE documentation_slots s SET status=CASE WHEN (SELECT count(*) FROM media_files m WHERE m.documentation_slot_id=s.id AND m.status='accepted')>=s.min_files THEN 'complete' ELSE 'missing' END,updated_at=now() WHERE s.id=$1`, slotID)
	if err != nil {
		return fmt.Errorf("update documentation slot status: %w", err)
	}
	return nil
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
