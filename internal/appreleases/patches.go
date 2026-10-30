package appreleases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"time"

	"github.com/jackc/pgx/v5"
)

// patchSources is how many earlier releases get a patch to the newest one;
// phones further behind download the whole APK.
const patchSources = 3

// releaseReady (SQL on app_releases r): the release is offered to phones
// once a patch (or "no patch") is recorded from each of its patchSources
// earlier releases, so phones see the small download from the first
// notification. Should making patches fail, it is offered 10 minutes after
// the upload anyway. Recording a patch signals phones (migration 00084).
const releaseReady = `(r.created_at < now() - interval '10 minutes' OR NOT EXISTS (
	SELECT 1 FROM (SELECT s.id FROM app_releases s WHERE s.platform = r.platform AND s.version_code < r.version_code
		ORDER BY s.version_code DESC LIMIT 3) src
	WHERE NOT EXISTS (SELECT 1 FROM app_release_patches p WHERE p.release_id = r.id AND p.from_release_id = src.id)))`

// pendingReleases are the releases not offered yet (patches being made).
func (s *Service) pendingReleases(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text FROM app_releases r WHERE NOT `+releaseReady)
	if err != nil {
		return nil, fmt.Errorf("list pending app releases: %w", err)
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

// Patch is the delta a phone on an earlier release can download instead of
// the whole APK.
type Patch struct {
	DownloadPath string `json:"download_path"`
	SizeBytes    int64  `json:"size_bytes"`
	// Checksum is the patch file's SHA-256; FromChecksum the APK it applies
	// to (the phone checks its installed APK against it first).
	Checksum     string `json:"checksum"`
	FromChecksum string `json:"from_checksum"`
}

// PatchSummary is shown with a release on the website.
type PatchSummary struct {
	FromVersionName string `json:"from_version_name"`
	FromVersionCode int    `json:"from_version_code"`
	SizeBytes       int64  `json:"size_bytes"`
	Ready           bool   `json:"ready"`
}

type patchJob struct {
	releaseID, releaseKey string
	fromID, fromKey       string
	newSize               int64
}

// patchFor is the ready patch from the release with fromCode to releaseID.
func (s *Service) patchFor(ctx context.Context, platform, releaseID string, fromCode int) (*Patch, error) {
	var patch Patch
	err := s.pool.QueryRow(ctx, `SELECT p.size_bytes, p.checksum, f.checksum FROM app_release_patches p
		JOIN app_releases f ON f.id = p.from_release_id
		WHERE p.release_id::text = $1 AND f.platform = $2 AND f.version_code = $3 AND p.storage_key IS NOT NULL`,
		releaseID, platform, fromCode).Scan(&patch.SizeBytes, &patch.Checksum, &patch.FromChecksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("app release patch: %w", err)
	}
	patch.DownloadPath = fmt.Sprintf("/api/v1/app/releases/%s/patch/%d", releaseID, fromCode)
	return &patch, nil
}

func (s *Service) patchSummaries(ctx context.Context) (map[string][]PatchSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.release_id::text, f.version_name, f.version_code, p.size_bytes, p.storage_key IS NOT NULL
		FROM app_release_patches p JOIN app_releases f ON f.id = p.from_release_id ORDER BY f.version_code DESC`)
	if err != nil {
		return nil, fmt.Errorf("list app release patches: %w", err)
	}
	defer rows.Close()
	result := map[string][]PatchSummary{}
	for rows.Next() {
		var id string
		var summary PatchSummary
		if err := rows.Scan(&id, &summary.FromVersionName, &summary.FromVersionCode, &summary.SizeBytes, &summary.Ready); err != nil {
			return nil, fmt.Errorf("scan app release patch: %w", err)
		}
		result[id] = append(result[id], summary)
	}
	return result, rows.Err()
}

// OpenPatch streams the patch from the release with fromCode to release id.
func (s *Service) OpenPatch(ctx context.Context, id string, fromCode int) (Content, error) {
	if s.storage == nil {
		return Content{}, errMissingStorage
	}
	var key string
	var size int64
	var name string
	err := s.pool.QueryRow(ctx, `SELECT p.storage_key, p.size_bytes, r.version_name FROM app_release_patches p
		JOIN app_releases r ON r.id = p.release_id JOIN app_releases f ON f.id = p.from_release_id
		WHERE p.release_id::text = $1 AND f.platform = r.platform AND f.version_code = $2 AND p.storage_key IS NOT NULL`,
		id, fromCode).Scan(&key, &size, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, fmt.Errorf("get app release patch: %w", err)
	}
	reader, err := s.storage.Open(ctx, key)
	if err != nil {
		return Content{}, fmt.Errorf("open app release patch: %w", err)
	}
	return Content{Reader: reader, Size: size, Filename: fmt.Sprintf("konkit-%s-from-%d.patch", name, fromCode)}, nil
}

// patchStorageKeys are the files of every patch to or from release id.
func (s *Service) patchStorageKeys(ctx context.Context, id string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT storage_key FROM app_release_patches
		WHERE (release_id::text = $1 OR from_release_id::text = $1) AND storage_key IS NOT NULL`, id)
	if err != nil {
		return nil, fmt.Errorf("list app release patch files: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Service) missingPatch(ctx context.Context) (*patchJob, error) {
	var job patchJob
	err := s.pool.QueryRow(ctx, `WITH latest AS (
			SELECT id, storage_key, size_bytes, version_code FROM app_releases WHERE platform = 'android' ORDER BY version_code DESC LIMIT 1
		), sources AS (
			SELECT r.id, r.storage_key FROM app_releases r, latest
			WHERE r.platform = 'android' AND r.version_code < latest.version_code
			ORDER BY r.version_code DESC LIMIT $1
		)
		SELECT latest.id::text, latest.storage_key, sources.id::text, sources.storage_key, latest.size_bytes FROM latest, sources
		WHERE NOT EXISTS (SELECT 1 FROM app_release_patches p WHERE p.release_id = latest.id AND p.from_release_id = sources.id)
		LIMIT 1`, patchSources).Scan(&job.releaseID, &job.releaseKey, &job.fromID, &job.fromKey, &job.newSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find missing app release patch: %w", err)
	}
	return &job, nil
}

func (s *Service) readStored(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.storage.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, MaxDeltaInput+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxDeltaInput {
		return nil, errors.New("APK too large for a delta patch")
	}
	return data, nil
}

// makePatch builds and stores one patch. A patch not much smaller than the
// APK is recorded as "none".
func (s *Service) makePatch(ctx context.Context, job patchJob) error {
	var key, checksum string
	var size int64
	patch, err := func() ([]byte, error) {
		oldData, err := s.readStored(ctx, job.fromKey)
		if err != nil {
			return nil, err
		}
		newData, err := s.readStored(ctx, job.releaseKey)
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if err := Diff(oldData, newData, &out); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}()
	debug.FreeOSMemory()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		log.Printf("app release patch %s -> %s: %v", job.fromID, job.releaseID, err)
	} else if int64(len(patch))*10 < job.newSize*6 {
		key, size, checksum, err = s.storage.Put(ctx, fmt.Sprintf("apk-patch-%s-%s-%d", job.fromID, job.releaseID, time.Now().UnixNano()), nil, bytes.NewReader(patch))
		if err != nil {
			return fmt.Errorf("store app release patch: %w", err)
		}
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO app_release_patches (release_id, from_release_id, storage_key, size_bytes, checksum)
		SELECT $1::uuid, $2::uuid, NULLIF($3, ''), $4, $5
		WHERE EXISTS (SELECT 1 FROM app_releases WHERE id = $1::uuid) AND EXISTS (SELECT 1 FROM app_releases WHERE id = $2::uuid)
		ON CONFLICT DO NOTHING`, job.releaseID, job.fromID, key, size, checksum)
	if err != nil || tag.RowsAffected() == 0 {
		if key != "" {
			_ = s.storage.Delete(ctx, key)
		}
		if err != nil {
			return fmt.Errorf("record app release patch: %w", err)
		}
	}
	return nil
}

// RunPatches makes the missing patches for the newest release, one at a
// time (each holds both APKs and a suffix array in memory), checking every
// minute and right after an upload.
func (s *Service) RunPatches(ctx context.Context) {
	if s.storage == nil {
		return
	}
	for {
		job, err := s.missingPatch(ctx)
		if err == nil && job != nil {
			err = s.makePatch(ctx, *job)
			if err == nil {
				continue
			}
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("app release patches: %v", err)
		}
		timer := time.NewTimer(time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.uploaded:
			timer.Stop()
		case <-timer.C:
		}
	}
}
