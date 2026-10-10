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
	var programType, regencyName, recipientName string
	var zoneName *string
	var zonePlaceholder *bool
	if err := tx.QueryRow(ctx, `
		SELECT ds.distribution_date,ds.slot_number,(ds.recipient_person_id IS NOT NULL),p.program_type,z.name,r.name,z.is_placeholder,COALESCE(rp.full_name,'')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		JOIN programs p ON p.id=ps.program_id
		JOIN regencies r ON r.id=ps.regency_id
		LEFT JOIN people rp ON rp.id=ds.recipient_person_id
		LEFT JOIN program_regency_assignments pra ON pra.program_id=ps.program_id AND pra.regency_id=ps.regency_id
		LEFT JOIN program_zones z ON z.id=pra.zone_id
		WHERE ds.id=$1
	`, distributionSlotID).Scan(&distributionDate, &slotNumber, &hasRecipient, &programType, &zoneName, &regencyName, &zonePlaceholder, &recipientName); err != nil {
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
		RETURNING m.id::text,m.storage_target_generation,m.mime_type,ds.label_snapshot,ds.max_files,
			(SELECT count(*) FROM media_files m2
			 WHERE m2.documentation_slot_id=m.documentation_slot_id AND m2.status='accepted'
			   AND (m2.uploaded_at,m2.id)<=(m.uploaded_at,m.id))
	`, distributionSlotID, mediaID)
	if err != nil {
		return fmt.Errorf("prepare media moves: %w", err)
	}
	type generatedMove struct {
		mediaID    string
		generation int64
		filename   string
	}
	moves := make([]generatedMove, 0)
	for rows.Next() {
		var move generatedMove
		var mimeType, label string
		var maxFiles, sequence int
		if err := rows.Scan(&move.mediaID, &move.generation, &mimeType, &label, &maxFiles, &sequence); err != nil {
			rows.Close()
			return fmt.Errorf("scan prepared media move: %w", err)
		}
		move.filename = formatDistributionFinalMediaFilename(recipientName, label, mimeType, sequence, maxFiles)
		moves = append(moves, move)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("prepare media moves: %w", err)
	}
	rows.Close()

	for _, move := range moves {
		if _, err := tx.Exec(ctx, `
			INSERT INTO distribution_media_move_jobs(media_file_id,target_path,target_filename,target_generation,status,attempts,next_attempt_at,locked_at,last_error)
			VALUES($1,$2,$3,$4,'queued',0,now(),NULL,'')
			ON CONFLICT(media_file_id) DO UPDATE SET
				target_path=EXCLUDED.target_path,target_filename=EXCLUDED.target_filename,target_generation=EXCLUDED.target_generation,
				status='queued',attempts=0,next_attempt_at=now(),locked_at=NULL,last_error='',updated_at=now()
		`, move.mediaID, targetPath, move.filename, move.generation); err != nil {
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

func (r *Repository) ClaimMediaMove(ctx context.Context, now time.Time, leaseDuration time.Duration) (MediaMoveJob, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MediaMoveJob{}, false, fmt.Errorf("begin media move claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var job MediaMoveJob
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT j.media_file_id
			FROM distribution_media_move_jobs j
			JOIN media_files m ON m.id=j.media_file_id
			WHERE m.status='accepted' AND (
				(j.status IN ('queued','retry') AND j.next_attempt_at <= $1)
				OR (j.status='processing' AND (j.locked_at IS NULL OR j.locked_at < $2))
			)
			ORDER BY j.next_attempt_at,j.updated_at
			FOR UPDATE OF j SKIP LOCKED
			LIMIT 1
		)
		UPDATE distribution_media_move_jobs j
		SET status='processing',attempts=j.attempts+1,locked_at=$1,updated_at=$1
		FROM candidate c,media_files m
		WHERE j.media_file_id=c.media_file_id AND m.id=j.media_file_id
		RETURNING j.media_file_id::text,m.storage_key::text,j.target_path,j.target_filename,j.target_generation,j.attempts
	`, now, now.Add(-leaseDuration)).Scan(&job.MediaFileID, &job.StorageKey, &job.TargetPath, &job.TargetFilename, &job.TargetGeneration, &job.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaMoveJob{}, false, nil
	}
	if err != nil {
		return MediaMoveJob{}, false, fmt.Errorf("claim media move: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MediaMoveJob{}, false, fmt.Errorf("commit media move claim: %w", err)
	}
	return job, true, nil
}

func (r *Repository) CompleteMediaMove(ctx context.Context, mediaID string, generation int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin media move completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var mediaGeneration, jobGeneration int64
	var targetFilename string
	err = tx.QueryRow(ctx, `
		SELECT m.storage_target_generation,j.target_generation,j.target_filename
		FROM media_files m
		JOIN distribution_media_move_jobs j ON j.media_file_id=m.id
		WHERE m.id=$1 AND m.status='accepted'
		FOR UPDATE OF m,j
	`, mediaID).Scan(&mediaGeneration, &jobGeneration, &targetFilename)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock media move completion: %w", err)
	}
	if mediaGeneration == generation && jobGeneration == generation {
		// Persist the applied Drive name so the application keeps showing the same filename the
		// worker just gave the file. An empty target_filename means "keep the upload-time name".
		if _, err := tx.Exec(ctx, `UPDATE media_files SET storage_state='final',storage_last_error='',original_filename=CASE WHEN NULLIF($2,'') IS NULL THEN original_filename ELSE $2 END,updated_at=now() WHERE id=$1`, mediaID, targetFilename); err != nil {
			return fmt.Errorf("finalize media move: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM distribution_media_move_jobs WHERE media_file_id=$1 AND target_generation=$2`, mediaID, generation); err != nil {
			return fmt.Errorf("delete completed media move: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE distribution_media_move_jobs SET status='queued',next_attempt_at=now(),locked_at=NULL,updated_at=now() WHERE media_file_id=$1`, mediaID); err != nil {
			return fmt.Errorf("requeue newer media target: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit media move completion: %w", err)
	}
	return nil
}

func (r *Repository) FailMediaMove(ctx context.Context, mediaID string, generation int64, message string, nextAttempt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin media move failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var mediaGeneration, jobGeneration int64
	err = tx.QueryRow(ctx, `
		SELECT m.storage_target_generation,j.target_generation
		FROM media_files m
		JOIN distribution_media_move_jobs j ON j.media_file_id=m.id
		WHERE m.id=$1 AND m.status='accepted'
		FOR UPDATE OF m,j
	`, mediaID).Scan(&mediaGeneration, &jobGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock failed media move: %w", err)
	}
	if mediaGeneration == generation && jobGeneration == generation {
		if _, err := tx.Exec(ctx, `UPDATE media_files SET storage_state='move_failed',storage_last_error=$2,updated_at=now() WHERE id=$1`, mediaID, message); err != nil {
			return fmt.Errorf("mark media move failed: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE distribution_media_move_jobs SET status='retry',next_attempt_at=$3,locked_at=NULL,last_error=$2,updated_at=now() WHERE media_file_id=$1 AND target_generation=$4`, mediaID, message, nextAttempt, generation); err != nil {
			return fmt.Errorf("schedule media move retry: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE distribution_media_move_jobs SET status='queued',next_attempt_at=now(),locked_at=NULL,updated_at=now() WHERE media_file_id=$1`, mediaID); err != nil {
			return fmt.Errorf("preserve newer media target: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit media move failure: %w", err)
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

	var slotID, slotStatus string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text,ds.status
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text = ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &slotStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock distribution date: %w", err)
	}
	if slotStatus == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
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

func (r *Repository) UpdateEquipmentSerials(ctx context.Context, actor auth.Principal, input UpdateEquipmentSerialsInput, meta auth.ClientMeta, scope auth.RegencyScope) (DistributionSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("begin equipment serial update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var slotID, status string
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text,ds.status
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		FOR UPDATE OF ds
	`, input.ScheduleID, input.SlotNumber, scope.Unrestricted, scope.RegencyIDs).Scan(&slotID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return DistributionSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return DistributionSlot{}, fmt.Errorf("lock equipment serials: %w", err)
	}
	if status == "completed" {
		return DistributionSlot{}, ErrAlreadyCompleted
	}
	if _, err := tx.Exec(ctx, `
		UPDATE distribution_slots
		SET machine_serial_number=NULLIF($2,''),converter_serial_number=NULLIF($3,''),updated_at=now()
		WHERE id=$1
	`, slotID, input.MachineSerialNumber, input.ConverterSerialNumber); err != nil {
		return DistributionSlot{}, fmt.Errorf("update equipment serials: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.equipment_serials_updated", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"slot_number": input.SlotNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit equipment serial update: %w", err)
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
			) AS documentation_complete,
			ds.needs_recompletion,
			GREATEST(
				ds.updated_at,
				(SELECT max(dcs.updated_at) FROM documentation_slots dcs WHERE dcs.distribution_slot_id = ds.id),
				(SELECT max(mf.updated_at) FROM media_files mf
					JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
					WHERE dcs.distribution_slot_id = ds.id)
			) AS last_activity_at
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
		if err := rows.Scan(&entry.SlotNumber, &entry.Status, &entry.DocumentationComplete, &entry.NeedsRecompletion, &entry.LastActivityAt); err != nil {
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

// receivableAllocationPredicate is the single definition of "this allocation may still receive a
// package". It is shared by the candidate search and by every mutation that mounts a recipient,
// because the suggestion list is convenience only — the backend is the security boundary. Status
// needs_review, cancelled, replaced, and distributed never receive.
const receivableAllocationPredicate = `pa.distribution_number IS NULL AND pa.status IN ('candidate','ready')`

// notPreviouslyReceivedPredicate excludes people who already completed a distribution elsewhere.
const notPreviouslyReceivedPredicate = `NOT EXISTS (
	SELECT 1 FROM distribution_slots done
	WHERE done.recipient_person_id = p.id AND done.status='completed'
)`

func (r *Repository) SuggestCandidates(ctx context.Context, scheduleID, nikPrefix string, limit int, scope auth.RegencyScope) ([]CandidateMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.id::text, p.full_name, COALESCE(p.nik,''), COALESCE(psi.identifier_type,''), COALESCE(psi.normalized_value,''),
			COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), COALESCE(p.phone_number,''), cn.program_type
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		LEFT JOIN LATERAL (SELECT identifier_type, normalized_value FROM person_sector_identifiers WHERE person_id = p.id LIMIT 1) psi ON true
		WHERE pa.schedule_id = $1 AND p.nik LIKE $2 || '%'
			AND `+receivableAllocationPredicate+`
			AND `+notPreviouslyReceivedPredicate+`
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

// LookupCandidate classifies a single NIK so the interface can explain an empty suggestion list
// instead of guessing. It deliberately returns no recipient payload: only the state and the
// allocation facts needed for the message and for the "create new recipient" decision.
func (r *Repository) LookupCandidate(ctx context.Context, scheduleID, nik string, scope auth.RegencyScope) (CandidateLookup, error) {
	var status, fullName string
	var distributionNumber *int
	var previouslyReceived bool
	err := r.pool.QueryRow(ctx, `
		SELECT pa.status, COALESCE(p.full_name,''), pa.distribution_number,
			EXISTS(SELECT 1 FROM distribution_slots done WHERE done.recipient_person_id = p.id AND done.status='completed')
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN program_schedules ps ON ps.id = pa.schedule_id
		JOIN people p ON p.id = cn.person_id
		WHERE pa.schedule_id = $1 AND p.nik = $2 AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY pa.created_at, pa.id
		LIMIT 1
	`, scheduleID, nik, scope.Unrestricted, scope.RegencyIDs).Scan(&status, &fullName, &distributionNumber, &previouslyReceived)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateLookup{State: CandidateStateNotRegistered}, nil
	}
	if err != nil {
		return CandidateLookup{}, fmt.Errorf("lookup candidate: %w", err)
	}
	result := CandidateLookup{FullName: fullName, AllocationStatus: status, DistributionNumber: distributionNumber}
	switch {
	case status == "needs_review":
		result.State = CandidateStateNeedsReview
	case status != "candidate" && status != "ready":
		// cancelled, replaced, and distributed are all terminal for this allocation; the officer
		// must repair the data in Data Penerima rather than mount it here.
		result.State = CandidateStateNotAvailable
	case previouslyReceived:
		result.State = CandidateStatePreviouslyReceived
	case distributionNumber != nil:
		result.State = CandidateStateAlreadyAssigned
	default:
		result.State = CandidateStateReceivable
	}
	return result, nil
}

// lockedCandidate is a package_allocations row locked for a recipient mutation, together with the
// facts that decide whether it may still receive.
type lockedCandidate struct {
	AllocationID       string
	PersonID           string
	ProgramType        string
	Status             string
	DistributionNumber *int
}

// lockCandidateByNIK locks the oldest allocation for a NIK in a schedule regardless of its status.
// Locking first and classifying afterwards is what lets callers answer with a precise reason
// instead of a blanket "not found", and it stops the status changing between check and write.
func lockCandidateByNIK(ctx context.Context, tx pgx.Tx, scheduleID, nik string) (lockedCandidate, error) {
	var candidate lockedCandidate
	err := tx.QueryRow(ctx, `
		SELECT pa.id::text, p.id::text, cn.program_type, pa.status, pa.distribution_number
		FROM package_allocations pa
		JOIN candidate_nominations cn ON cn.id = pa.nomination_id
		JOIN people p ON p.id = cn.person_id
		WHERE pa.schedule_id=$1 AND p.nik=$2
		ORDER BY (pa.distribution_number IS NULL AND pa.status IN ('candidate','ready')) DESC, pa.created_at, pa.id
		LIMIT 1
		FOR UPDATE OF pa
	`, scheduleID, nik).Scan(&candidate.AllocationID, &candidate.PersonID, &candidate.ProgramType, &candidate.Status, &candidate.DistributionNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedCandidate{}, ErrCandidateNotFound
	}
	if err != nil {
		return lockedCandidate{}, fmt.Errorf("lock candidate: %w", err)
	}
	return candidate, nil
}

// receivableCandidateError explains why a locked candidate may not receive, or returns nil when it
// may. Allocation status is reported before the distribution number: an allocation cancelled in
// Data Penerima that still carries a stale number is better described as unavailable than as
// already assigned to another number.
func receivableCandidateError(candidate lockedCandidate) error {
	switch {
	case candidate.Status == "needs_review":
		return ErrCandidateNeedsReview
	case candidate.Status != "candidate" && candidate.Status != "ready":
		return ErrCandidateNotAvailable
	case candidate.DistributionNumber != nil:
		return ErrCandidateAlreadyAssigned
	}
	return nil
}

// previousReceiptError guards the person-level rule that a recipient may only receive once.
func previousReceiptError(ctx context.Context, tx pgx.Tx, personID, excludeSlotID string) error {
	var previouslyReceived bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM distribution_slots
			WHERE recipient_person_id=$1 AND status='completed' AND ($2 = '' OR id <> NULLIF($2,'')::uuid)
		)
	`, personID, excludeSlotID).Scan(&previouslyReceived)
	if err != nil {
		return fmt.Errorf("check previously received: %w", err)
	}
	if previouslyReceived {
		return ErrPreviouslyReceived
	}
	return nil
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

	candidate, err := lockCandidateByNIK(ctx, tx, input.ScheduleID, input.NIK)
	if err != nil {
		return DistributionSlot{}, err
	}
	if err := receivableCandidateError(candidate); err != nil {
		return DistributionSlot{}, err
	}
	if err := previousReceiptError(ctx, tx, candidate.PersonID, ""); err != nil {
		return DistributionSlot{}, err
	}
	allocationID, personID, programType := candidate.AllocationID, candidate.PersonID, candidate.ProgramType

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

	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return DistributionSlot{}, ErrReplacementReasonRequired
	}

	newAllocationID, newPersonID, programType, origin, err := r.resolveReplacementAllocation(ctx, tx, input, *oldAllocationID)
	if err != nil {
		return DistributionSlot{}, err
	}
	if err := previousReceiptError(ctx, tx, newPersonID, slotID); err != nil {
		return DistributionSlot{}, err
	}
	if err := updateRecipientDetails(ctx, tx, newPersonID, programType, input.Address, input.Village, input.District, input.PhoneNumber, input.SectorIdentifier); err != nil {
		return DistributionSlot{}, err
	}
	// The original allocation is released from the slot and marked replaced, so it stops appearing
	// as the recipient of this number while remaining visible in Data Penerima and reports as a
	// replaced allocation rather than disappearing entirely.
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=NULL,status='replaced',actual_recipient_person_id=NULL,updated_at=now() WHERE id=$1`, *oldAllocationID); err != nil {
		return DistributionSlot{}, fmt.Errorf("release previous allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE package_allocations SET distribution_number=$2,status='ready',actual_recipient_person_id=$3,updated_at=now() WHERE id=$1`, newAllocationID, input.SlotNumber, newPersonID); err != nil {
		return DistributionSlot{}, fmt.Errorf("assign replacement allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE distribution_slots SET allocation_id=$2,recipient_person_id=$3,updated_at=now() WHERE id=$1`, slotID, newAllocationID, newPersonID); err != nil {
		return DistributionSlot{}, fmt.Errorf("replace slot recipient: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recipient_replacements(distribution_slot_id,old_allocation_id,old_person_id,new_allocation_id,new_person_id,origin,reason,replaced_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid)
	`, slotID, *oldAllocationID, *oldPersonID, newAllocationID, newPersonID, origin, reason, actor.UserID); err != nil {
		return DistributionSlot{}, fmt.Errorf("record recipient replacement: %w", err)
	}
	if err := r.queueMediaMovesForSlot(ctx, tx, slotID); err != nil {
		return DistributionSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "distribution.recipient_replaced", ResourceType: "distribution_slot", ResourceID: slotID, Metadata: map[string]any{"old_allocation_id": *oldAllocationID, "new_allocation_id": newAllocationID, "old_person_id": *oldPersonID, "new_person_id": newPersonID, "origin": origin, "reason": reason}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return DistributionSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DistributionSlot{}, fmt.Errorf("commit recipient replacement: %w", err)
	}
	return r.getSlotByID(ctx, slotID)
}

// resolveReplacementAllocation decides which allocation the replacement recipient will hold.
//
// A substitute who already has a receivable allocation in this schedule keeps it (existing
// allocation). Anyone else — a NIK that is registered only in another schedule, or not registered
// at all — receives a fresh nomination and allocation carrying this slot's distribution number.
// The person row is always reused when the NIK already exists, because people_nik_uq only permits
// one row per NIK and a second insert would be rejected anyway.
func (r *Repository) resolveReplacementAllocation(ctx context.Context, tx pgx.Tx, input ReplaceRecipientInput, oldAllocationID string) (allocationID, personID, programType, origin string, err error) {
	candidate, err := lockCandidateByNIK(ctx, tx, input.ScheduleID, input.NIK)
	if err == nil {
		// Checked before the eligibility rules: the current recipient's own allocation carries this
		// slot's number, so it would otherwise be reported as "already assigned elsewhere".
		if candidate.AllocationID == oldAllocationID {
			return "", "", "", "", ErrReplacementSameRecipient
		}
		if receivableErr := receivableCandidateError(candidate); receivableErr != nil {
			return "", "", "", "", receivableErr
		}
		return candidate.AllocationID, candidate.PersonID, candidate.ProgramType, "existing_allocation", nil
	}
	if !errors.Is(err, ErrCandidateNotFound) {
		return "", "", "", "", err
	}

	personID, err = resolveReplacementPerson(ctx, tx, input)
	if err != nil {
		return "", "", "", "", err
	}
	var scheduledProgramType string
	if err := tx.QueryRow(ctx, `
		SELECT prog.program_type FROM program_schedules ps
		JOIN programs prog ON prog.id=ps.program_id
		WHERE ps.id=$1
	`, input.ScheduleID).Scan(&scheduledProgramType); err != nil {
		return "", "", "", "", fmt.Errorf("resolve replacement schedule program: %w", err)
	}
	var nominationID string
	if err := tx.QueryRow(ctx, `INSERT INTO candidate_nominations(person_id,program_type,source_snapshot_json,status) VALUES($1,$2,'{}'::jsonb,'ready') RETURNING id::text`,
		personID, scheduledProgramType).Scan(&nominationID); err != nil {
		return "", "", "", "", fmt.Errorf("insert replacement nomination: %w", err)
	}
	allocationSnapshot := "{}"
	if err := tx.QueryRow(ctx, `SELECT COALESCE(package_snapshot_json,'{}'::jsonb)::text FROM package_allocations WHERE id=(SELECT allocation_id FROM distribution_slots WHERE schedule_id=$1 AND slot_number=$2)`,
		input.ScheduleID, input.SlotNumber).Scan(&allocationSnapshot); err != nil {
		return "", "", "", "", fmt.Errorf("resolve replacement package snapshot: %w", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,status,package_snapshot_json) VALUES($1,$2,$3,'ready',$4::jsonb) RETURNING id::text`,
		input.ScheduleID, nominationID, personID, allocationSnapshot).Scan(&allocationID); err != nil {
		return "", "", "", "", fmt.Errorf("insert replacement allocation: %w", err)
	}
	return allocationID, personID, scheduledProgramType, "new_allocation", nil
}

const peopleNIKUniqueIndex = "people_nik_uq"

// resolveReplacementPerson reuses the person row for a NIK, creating one only when the NIK is
// genuinely unknown. Creating identity data is the operation that most needs guarding, so the full
// name is required and a lost race against a concurrent registration reuses the winner's row
// instead of failing.
func resolveReplacementPerson(ctx context.Context, tx pgx.Tx, input ReplaceRecipientInput) (string, error) {
	var personID string
	err := tx.QueryRow(ctx, `SELECT id::text FROM people WHERE nik=$1`, input.NIK).Scan(&personID)
	if err == nil {
		return personID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find replacement person: %w", err)
	}
	fullName := strings.TrimSpace(input.FullName)
	if fullName == "" {
		return "", ErrReplacementNameRequired
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO people(full_name,nik,address,village,district,phone_number) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text
	`, fullName, input.NIK, input.Address, input.Village, input.District, input.PhoneNumber).Scan(&personID)
	if err == nil {
		return personID, nil
	}
	if constraint, ok := uniqueViolationConstraint(err); ok && constraint == peopleNIKUniqueIndex {
		if lookupErr := tx.QueryRow(ctx, `SELECT id::text FROM people WHERE nik=$1`, input.NIK).Scan(&personID); lookupErr != nil {
			return "", fmt.Errorf("resolve raced replacement person: %w", lookupErr)
		}
		return personID, nil
	}
	return "", fmt.Errorf("insert replacement person: %w", err)
}

// ListReplacements returns a slot's replacement history, oldest first, so POS Dokumen can show that
// a recipient was replaced and Data Penerima can explain how the number changed hands.
func (r *Repository) ListReplacements(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) ([]RecipientReplacement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rr.id::text, ds.slot_number,
			rr.old_person_id::text, COALESCE(old_person.full_name,''),
			rr.new_person_id::text, COALESCE(new_person.full_name,''),
			rr.origin, rr.reason, COALESCE(u.full_name,''), rr.replaced_at
		FROM recipient_replacements rr
		JOIN distribution_slots ds ON ds.id=rr.distribution_slot_id
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		JOIN people old_person ON old_person.id=rr.old_person_id
		JOIN people new_person ON new_person.id=rr.new_person_id
		LEFT JOIN users u ON u.id=rr.replaced_by
		WHERE ds.schedule_id=$1 AND ds.slot_number=$2 AND ($3 OR ps.regency_id::text=ANY($4))
		ORDER BY rr.replaced_at, rr.id
	`, scheduleID, slotNumber, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list recipient replacements: %w", err)
	}
	defer rows.Close()
	items := []RecipientReplacement{}
	for rows.Next() {
		var item RecipientReplacement
		if err := rows.Scan(&item.ID, &item.SlotNumber, &item.OldPersonID, &item.OldFullName, &item.NewPersonID, &item.NewFullName, &item.Origin, &item.Reason, &item.ReplacedByName, &item.ReplacedAt); err != nil {
			return nil, fmt.Errorf("scan recipient replacement: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
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
	var distributionDate *time.Time
	var fullName, nik, sectorIdentifier string
	var packageJSON []byte
	var equipmentSelection UpdateEquipmentInput
	err = tx.QueryRow(ctx, `
		SELECT ds.id::text, ds.status,ds.distribution_date, pa.id::text, p.id::text, COALESCE(p.full_name,''), COALESCE(p.nik,''), COALESCE(psi.normalized_value,''),
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
		&slotID, &status, &distributionDate, &allocationIDPtr, &personIDPtr, &fullName, &nik, &sectorIdentifier,
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
	if distributionDate == nil {
		return DistributionSlot{}, ErrDistributionDateRequired
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
	var hasFailedMove bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM documentation_slots ds
			JOIN media_files m ON m.documentation_slot_id=ds.id
			WHERE ds.distribution_slot_id=$1 AND ds.is_required AND m.status='accepted' AND m.storage_state='move_failed'
		)
	`, slotID).Scan(&hasFailedMove); err != nil {
		return DistributionSlot{}, fmt.Errorf("check failed media moves: %w", err)
	}
	if hasFailedMove {
		return DistributionSlot{}, ErrMediaMoveFailed
	}
	var hasPendingMove bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM documentation_slots ds
			JOIN media_files m ON m.documentation_slot_id=ds.id
			WHERE ds.distribution_slot_id=$1 AND ds.is_required AND m.status='accepted' AND m.storage_state<>'final'
		)
	`, slotID).Scan(&hasPendingMove); err != nil {
		return DistributionSlot{}, fmt.Errorf("check pending media moves: %w", err)
	}
	if hasPendingMove {
		return DistributionSlot{}, ErrMediaMovePending
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

// FindSerialMatches returns up to 5 non-cancelled slots across all schedules
// whose machine or converter serial equals serial (already uppercased).
// Regency scope is intentionally not applied: a unit recorded in another
// regency is still a duplicate. Only the slot number and regency are exposed.
func (r *Repository) FindSerialMatches(ctx context.Context, serial, excludeScheduleID string, excludeSlotNumber int) ([]SerialMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ps.id::text, ps.name, rg.name, ds.slot_number,
			CASE WHEN upper(ds.machine_serial_number) = $1 THEN 'machine' ELSE 'converter' END,
			ds.status
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN regencies rg ON rg.id = ps.regency_id
		WHERE ds.status <> 'cancelled'
			AND (upper(ds.machine_serial_number) = $1 OR upper(ds.converter_serial_number) = $1)
			AND NOT (ds.schedule_id::text = $2 AND ds.slot_number = $3)
		ORDER BY ds.updated_at DESC
		LIMIT 5
	`, serial, excludeScheduleID, excludeSlotNumber)
	if err != nil {
		return nil, fmt.Errorf("find serial matches: %w", err)
	}
	defer rows.Close()
	matches := []SerialMatch{}
	for rows.Next() {
		var match SerialMatch
		if err := rows.Scan(&match.ScheduleID, &match.ScheduleName, &match.RegencyName, &match.SlotNumber, &match.Field, &match.Status); err != nil {
			return nil, fmt.Errorf("scan serial match: %w", err)
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}
