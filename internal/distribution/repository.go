package distribution

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

func (r *Repository) GetMediaSlot(ctx context.Context, slotID string, scope auth.RegencyScope) (MediaSlot, error) {
	var result MediaSlot
	err := r.pool.QueryRow(ctx, `
		SELECT s.id::text,s.input_source,s.require_location,s.require_captured_at,s.min_files,s.max_files,count(m.id) FILTER(WHERE m.status='accepted')
		FROM documentation_slots s
		LEFT JOIN media_files m ON m.documentation_slot_id=s.id
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		WHERE s.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY s.id
	`, slotID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.InputSource, &result.RequireLocation, &result.RequireCapturedAt, &result.MinFiles, &result.MaxFiles, &result.AcceptedFiles)
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

func (r *Repository) GetMedia(ctx context.Context, mediaID string, scope auth.RegencyScope) (MediaFile, error) {
	var result MediaFile
	err := r.pool.QueryRow(ctx, `
		SELECT m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.uploaded_at
		FROM media_files m
		JOIN documentation_slots s ON s.id=m.documentation_slot_id
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text = ANY($3))
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("get documentation media: %w", err)
	}
	result.ContentURL = "/api/v1/distribution/media/" + result.ID + "/content"
	return result, nil
}

func (r *Repository) DeleteMedia(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result MediaFile
	err = tx.QueryRow(ctx, `
		UPDATE media_files m
		SET status='deleted', updated_at=now()
		FROM documentation_slots s
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		WHERE m.documentation_slot_id=s.id
			AND m.id=$1 AND m.status='accepted'
			AND ($2 OR ps.regency_id::text = ANY($3))
		RETURNING m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.uploaded_at
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.UploadedAt)
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func uniqueViolationConstraint(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}

// packageAllocationsScheduleDistributionNumberUniqueIndex is the auto-generated name of
// package_allocations' UNIQUE (schedule_id, distribution_number) constraint. It has nothing to do
// with a recipient's NIK or sector identifier, so a violation of it must not be reported as
// ErrIdentifierConflict.
const packageAllocationsScheduleDistributionNumberUniqueIndex = "package_allocations_schedule_id_distribution_number_key"

func (r *Repository) CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, meta auth.ClientMeta) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin create slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var documentationTemplateID string
	if err := tx.QueryRow(ctx, `SELECT documentation_template_version_id FROM program_schedules WHERE id=$1 FOR UPDATE`, input.ScheduleID).Scan(&documentationTemplateID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrScheduleRequired
		}
		return DistributionSlot{}, fmt.Errorf("lock schedule: %w", err)
	}

	var nextNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(slot_number),0)+1 FROM distribution_slots WHERE schedule_id=$1`, input.ScheduleID).Scan(&nextNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("allocate slot number: %w", err)
	}

	var slotID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO distribution_slots (schedule_id, slot_number, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''))
		RETURNING id::text
	`, input.ScheduleID, nextNumber, input.MachineOptionCode, input.MachineSerialNumber, input.HoseOptionCode, input.HoseSerialNumber, input.ConverterSerialNumber).Scan(&slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("insert distribution slot: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
		SELECT $1,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order
		FROM documentation_template_slots WHERE template_version_id=$2
	`, slotID, documentationTemplateID); err != nil {
		return DistributionSlot{}, fmt.Errorf("snapshot documentation slots: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_created", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"schedule_id": input.ScheduleID, "slot_number": nextNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit create slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) getSlotByID(ctx context.Context, id string) (DistributionSlot, error) {
	var slot DistributionSlot
	var machineOption, machineSerial, hoseOption, hoseSerial, converterSerial *string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, schedule_id::text, slot_number, status, allocation_id::text, machine_option_code, machine_serial_number, hose_option_code, hose_serial_number, converter_serial_number, distributed_at, created_at, updated_at
		FROM distribution_slots WHERE id=$1
	`, id).Scan(&slot.ID, &slot.ScheduleID, &slot.SlotNumber, &slot.Status, &slot.AllocationID, &machineOption, &machineSerial, &hoseOption, &hoseSerial, &converterSerial, &slot.DistributedAt, &slot.CreatedAt, &slot.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("get distribution slot: %w", err)
	}
	if machineOption != nil {
		slot.MachineOptionCode = *machineOption
	}
	if machineSerial != nil {
		slot.MachineSerialNumber = *machineSerial
	}
	if hoseOption != nil {
		slot.HoseOptionCode = *hoseOption
	}
	if hoseSerial != nil {
		slot.HoseSerialNumber = *hoseSerial
	}
	if converterSerial != nil {
		slot.ConverterSerialNumber = *converterSerial
	}
	slot.Documentation, err = r.listSlotDocumentation(ctx, id)
	if err != nil {
		return DistributionSlot{}, err
	}
	return slot, nil
}

func (r *Repository) listSlotDocumentation(ctx context.Context, distributionSlotID string) ([]SlotSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_code, ds.label_snapshot, ds.stage, ds.status, ds.is_required, ds.min_files, ds.max_files, ds.input_source, ds.require_location, ds.require_captured_at
		FROM documentation_slots ds WHERE ds.distribution_slot_id=$1 ORDER BY ds.sort_order
	`, distributionSlotID)
	if err != nil {
		return nil, fmt.Errorf("list slot documentation: %w", err)
	}
	defer rows.Close()
	var summaries []SlotSummary
	for rows.Next() {
		var summary SlotSummary
		if err := rows.Scan(&summary.ID, &summary.Code, &summary.Label, &summary.Stage, &summary.Status, &summary.Required, &summary.MinFiles, &summary.MaxFiles, &summary.InputSource, &summary.RequireLocation, &summary.RequireCapturedAt); err != nil {
			return nil, fmt.Errorf("scan slot documentation: %w", err)
		}
		files, err := r.listMediaFiles(ctx, summary.ID)
		if err != nil {
			return nil, err
		}
		summary.Files = files
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// listMediaFiles is a minimal stand-in for the shared media-listing helper Task 8 introduces;
// it exists so this file compiles in isolation while POS Mesin lands ahead of the other POS stations.
func (r *Repository) listMediaFiles(ctx context.Context, documentationSlotID string) ([]MediaFile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, documentation_slot_id::text, storage_key::text, original_filename, mime_type, byte_size, source, captured_at, latitude, longitude, status, uploaded_at
		FROM media_files WHERE documentation_slot_id=$1 AND status='accepted' ORDER BY uploaded_at
	`, documentationSlotID)
	if err != nil {
		return nil, fmt.Errorf("list media files: %w", err)
	}
	defer rows.Close()
	var files []MediaFile
	for rows.Next() {
		var file MediaFile
		if err := rows.Scan(&file.ID, &file.SlotID, &file.StorageKey, &file.OriginalFilename, &file.MimeType, &file.ByteSize, &file.Source, &file.CapturedAt, &file.Latitude, &file.Longitude, &file.Status, &file.UploadedAt); err != nil {
			return nil, fmt.Errorf("scan media file: %w", err)
		}
		file.ContentURL = "/api/v1/distribution/media/" + file.ID + "/content"
		files = append(files, file)
	}
	return files, rows.Err()
}

func (r *Repository) SearchCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateMatch, error) {
	var match CandidateMatch
	err := r.pool.QueryRow(ctx, `
		SELECT pa.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
			COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''), cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		LEFT JOIN LATERAL (SELECT identifier_type, normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE pa.schedule_id = $1 AND p.nik = $2 AND pa.distribution_number IS NULL
			AND ($3 OR ps.regency_id::text = ANY($4))
	`, scheduleID, nik, scope.Unrestricted, scope.RegencyIDs).Scan(&match.AllocationID, &match.FullName, &match.NIK, &match.SectorIdentifierType, &match.SectorIdentifier, &match.Address, &match.Village, &match.District, &match.PhoneNumber, &match.ProgramType)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateMatch{}, ErrCandidateNotFound
	}
	if err != nil {
		return CandidateMatch{}, fmt.Errorf("search candidate: %w", err)
	}
	return match, nil
}

func (r *Repository) LinkSlot(ctx context.Context, actor auth.Principal, input LinkSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin link slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, slotStatus string
	if err := tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &slotStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrSlotNotFound
		}
		return DistributionSlot{}, fmt.Errorf("lock distribution slot: %w", err)
	}
	if slotStatus != "open" {
		return DistributionSlot{}, ErrSlotNotOpen
	}

	var allocationID, personID, programType string
	if err := tx.QueryRow(ctx, `
		SELECT pa.id::text, p.id::text, cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN people p ON p.id = cn.person_id
		WHERE pa.schedule_id=$1 AND p.nik=$2 AND pa.distribution_number IS NULL
		FOR UPDATE OF pa
	`, input.ScheduleID, input.NIK).Scan(&allocationID, &personID, &programType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrCandidateNotFound
		}
		return DistributionSlot{}, fmt.Errorf("lock candidate: %w", err)
	}

	var previouslyReceived bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed')`, personID).Scan(&previouslyReceived); err != nil {
		return DistributionSlot{}, fmt.Errorf("check previously received: %w", err)
	}
	if previouslyReceived {
		return DistributionSlot{}, ErrPreviouslyReceived
	}

	if _, err := tx.Exec(ctx, `UPDATE people SET address=COALESCE(NULLIF($2,''),address), village=COALESCE(NULLIF($3,''),village), district=COALESCE(NULLIF($4,''),district), phone_number=COALESCE(NULLIF($5,''),phone_number), updated_at=now() WHERE id=$1`, personID, input.Address, input.Village, input.District, input.PhoneNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("update person: %w", err)
	}
	if input.SectorIdentifier != "" {
		identifierType := "kusuka"
		if programType == "farmer" {
			identifierType = "farmer_card"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)
			ON CONFLICT (person_id,identifier_type) DO UPDATE SET normalized_value=EXCLUDED.normalized_value, display_value=EXCLUDED.display_value, updated_at=now()
		`, personID, identifierType, input.SectorIdentifier); err != nil {
			if code, ok := uniqueViolationConstraint(err); ok && code != "" {
				return DistributionSlot{}, ErrIdentifierConflict
			}
			return DistributionSlot{}, fmt.Errorf("upsert sector identifier: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=$2, status='ready', updated_at=now() WHERE id=$1`, allocationID, input.SlotNumber); err != nil {
		if constraint, ok := uniqueViolationConstraint(err); ok && constraint == packageAllocationsScheduleDistributionNumberUniqueIndex {
			// This constraint is (schedule_id, distribution_number), not an identifier — the slot's own
			// FOR UPDATE lock above should make this unreachable in normal operation, so treat it as an
			// unexpected infrastructure-level failure rather than a user-facing identifier conflict.
			return DistributionSlot{}, fmt.Errorf("distribution number %d already allocated for schedule %s: %w", input.SlotNumber, input.ScheduleID, err)
		}
		return DistributionSlot{}, fmt.Errorf("update package allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id=$2, recipient_person_id=$3, status='linked', updated_at=now() WHERE id=$1`, slotID, allocationID, personID); err != nil {
		return DistributionSlot{}, fmt.Errorf("link distribution slot: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_linked", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": allocationID, "slot_number": input.SlotNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit link slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) SearchLinkedSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ds.status='linked' AND ($4 OR ps.regency_id::text = ANY($5))
			AND (ds.slot_number::text = $2 OR (p.nik IS NOT NULL AND p.nik = NULLIF($3,'')))
		LIMIT 1
	`, scheduleID, query, digits, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("search linked slot: %w", err)
	}
	return r.getSlotByID(ctx, id)
}

func (r *Repository) CompleteSlot(ctx context.Context, actor auth.Principal, input CompleteSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin complete slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status string
	var allocationIDPtr, personIDPtr *string
	var fullName, nik, sectorIdentifier string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, pa.id::text, p.id::text, COALESCE(p.full_name,''), COALESCE(p.nik,''), COALESCE(psi.normalized_value,'')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN package_allocations pa ON pa.id = ds.allocation_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN LATERAL (SELECT normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &allocationIDPtr, &personIDPtr, &fullName, &nik, &sectorIdentifier)
	// pa and p are LEFT JOINed (not INNER JOINed) because a freshly-created 'open' slot has
	// NULL allocation_id/recipient_person_id — an INNER JOIN would silently drop that row before
	// the WHERE clause or FOR UPDATE lock ever apply, turning a legitimate ErrSlotNotLinked case
	// into a misleading ErrSlotNotFound. We must lock+fetch the slot row regardless of link state,
	// decide ErrSlotNotFound/ErrAlreadyCompleted/ErrSlotNotLinked from ds.status alone, and only
	// dereference allocationIDPtr/personIDPtr once status=='linked' guarantees they are non-null.
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock distribution slot: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if status != "linked" {
		return DistributionSlot{}, ErrSlotNotLinked
	}
	if allocationIDPtr == nil || personIDPtr == nil {
		return DistributionSlot{}, fmt.Errorf("linked distribution slot %s is missing its allocation or recipient link", slotID)
	}
	allocationID, personID := *allocationIDPtr, *personIDPtr
	if strings.TrimSpace(fullName) == "" || len(nik) != 16 || sectorIdentifier == "" {
		return DistributionSlot{}, ErrIdentityIncomplete
	}

	var previouslyReceived bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed' AND id<>$2)`, personID, slotID).Scan(&previouslyReceived); err != nil {
		return DistributionSlot{}, fmt.Errorf("check previously received: %w", err)
	}
	if previouslyReceived {
		return DistributionSlot{}, ErrPreviouslyReceived
	}

	var incomplete bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM documentation_slots ds
			LEFT JOIN (SELECT documentation_slot_id, count(*) AS accepted FROM media_files WHERE status='accepted' GROUP BY documentation_slot_id) m ON m.documentation_slot_id = ds.id
			WHERE ds.distribution_slot_id=$1 AND ds.is_required AND COALESCE(m.accepted,0) < ds.min_files
		)
	`, slotID).Scan(&incomplete); err != nil {
		return DistributionSlot{}, fmt.Errorf("check documentation completeness: %w", err)
	}
	if incomplete {
		return DistributionSlot{}, ErrDocumentationIncomplete
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET actual_recipient_person_id=$2, status='distributed', updated_at=now() WHERE id=$1`, allocationID, personID); err != nil {
		return DistributionSlot{}, fmt.Errorf("update package allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET status='completed', distributed_at=now(), distributed_by=$2, completed_at=now(), updated_at=now() WHERE id=$1`, slotID, actor.UserID); err != nil {
		return DistributionSlot{}, fmt.Errorf("complete distribution slot: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_completed", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": allocationID, "person_id": personID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit complete slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}
