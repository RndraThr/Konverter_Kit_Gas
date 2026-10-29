package activities

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/jackc/pgx/v5"
)

// Foto Training dari distribusi: setiap foto slot recipient_training pada
// penerimaan yang selesai disalin ke dokumentasi kegiatan Training 100%.
// Foto dari 10% penerima pertama tiap tanggal distribusi (urutan nomor bagi,
// sama dengan peserta BA Training 10%) juga disalin ke Training 10%. Salinan
// berupa berkas sendiri di folder kegiatan, sehingga tampil di halaman
// Dokumentasi Kegiatan, folder Drive, dan pilihan foto laporan akhir.

const trainingPhotoSlotCode = "recipient_training"

// jakarta names copies by the photo's local upload time.
var jakarta = func() *time.Location {
	if location, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return location
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// trainingPhotoCopy is one distribution photo that belongs in an activity type.
type trainingPhotoCopy struct {
	DistributionMediaID string
	ProgramID           string
	RegencyID           string
	ActivityType        string
	StorageKey          string
	MimeType            string
	SlotNumber          int
	UploadedAt          time.Time
}

// staleTrainingCopy is a copy whose source photo no longer qualifies.
type staleTrainingCopy struct {
	ID         string
	StorageKey string
}

// trainingPhotoQualifying lists (photo, activity type) pairs that should
// exist as activity media. 10% = ceil(n/10) first completed slots of the
// schedule's distribution date, the same participants as BA Training 10%.
const trainingPhotoQualifying = `
WITH done AS (
	SELECT ds.id, ds.slot_number, ps.program_id, ps.regency_id,
	       row_number() OVER w AS rn, count(*) OVER w AS n
	FROM distribution_slots ds
	JOIN program_schedules ps ON ps.id = ds.schedule_id
	WHERE ds.status = 'completed' AND ds.distributed_at IS NOT NULL
	WINDOW w AS (PARTITION BY ds.schedule_id, COALESCE(ds.distribution_date, (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date)
	             ORDER BY ds.slot_number, ds.id ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING)
), qualifying AS (
	SELECT mf.id AS media_id, done.program_id, done.regency_id, t.activity_type, mf.storage_key::text AS storage_key,
	       mf.mime_type, done.slot_number, mf.uploaded_at
	FROM done
	JOIN documentation_slots doc ON doc.distribution_slot_id = done.id AND doc.slot_code = '` + trainingPhotoSlotCode + `'
	JOIN media_files mf ON mf.documentation_slot_id = doc.id AND mf.status = 'accepted'
	CROSS JOIN (VALUES ('training_100'), ('training_10')) AS t(activity_type)
	WHERE t.activity_type = 'training_100' OR done.rn <= ceil(done.n / 10.0)
)
`

// MissingTrainingCopies returns qualifying photos without an active copy,
// skipping copies a user deleted on purpose.
func (r *Repository) MissingTrainingCopies(ctx context.Context, limit int) ([]trainingPhotoCopy, error) {
	rows, err := r.pool.Query(ctx, trainingPhotoQualifying+`
		SELECT q.media_id::text, q.program_id::text, q.regency_id::text, q.activity_type, q.storage_key, q.mime_type, q.slot_number, q.uploaded_at
		FROM qualifying q
		WHERE NOT EXISTS (
			SELECT 1 FROM activity_media m
			WHERE m.distribution_media_id = q.media_id AND m.activity_type = q.activity_type
			  AND (m.status = 'active' OR NOT m.removed_by_sync)
		)
		ORDER BY q.uploaded_at, q.media_id, q.activity_type DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list missing training copies: %w", err)
	}
	defer rows.Close()
	var items []trainingPhotoCopy
	for rows.Next() {
		var item trainingPhotoCopy
		if err := rows.Scan(&item.DistributionMediaID, &item.ProgramID, &item.RegencyID, &item.ActivityType, &item.StorageKey, &item.MimeType, &item.SlotNumber, &item.UploadedAt); err != nil {
			return nil, fmt.Errorf("scan missing training copy: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// StaleTrainingCopies returns active copies whose photo no longer qualifies
// (photo deleted, distribution reopened, or no longer in the first 10%).
func (r *Repository) StaleTrainingCopies(ctx context.Context, limit int) ([]staleTrainingCopy, error) {
	rows, err := r.pool.Query(ctx, trainingPhotoQualifying+`
		SELECT m.id::text, m.storage_key
		FROM activity_media m
		WHERE m.source = 'distribution' AND m.status = 'active'
		  AND NOT EXISTS (SELECT 1 FROM qualifying q WHERE q.media_id = m.distribution_media_id AND q.activity_type = m.activity_type)
		ORDER BY m.uploaded_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale training copies: %w", err)
	}
	defer rows.Close()
	var items []staleTrainingCopy
	for rows.Next() {
		var item staleTrainingCopy
		if err := rows.Scan(&item.ID, &item.StorageKey); err != nil {
			return nil, fmt.Errorf("scan stale training copy: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// InsertTrainingCopy records a stored copy. ok is false when another
// instance inserted the same copy first (the caller then drops its file).
func (r *Repository) InsertTrainingCopy(ctx context.Context, item trainingPhotoCopy, storageKey, displayName string, size int64, checksum string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin insert training copy: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO activity_media (program_id, regency_id, activity_type, storage_key, display_name, original_filename,
			media_type, mime_type, byte_size, checksum, source, distribution_media_id, uploaded_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$5,'image',$6,$7,$8,'distribution',$9::uuid,$10)
		ON CONFLICT (distribution_media_id, activity_type) WHERE distribution_media_id IS NOT NULL AND status = 'active' DO NOTHING
		RETURNING id::text
	`, item.ProgramID, item.RegencyID, item.ActivityType, storageKey, displayName, item.MimeType, size, checksum, item.DistributionMediaID, item.UploadedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert training copy: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{
		Action: "activity_media.copied_from_distribution", ResourceType: "activity_media", ResourceID: id,
		Metadata: map[string]any{"activity_type": item.ActivityType, "regency_id": item.RegencyID, "distribution_media_id": item.DistributionMediaID},
	}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// RemoveTrainingCopy soft-deletes a stale copy, marked as removed by the
// copier so it is copied again if the photo qualifies later.
func (r *Repository) RemoveTrainingCopy(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE activity_media SET status = 'deleted', removed_by_sync = true, updated_at = now()
		WHERE id = $1::uuid AND status = 'active'
	`, id); err != nil {
		return fmt.Errorf("remove training copy: %w", err)
	}
	return nil
}

type trainingPhotoRepository interface {
	GetRegency(ctx context.Context, regencyID string, scope auth.RegencyScope) (regencyInfo, error)
	MissingTrainingCopies(ctx context.Context, limit int) ([]trainingPhotoCopy, error)
	StaleTrainingCopies(ctx context.Context, limit int) ([]staleTrainingCopy, error)
	InsertTrainingCopy(ctx context.Context, item trainingPhotoCopy, storageKey, displayName string, size int64, checksum string) (bool, error)
	RemoveTrainingCopy(ctx context.Context, id string) error
}

// TrainingPhotoWorker keeps the Training activity documentation in step with
// the distribution's training photos.
type TrainingPhotoWorker struct {
	service  *Service
	repo     trainingPhotoRepository
	interval time.Duration
	// failed holds photos whose copy failed, retried after failRetry.
	failed    map[string]time.Time
	failRetry time.Duration
}

const trainingPhotoBatch = 50

func NewTrainingPhotoWorker(service *Service) *TrainingPhotoWorker {
	repo, _ := service.repository.(trainingPhotoRepository)
	return &TrainingPhotoWorker{service: service, repo: repo, interval: 2 * time.Minute, failed: map[string]time.Time{}, failRetry: 30 * time.Minute}
}

// Run copies and removes until ctx ends, pausing when there is nothing to do.
func (w *TrainingPhotoWorker) Run(ctx context.Context) {
	if w.repo == nil {
		return
	}
	for {
		busy, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("training photo worker: %v", err)
		}
		if busy && err == nil {
			continue
		}
		timer := time.NewTimer(w.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// RunOnce handles one batch. busy is true when a full batch was copied, so
// more may be waiting.
func (w *TrainingPhotoWorker) RunOnce(ctx context.Context) (bool, error) {
	stale, err := w.repo.StaleTrainingCopies(ctx, trainingPhotoBatch)
	if err != nil {
		return false, err
	}
	for _, item := range stale {
		if err := w.repo.RemoveTrainingCopy(ctx, item.ID); err != nil {
			return false, err
		}
		_ = w.service.storage.Delete(ctx, item.StorageKey)
	}
	missing, err := w.repo.MissingTrainingCopies(ctx, trainingPhotoBatch+len(w.failed))
	if err != nil {
		return false, err
	}
	now := w.service.now()
	copied := 0
	for _, item := range missing {
		key := item.DistributionMediaID + "/" + item.ActivityType
		if at, ok := w.failed[key]; ok && now.Sub(at) < w.failRetry {
			continue
		}
		if copied == trainingPhotoBatch {
			break
		}
		if err := w.copy(ctx, item); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			log.Printf("training photo worker: copy %s to %s: %v", item.DistributionMediaID, item.ActivityType, err)
			w.failed[key] = now
			continue
		}
		delete(w.failed, key)
		copied++
	}
	return copied == trainingPhotoBatch || len(stale) == trainingPhotoBatch, nil
}

func (w *TrainingPhotoWorker) copy(ctx context.Context, item trainingPhotoCopy) error {
	s := w.service
	system := auth.RegencyScope{Unrestricted: true}
	storageContext, err := s.programContexts.ResolveStorageContext(ctx, item.ProgramID, item.RegencyID, system)
	if err != nil {
		return err
	}
	regency, err := w.repo.GetRegency(ctx, item.RegencyID, system)
	if err != nil {
		return err
	}
	category, child := folderDestination(item.ActivityType)
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName,
		RegencyName: storageContext.RegencyName, Category: category, Child: child,
	})
	if err != nil {
		return err
	}
	source, err := s.storage.Open(ctx, item.StorageKey)
	if err != nil {
		return err
	}
	defer source.Close()
	key, err := newStorageKey()
	if err != nil {
		return err
	}
	displayName := fmt.Sprintf("%s-%s-%04d-%s", regency.DocumentCode, activityTypeCodes[item.ActivityType], item.SlotNumber, item.UploadedAt.In(jakarta).Format("20060102-150405"))
	storageKey, size, checksum, err := media.PutNamed(ctx, s.storage, key, displayName+activityFileExtension(item.MimeType), folderPath, source)
	if err != nil {
		return err
	}
	inserted, err := w.repo.InsertTrainingCopy(ctx, item, storageKey, displayName, size, checksum)
	if err != nil || !inserted {
		_ = s.storage.Delete(context.Background(), storageKey)
	}
	return err
}
