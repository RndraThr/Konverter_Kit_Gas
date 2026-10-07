package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"
	"konkit/internal/programs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) GetMediaSlot(ctx context.Context, slotID string, scope auth.RegencyScope) (MediaSlot, error) {
	var result MediaSlot
	var zoneName *string
	var isPlaceholder *bool
	err := r.pool.QueryRow(ctx, `
		SELECT s.id::text,dsl.schedule_id::text,dsl.slot_number,dsl.distribution_date::text,(dsl.recipient_person_id IS NOT NULL),dsl.status,s.label_snapshot,s.input_source,s.media_kind,s.require_location,s.require_captured_at,s.min_files,s.max_files,
			count(m.id) FILTER(WHERE m.status='accepted'), p.program_type, z.name, r.name, z.is_placeholder
		FROM documentation_slots s
		LEFT JOIN media_files m ON m.documentation_slot_id=s.id
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		JOIN programs p ON p.id=ps.program_id
		JOIN regencies r ON r.id=ps.regency_id
		LEFT JOIN program_regency_assignments pra ON pra.program_id=ps.program_id AND pra.regency_id=ps.regency_id
		LEFT JOIN program_zones z ON z.id=pra.zone_id
		WHERE s.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY s.id,dsl.schedule_id,dsl.slot_number,dsl.distribution_date,dsl.recipient_person_id,dsl.status,s.label_snapshot,p.program_type,z.name,r.name,z.is_placeholder
	`, slotID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.ScheduleID, &result.SlotNumber, &result.DistributionDate, &result.HasRecipient, &result.DistributionStatus, &result.Label, &result.InputSource, &result.MediaKind, &result.RequireLocation, &result.RequireCapturedAt, &result.MinFiles, &result.MaxFiles, &result.AcceptedFiles, &result.ProgramType, &zoneName, &result.RegencyName, &isPlaceholder)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaSlot{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaSlot{}, fmt.Errorf("get documentation slot: %w", err)
	}
	if zoneName == nil || isPlaceholder == nil || *isPlaceholder {
		return MediaSlot{}, programs.ErrZoneNotConfigured
	}
	result.ZoneName = *zoneName
	return result, nil
}

func (r *Repository) DocumentationSlotStage(ctx context.Context, documentationSlotID string, scope auth.RegencyScope) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `
		SELECT s.stage FROM documentation_slots s
		JOIN distribution_slots ds ON ds.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE s.id=$1 AND ($2 OR ps.regency_id::text=ANY($3))
	`, documentationSlotID, scope.Unrestricted, scope.RegencyIDs).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMediaNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get documentation stage: %w", err)
	}
	return stage, nil
}

func (r *Repository) MediaStage(ctx context.Context, mediaID string, scope auth.RegencyScope) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `
		SELECT s.stage FROM media_files m
		JOIN documentation_slots s ON s.id=m.documentation_slot_id
		JOIN distribution_slots ds ON ds.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text=ANY($3))
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMediaNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get media stage: %w", err)
	}
	return stage, nil
}

func (r *Repository) SaveMedia(ctx context.Context, actor auth.Principal, input MediaFileInput, meta auth.ClientMeta) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media metadata: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result MediaFile
	err = tx.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,captured_at,latitude,longitude,storage_state,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,COALESCE(NULLIF($11,''),'final'),NULLIF($12,'')::uuid) RETURNING id::text,documentation_slot_id::text,storage_key::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,storage_state,storage_last_error,uploaded_at`, input.SlotID, input.StorageKey, input.OriginalFilename, input.MimeType, input.ByteSize, input.Checksum, input.Source, input.CapturedAt, input.Latitude, input.Longitude, input.StorageState, actor.UserID).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.StorageState, &result.StorageLastError, &result.UploadedAt)
	if err != nil {
		return MediaFile{}, fmt.Errorf("save media metadata: %w", err)
	}
	if err := updateSlotStatus(ctx, tx, input.SlotID); err != nil {
		return MediaFile{}, err
	}
	if input.StorageState == "staging" {
		var distributionSlotID string
		if err := tx.QueryRow(ctx, `SELECT distribution_slot_id::text FROM documentation_slots WHERE id=$1`, input.SlotID).Scan(&distributionSlotID); err != nil {
			return MediaFile{}, fmt.Errorf("resolve staged media distribution slot: %w", err)
		}
		if err := r.queueMediaMove(ctx, tx, distributionSlotID, result.ID); err != nil {
			return MediaFile{}, err
		}
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
		SELECT m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.storage_state,m.storage_last_error,m.uploaded_at
		FROM media_files m
		JOIN documentation_slots s ON s.id=m.documentation_slot_id
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text = ANY($3))
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.StorageState, &result.StorageLastError, &result.UploadedAt)
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
	var distributionStatus string
	err = tx.QueryRow(ctx, `
		SELECT dsl.status FROM media_files m
		JOIN documentation_slots s ON s.id=m.documentation_slot_id
		JOIN distribution_slots dsl ON dsl.id=s.distribution_slot_id
		JOIN program_schedules ps ON ps.id=dsl.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text=ANY($3))
		FOR UPDATE OF m,dsl
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&distributionStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("lock media delete: %w", err)
	}
	if distributionStatus == "completed" {
		return MediaFile{}, ErrAlreadyCompleted
	}
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
		RETURNING m.id::text,m.documentation_slot_id::text,m.storage_key::text,m.original_filename,m.mime_type,m.byte_size,m.source,m.captured_at,m.latitude::float8,m.longitude::float8,m.status,m.storage_state,m.storage_last_error,m.uploaded_at
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.SlotID, &result.StorageKey, &result.OriginalFilename, &result.MimeType, &result.ByteSize, &result.Source, &result.CapturedAt, &result.Latitude, &result.Longitude, &result.Status, &result.StorageState, &result.StorageLastError, &result.UploadedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("mark media deleted: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM distribution_media_move_jobs WHERE media_file_id=$1`, mediaID); err != nil {
		return MediaFile{}, fmt.Errorf("delete media move job: %w", err)
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
	rows, err := r.pool.Query(ctx, `SELECT id::text,documentation_slot_id::text,original_filename,mime_type,byte_size,source,captured_at,latitude::float8,longitude::float8,status,storage_state,storage_last_error,uploaded_at FROM media_files WHERE documentation_slot_id=$1 AND status='accepted' ORDER BY uploaded_at,id`, slotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []MediaFile{}
	for rows.Next() {
		var item MediaFile
		if err := rows.Scan(&item.ID, &item.SlotID, &item.OriginalFilename, &item.MimeType, &item.ByteSize, &item.Source, &item.CapturedAt, &item.Latitude, &item.Longitude, &item.Status, &item.StorageState, &item.StorageLastError, &item.UploadedAt); err != nil {
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

func (r *Repository) queueMediaMovesForSlot(ctx context.Context, tx pgx.Tx, distributionSlotID string) error {
	return r.queueMediaMove(ctx, tx, distributionSlotID, "")
}

func (r *Repository) queueMediaMove(ctx context.Context, tx pgx.Tx, distributionSlotID, mediaID string) error {
	var distributionDate *time.Time
	var slotNumber int
	var hasRecipient bool
	var programType, regencyName string
	var zoneName *string
	var zonePlaceholder *bool
	if err := tx.QueryRow(ctx, `
		SELECT ds.distribution_date,ds.slot_number,(ds.recipient_person_id IS NOT NULL),p.program_type,z.name,r.name,z.is_placeholder
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		JOIN programs p ON p.id=ps.program_id
		JOIN regencies r ON r.id=ps.regency_id
		LEFT JOIN program_regency_assignments pra ON pra.program_id=ps.program_id AND pra.regency_id=ps.regency_id
		LEFT JOIN program_zones z ON z.id=pra.zone_id
		WHERE ds.id=$1
	`, distributionSlotID).Scan(&distributionDate, &slotNumber, &hasRecipient, &programType, &zoneName, &regencyName, &zonePlaceholder); err != nil {
		return fmt.Errorf("resolve media move target: %w", err)
	}
	if distributionDate == nil || !hasRecipient {
		return nil
	}
	if zoneName == nil || zonePlaceholder == nil || *zonePlaceholder {
		return programs.ErrZoneNotConfigured
	}
	basePath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: programType,
		ZoneName:    *zoneName,
		RegencyName: regencyName,
		Category:    media.FolderPhotos,
	})
	if err != nil {
		return err
	}
	targetPath, err := media.BuildDistributionFinalPath(basePath, *distributionDate, slotNumber)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		UPDATE media_files m
		SET storage_state='moving',storage_last_error='',
			storage_target_generation=storage_target_generation+1,updated_at=now()
		FROM documentation_slots ds
		WHERE m.documentation_slot_id=ds.id
		  AND ds.distribution_slot_id=$1 AND m.status='accepted'
		  AND (NULLIF($2,'') IS NULL OR m.id=NULLIF($2,'')::uuid)
		RETURNING m.id::text,m.storage_target_generation
	`, distributionSlotID, mediaID)
	if err != nil {
		return fmt.Errorf("prepare media moves: %w", err)
	}
	type generatedMove struct {
		mediaID    string
		generation int64
	}
	moves := make([]generatedMove, 0)
	for rows.Next() {
		var move generatedMove
		if err := rows.Scan(&move.mediaID, &move.generation); err != nil {
			rows.Close()
			return fmt.Errorf("scan prepared media move: %w", err)
		}
		moves = append(moves, move)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("prepare media moves: %w", err)
	}
	rows.Close()

	for _, move := range moves {
		if _, err := tx.Exec(ctx, `
			INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_generation,status,attempts,next_attempt_at,locked_at,last_error)
			VALUES($1,$2,$3,'queued',0,now(),NULL,'')
			ON CONFLICT(media_file_id) DO UPDATE SET
				target_path=EXCLUDED.target_path,target_generation=EXCLUDED.target_generation,
				status='queued',attempts=0,next_attempt_at=now(),locked_at=NULL,last_error='',updated_at=now()
		`, move.mediaID, targetPath, move.generation); err != nil {
			return fmt.Errorf("queue media move: %w", err)
		}
	}
	return nil
}

func (r *Repository) RetryMediaMove(ctx context.Context, actor auth.Principal, mediaID string, meta auth.ClientMeta, scope auth.RegencyScope) (MediaFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaFile{}, fmt.Errorf("begin media move retry: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var distributionSlotID string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text
		FROM media_files m
		JOIN documentation_slots docs ON docs.id=m.documentation_slot_id
		JOIN distribution_slots ds ON ds.id=docs.distribution_slot_id
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE m.id=$1 AND m.status='accepted' AND ($2 OR ps.regency_id::text=ANY($3))
		FOR UPDATE OF m
	`, mediaID, scope.Unrestricted, scope.RegencyIDs).Scan(&distributionSlotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaFile{}, fmt.Errorf("lock media move retry: %w", err)
	}
	if err := r.queueMediaMove(ctx, tx, distributionSlotID, mediaID); err != nil {
		return MediaFile{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "documentation.media_move_retried", ResourceType: "media_file", ResourceID: mediaID, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return MediaFile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MediaFile{}, fmt.Errorf("commit media move retry: %w", err)
	}
	return r.GetMedia(ctx, mediaID, scope)
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

func (r *Repository) CreateSlot(ctx context.Context, actor auth.Principal, input CreateSlotInput, scope auth.RegencyScope, meta auth.ClientMeta) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin create slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var documentationTemplateID string
	var slotQuota *int
	if err := tx.QueryRow(ctx, `SELECT documentation_template_version_id, slot_quota FROM program_schedules WHERE id=$1 AND ($2 OR regency_id::text = ANY($3)) FOR UPDATE`, input.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&documentationTemplateID, &slotQuota); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DistributionSlot{}, ErrScheduleRequired
		}
		return DistributionSlot{}, fmt.Errorf("lock schedule: %w", err)
	}

	// The catalog-first flow lets an officer create any empty number within quota, not just the next
	// sequential one, by passing it explicitly. Omitting it (0) keeps the auto-allocate behavior. The
	// schedule row is already locked FOR UPDATE above, so the taken-number check and the insert are
	// serialized against concurrent CreateSlot calls on the same schedule.
	slotNumber := input.SlotNumber
	if slotNumber < 1 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(slot_number),0)+1 FROM distribution_slots WHERE schedule_id=$1`, input.ScheduleID).Scan(&slotNumber); err != nil {
			return DistributionSlot{}, fmt.Errorf("allocate slot number: %w", err)
		}
	} else {
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE schedule_id=$1 AND slot_number=$2)`, input.ScheduleID, slotNumber).Scan(&taken); err != nil {
			return DistributionSlot{}, fmt.Errorf("check slot number: %w", err)
		}
		if taken {
			return DistributionSlot{}, ErrSlotNumberTaken
		}
	}
	if slotQuota != nil && slotNumber > *slotQuota {
		return DistributionSlot{}, ErrSlotQuotaExceeded
	}

	var slotID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO distribution_slots (schedule_id, slot_number, distribution_date)
		VALUES ($1,$2,NULL)
		RETURNING id::text
	`, input.ScheduleID, slotNumber).Scan(&slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("insert distribution slot: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,media_kind,require_location,require_captured_at,sort_order)
		SELECT $1,slot_code,label,stage,is_required,min_files,max_files,input_source,media_kind,require_location,require_captured_at,sort_order
		FROM documentation_template_slots WHERE template_version_id=$2
	`, slotID, documentationTemplateID); err != nil {
		return DistributionSlot{}, fmt.Errorf("snapshot documentation slots: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_created", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"schedule_id": input.ScheduleID, "slot_number": slotNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit create slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) getSlotByID(ctx context.Context, id string) (DistributionSlot, error) {
	var slot DistributionSlot
	var machineOption, machineSerial, hoseOption, hoseSerial, converterOption, converterSerial, fullName, nik, sectorIdentifier, address, village, district, phoneNumber *string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text, ds.schedule_id::text, ds.slot_number, ds.distribution_date::text, ds.status, ds.allocation_id::text,
			ds.machine_option_code, ds.machine_serial_number, ds.hose_option_code, ds.hose_serial_number, ds.converter_option_code, ds.converter_serial_number,
			ds.distributed_at, ds.created_at, ds.updated_at, p.full_name, p.nik, psi.normalized_value, p.address, p.village, p.district, p.phone_number,
			ds.needs_recompletion, ds.reopened_at, ds.reopened_by::text, COALESCE(ds.reopened_stage,''), COALESCE(ds.revision_reason,'')
		FROM distribution_slots ds
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN LATERAL (SELECT normalized_value FROM person_sector_identifiers WHERE person_id=p.id ORDER BY id LIMIT 1) psi ON true
		WHERE ds.id=$1
	`, id).Scan(&slot.ID, &slot.ScheduleID, &slot.SlotNumber, &slot.DistributionDate, &slot.Status, &slot.AllocationID, &machineOption, &machineSerial, &hoseOption, &hoseSerial, &converterOption, &converterSerial, &slot.DistributedAt, &slot.CreatedAt, &slot.UpdatedAt, &fullName, &nik, &sectorIdentifier, &address, &village, &district, &phoneNumber, &slot.NeedsRecompletion, &slot.ReopenedAt, &slot.ReopenedBy, &slot.ReopenedStage, &slot.RevisionReason)
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
	if converterOption != nil {
		slot.ConverterOptionCode = *converterOption
	}
	if converterSerial != nil {
		slot.ConverterSerialNumber = *converterSerial
	}
	if fullName != nil {
		slot.FullName = *fullName
	}
	if nik != nil {
		slot.NIK = *nik
	}
	if sectorIdentifier != nil {
		slot.SectorIdentifier = *sectorIdentifier
	}
	if address != nil {
		slot.Address = *address
	}
	if village != nil {
		slot.Village = *village
	}
	if district != nil {
		slot.District = *district
	}
	if phoneNumber != nil {
		slot.PhoneNumber = *phoneNumber
	}
	slot.Documentation, err = r.listSlotDocumentation(ctx, id)
	if err != nil {
		return DistributionSlot{}, err
	}
	return slot, nil
}

func (r *Repository) SetDistributionDate(ctx context.Context, actor auth.Principal, input SetDistributionDateInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin distribution date update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock distribution date: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET distribution_date=$2::date,updated_at=now() WHERE id=$1`, slotID, input.DistributionDate); err != nil {
		return DistributionSlot{}, fmt.Errorf("update distribution date: %w", err)
	}
	if err := r.queueMediaMovesForSlot(ctx, tx, slotID); err != nil {
		return DistributionSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.date_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"distribution_date": input.DistributionDate}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit distribution date: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) UpdateEquipment(ctx context.Context, actor auth.Principal, input UpdateEquipmentInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin equipment update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock equipment: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if _, err := tx.Exec(ctx, `
		UPDATE distribution_slots
		SET machine_option_code=NULLIF($2,''), machine_serial_number=NULLIF($3,''),
			hose_option_code=NULLIF($4,''), hose_serial_number=NULLIF($5,''),
			converter_option_code=NULLIF($6,''), converter_serial_number=NULLIF($7,''),
			updated_at=now()
		WHERE id=$1
	`, slotID, input.MachineOptionCode, input.MachineSerialNumber, input.HoseOptionCode, input.HoseSerialNumber, input.ConverterOptionCode, input.ConverterSerialNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("update equipment: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.equipment_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"machine_option_code": input.MachineOptionCode, "converter_option_code": input.ConverterOptionCode, "hose_option_code": input.HoseOptionCode}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit equipment update: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) ReopenSlot(ctx context.Context, actor auth.Principal, input ReopenSlotInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin reopen distribution slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status string
	var distributionDate *string
	var allocationID *string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, ds.allocation_id::text, ds.distribution_date::text
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &allocationID, &distributionDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock distribution slot for reopen: %w", err)
	}
	if status != "completed" {
		return DistributionSlot{}, ErrRevisionNotCompleted
	}
	if allocationID == nil {
		return DistributionSlot{}, ErrRecipientNotLinked
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET status='ready',updated_at=now() WHERE id=$1`, *allocationID); err != nil {
		return DistributionSlot{}, fmt.Errorf("reset allocation for revision: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bast_daily_bundles b SET status='stale',updated_at=now()
		WHERE b.status='active' AND EXISTS (
			SELECT 1 FROM bast_daily_bundle_items bi
			JOIN bast_individual_documents d ON d.id=bi.individual_document_id
			WHERE bi.bundle_id=b.id AND d.distribution_slot_id=$1 AND d.status='final'
		)
	`, slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("invalidate daily BA bundle: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bast_individual_documents SET status='stale' WHERE distribution_slot_id=$1 AND status='final'`, slotID); err != nil {
		return DistributionSlot{}, fmt.Errorf("invalidate individual BA: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bast_aggregate_documents SET status='stale',updated_at=now() WHERE schedule_id=$1 AND status='active'`, input.ScheduleID); err != nil {
		return DistributionSlot{}, fmt.Errorf("invalidate aggregate BA: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE distribution_slots SET status='linked',verification_snapshot_json='{}'::jsonb,
			distributed_at=NULL,distributed_by=NULL,completed_at=NULL,needs_recompletion=true,
			reopened_at=now(),reopened_by=NULLIF($2,'')::uuid,reopened_stage=$3,revision_reason=$4,updated_at=now()
		WHERE id=$1
	`, slotID, actor.UserID, input.Stage, input.Reason); err != nil {
		return DistributionSlot{}, fmt.Errorf("reopen distribution slot: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_reopened", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": *allocationID, "stage": input.Stage, "reason": input.Reason, "distribution_date": distributionDate}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit reopen distribution slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) ListSlotCatalog(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]SlotCatalogEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.slot_number, ds.status,
			NOT EXISTS(
				SELECT 1 FROM documentation_slots dcs
				LEFT JOIN (SELECT documentation_slot_id, count(*) AS accepted FROM media_files WHERE status='accepted' GROUP BY documentation_slot_id) m ON m.documentation_slot_id = dcs.id
				WHERE dcs.distribution_slot_id = ds.id AND dcs.is_required AND COALESCE(m.accepted,0) < dcs.min_files
			) AS documentation_complete
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		WHERE ds.schedule_id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY ds.slot_number
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list slot catalog: %w", err)
	}
	defer rows.Close()
	entries := []SlotCatalogEntry{}
	for rows.Next() {
		var entry SlotCatalogEntry
		if err := rows.Scan(&entry.SlotNumber, &entry.Status, &entry.DocumentationComplete); err != nil {
			return nil, fmt.Errorf("scan slot catalog entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *Repository) listSlotDocumentation(ctx context.Context, distributionSlotID string) ([]SlotSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, ds.slot_code, ds.label_snapshot, ds.stage, ds.status, ds.is_required, ds.min_files, ds.max_files, ds.input_source, ds.media_kind, ds.require_location, ds.require_captured_at
		FROM documentation_slots ds WHERE ds.distribution_slot_id=$1 ORDER BY ds.sort_order
	`, distributionSlotID)
	if err != nil {
		return nil, fmt.Errorf("list slot documentation: %w", err)
	}
	defer rows.Close()
	var summaries []SlotSummary
	for rows.Next() {
		var summary SlotSummary
		if err := rows.Scan(&summary.ID, &summary.Code, &summary.Label, &summary.Stage, &summary.Status, &summary.Required, &summary.MinFiles, &summary.MaxFiles, &summary.InputSource, &summary.MediaKind, &summary.RequireLocation, &summary.RequireCapturedAt); err != nil {
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
		SELECT id::text, documentation_slot_id::text, storage_key::text, original_filename, mime_type, byte_size, source, captured_at, latitude, longitude, status, storage_state, storage_last_error, uploaded_at
		FROM media_files WHERE documentation_slot_id=$1 AND status='accepted' ORDER BY uploaded_at
	`, documentationSlotID)
	if err != nil {
		return nil, fmt.Errorf("list media files: %w", err)
	}
	defer rows.Close()
	var files []MediaFile
	for rows.Next() {
		var file MediaFile
		if err := rows.Scan(&file.ID, &file.SlotID, &file.StorageKey, &file.OriginalFilename, &file.MimeType, &file.ByteSize, &file.Source, &file.CapturedAt, &file.Latitude, &file.Longitude, &file.Status, &file.StorageState, &file.StorageLastError, &file.UploadedAt); err != nil {
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

func (r *Repository) SuggestCandidates(ctx context.Context, scheduleID, nikPrefix string, limit int, scope auth.RegencyScope) ([]CandidateMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
			COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''), cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		LEFT JOIN LATERAL (SELECT identifier_type, normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE pa.schedule_id = $1 AND p.nik LIKE $2 || '%' AND pa.distribution_number IS NULL
			AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY p.nik, p.full_name
		LIMIT $5
	`, scheduleID, nikPrefix, scope.Unrestricted, scope.RegencyIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest candidates: %w", err)
	}
	defer rows.Close()
	items := make([]CandidateMatch, 0, limit)
	for rows.Next() {
		var item CandidateMatch
		if err := rows.Scan(&item.AllocationID, &item.FullName, &item.NIK, &item.SectorIdentifierType, &item.SectorIdentifier, &item.Address, &item.Village, &item.District, &item.PhoneNumber, &item.ProgramType); err != nil {
			return nil, fmt.Errorf("scan candidate suggestion: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("suggest candidates: %w", err)
	}
	return items, nil
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
	if err := r.queueMediaMovesForSlot(ctx, tx, slotID); err != nil {
		return DistributionSlot{}, err
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.slot_linked", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"allocation_id": allocationID, "slot_number": input.SlotNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit link slot: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) UpdateRecipient(ctx context.Context, actor auth.Principal, input UpdateRecipientInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin recipient update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status, personID, programType string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text,ds.status,p.id::text,cn.program_type
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		LEFT JOIN package_allocations pa ON pa.id=ds.allocation_id
		LEFT JOIN candidate_nominations cn ON cn.id=pa.nomination_id
		LEFT JOIN people p ON p.id=ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &personID, &programType)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock recipient slot: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if status != "linked" {
		return DistributionSlot{}, ErrRecipientNotLinked
	}
	if err := updateRecipientDetails(ctx, tx, personID, programType, input.Address, input.Village, input.District, input.PhoneNumber, input.SectorIdentifier); err != nil {
		return DistributionSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.recipient_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"slot_number": input.SlotNumber, "person_id": personID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit recipient update: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func (r *Repository) ReplaceRecipient(ctx context.Context, actor auth.Principal, input ReplaceRecipientInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin recipient replacement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status string
	var oldAllocationID, oldPersonID *string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text,ds.status,ds.allocation_id::text,ds.recipient_person_id::text
		FROM distribution_slots ds JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status, &oldAllocationID, &oldPersonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock recipient replacement slot: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if status != "linked" || oldAllocationID == nil || oldPersonID == nil {
		return DistributionSlot{}, ErrRecipientNotLinked
	}

	var newAllocationID, newPersonID, programType string
	err = tx.QueryRow(ctx, `
		SELECT pa.id::text,p.id::text,cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id=pa.nomination_id
		JOIN people p ON p.id=cn.person_id
		WHERE pa.schedule_id=$1 AND p.nik=$2 AND pa.distribution_number IS NULL
		FOR UPDATE OF pa
	`, input.ScheduleID, input.NIK).Scan(&newAllocationID, &newPersonID, &programType)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrCandidateNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock replacement candidate: %w", err)
	}
	var previouslyReceived bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_slots WHERE recipient_person_id=$1 AND status='completed' AND id<>$2)`, newPersonID, slotID).Scan(&previouslyReceived); err != nil {
		return DistributionSlot{}, fmt.Errorf("check replacement history: %w", err)
	}
	if previouslyReceived {
		return DistributionSlot{}, ErrPreviouslyReceived
	}
	if err := updateRecipientDetails(ctx, tx, newPersonID, programType, input.Address, input.Village, input.District, input.PhoneNumber, input.SectorIdentifier); err != nil {
		return DistributionSlot{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=NULL,status='candidate',actual_recipient_person_id=NULL,updated_at=now() WHERE id=$1`, *oldAllocationID); err != nil {
		return DistributionSlot{}, fmt.Errorf("release previous allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=$2,status='ready',actual_recipient_person_id=$3,updated_at=now() WHERE id=$1`, newAllocationID, input.SlotNumber, newPersonID); err != nil {
		return DistributionSlot{}, fmt.Errorf("assign replacement allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id=$2,recipient_person_id=$3,updated_at=now() WHERE id=$1`, slotID, newAllocationID, newPersonID); err != nil {
		return DistributionSlot{}, fmt.Errorf("replace slot recipient: %w", err)
	}
	if err := r.queueMediaMovesForSlot(ctx, tx, slotID); err != nil {
		return DistributionSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.recipient_replaced", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"old_allocation_id": *oldAllocationID, "new_allocation_id": newAllocationID, "old_person_id": *oldPersonID, "new_person_id": newPersonID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit recipient replacement: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

func updateRecipientDetails(ctx context.Context, tx pgx.Tx, personID, programType, address, village, district, phoneNumber, sectorIdentifier string) error {
	if _, err := tx.Exec(ctx, `UPDATE people SET address=COALESCE(NULLIF($2,''),address),village=COALESCE(NULLIF($3,''),village),district=COALESCE(NULLIF($4,''),district),phone_number=COALESCE(NULLIF($5,''),phone_number),updated_at=now() WHERE id=$1`, personID, address, village, district, phoneNumber); err != nil {
		return fmt.Errorf("update recipient details: %w", err)
	}
	if sectorIdentifier == "" {
		return nil
	}
	identifierType := "kusuka"
	if programType == "farmer" {
		identifierType = "farmer_card"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$3)
		ON CONFLICT (person_id,identifier_type) DO UPDATE SET normalized_value=EXCLUDED.normalized_value,display_value=EXCLUDED.display_value,updated_at=now()
	`, personID, identifierType, sectorIdentifier); err != nil {
		if constraint, ok := uniqueViolationConstraint(err); ok && constraint != "" {
			return ErrIdentifierConflict
		}
		return fmt.Errorf("upsert recipient sector identifier: %w", err)
	}
	return nil
}

func (r *Repository) SearchSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (DistributionSlot, error) {
	digits := stripNonDigits.ReplaceAllString(query, "")
	var id string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id::text FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ($4 OR ps.regency_id::text = ANY($5))
			AND (ds.slot_number::text = $2 OR (p.nik IS NOT NULL AND p.nik = NULLIF($3,'')))
		LIMIT 1
	`, scheduleID, query, digits, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("search slot: %w", err)
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
	var packageJSON []byte
	var equipmentSelection UpdateEquipmentInput
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status, pa.id::text, p.id::text, COALESCE(p.full_name,''), COALESCE(p.nik,''), COALESCE(psi.normalized_value,''),
			pt.values_json, COALESCE(ds.machine_option_code,''), COALESCE(ds.machine_serial_number,''),
			COALESCE(ds.hose_option_code,''), COALESCE(ds.hose_serial_number,''),
			COALESCE(ds.converter_option_code,''), COALESCE(ds.converter_serial_number,'')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN package_template_versions pt ON pt.id = ps.package_template_version_id
		LEFT JOIN package_allocations pa ON pa.id = ds.allocation_id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN LATERAL (SELECT normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(
		&slotID, &status, &allocationIDPtr, &personIDPtr, &fullName, &nik, &sectorIdentifier,
		&packageJSON, &equipmentSelection.MachineOptionCode, &equipmentSelection.MachineSerialNumber,
		&equipmentSelection.HoseOptionCode, &equipmentSelection.HoseSerialNumber,
		&equipmentSelection.ConverterOptionCode, &equipmentSelection.ConverterSerialNumber,
	)
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
	equipmentSnapshot, err := buildEquipmentVerificationSnapshot(packageJSON, equipmentSelection)
	if err != nil {
		return DistributionSlot{}, err
	}
	equipmentPayload, err := json.Marshal(equipmentSnapshot)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("encode equipment verification snapshot: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET actual_recipient_person_id=$2, status='distributed', updated_at=now() WHERE id=$1`, allocationID, personID); err != nil {
		return DistributionSlot{}, fmt.Errorf("update package allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET status='completed', verification_snapshot_json=jsonb_set(verification_snapshot_json,'{equipment}',$3::jsonb,true), distributed_at=now(), distributed_by=$2, completed_at=now(), needs_recompletion=false, updated_at=now() WHERE id=$1`, slotID, actor.UserID, string(equipmentPayload)); err != nil {
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
