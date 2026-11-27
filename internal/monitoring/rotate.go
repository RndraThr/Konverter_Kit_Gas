package monitoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"konkit/internal/activities"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/distribution"
	"konkit/internal/media"

	"github.com/jackc/pgx/v5"
)

// ErrRotateTarget: the file to rotate is not (or no longer) part of the schedule.
var ErrRotateTarget = errors.New("file to rotate not found")

// ErrRotateUnavailable: no file storage is configured for rotation.
var ErrRotateUnavailable = errors.New("file rotation unavailable")

// Upper bounds of the tables (byte_size checks) and of what is read to rotate.
const (
	maxDistributionPhoto = 10 << 20
	maxActivityFile      = 100 << 20
	maxRotateRead        = 100 << 20
)

// RotateInput turns one file of a schedule a quarter (or half) turn clockwise.
// Kind: photo (distribution photo, id = media id), activity (activity file,
// id = media id) or ba (uploaded BA, id = the BA item key).
type RotateInput struct {
	ScheduleID string `json:"schedule_id"`
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Degrees    int    `json:"degrees"`
}

// RotateResult is where the rotated file is read now (a new URL, so caches
// of the old content are not used).
type RotateResult struct {
	ContentURL string `json:"content_url"`
}

// WithStorage gives the service the file storage, needed to rotate files.
func (s *Service) WithStorage(storage media.Storage) *Service {
	s.storage = storage
	return s
}

// Rotate rewrites the file itself, turned: the stored copy (Google Drive or
// local) and the database copy of BA scans. Everyone, also the field app and
// the reports made later, sees the rotated file.
func (s *Service) Rotate(ctx context.Context, actor auth.Principal, input RotateInput, scope auth.RegencyScope, meta auth.ClientMeta) (RotateResult, error) {
	degrees, err := media.NormalizeDegrees(input.Degrees)
	if err != nil {
		return RotateResult{}, err
	}
	scheduleID, err := s.scheduleInScope(ctx, input.ScheduleID, scope)
	if err != nil {
		return RotateResult{}, err
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return RotateResult{}, ErrRotateTarget
	}
	var result RotateResult
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		switch input.Kind {
		case "photo":
			result, err = s.rotatePhoto(ctx, tx, scheduleID, id, degrees)
		case "activity":
			result, err = s.rotateActivity(ctx, tx, scheduleID, id, degrees)
		case "ba":
			result, err = s.rotateBA(ctx, tx, scheduleID, id, degrees)
		default:
			return ErrRotateTarget
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.file_rotated", ResourceType: "program_schedule", ResourceID: scheduleID,
			Metadata: map[string]any{"kind": input.Kind, "id": id, "degrees": degrees}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent})
	})
	return result, err
}

// rotateStored rotates a file kept in the storage, in place (same Drive file).
func (s *Service) rotateStored(ctx context.Context, storageKey, mimeType string, degrees int, maxBytes int64) (int64, string, error) {
	if s.storage == nil {
		return 0, "", ErrRotateUnavailable
	}
	if degrees == 0 {
		return 0, "", nil
	}
	reader, err := s.storage.Open(ctx, storageKey)
	if err != nil {
		return 0, "", fmt.Errorf("open file to rotate: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxRotateRead+1))
	_ = reader.Close()
	if err != nil {
		return 0, "", fmt.Errorf("read file to rotate: %w", err)
	}
	if len(data) > maxRotateRead {
		return 0, "", media.ErrRotateUnsupported
	}
	rotated, err := media.RotateFile(data, mimeType, degrees, maxBytes)
	if err != nil {
		return 0, "", err
	}
	if int64(len(rotated)) > maxBytes {
		return 0, "", media.ErrRotateUnsupported
	}
	return media.Replace(ctx, s.storage, storageKey, bytes.NewReader(rotated))
}

func (s *Service) rotatePhoto(ctx context.Context, tx pgx.Tx, scheduleID, id string, degrees int) (RotateResult, error) {
	var storageKey, mimeType string
	var version int
	err := tx.QueryRow(ctx, `
		SELECT m.storage_key::text, m.mime_type, m.content_version
		FROM media_files m
		JOIN documentation_slots dcs ON dcs.id = m.documentation_slot_id
		JOIN distribution_slots ds ON ds.id = dcs.distribution_slot_id
		WHERE m.id::text = $1 AND ds.schedule_id::text = $2 AND m.status = 'accepted'
		FOR UPDATE OF m`, id, scheduleID).Scan(&storageKey, &mimeType, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return RotateResult{}, ErrRotateTarget
	}
	if err != nil {
		return RotateResult{}, fmt.Errorf("find photo to rotate: %w", err)
	}
	if degrees == 0 {
		return RotateResult{ContentURL: distribution.MediaContentURL(id, version)}, nil
	}
	size, checksum, err := s.rotateStored(ctx, storageKey, mimeType, degrees, maxDistributionPhoto)
	if err != nil {
		return RotateResult{}, err
	}
	if err := tx.QueryRow(ctx, `UPDATE media_files SET byte_size = $2, checksum = $3, content_version = content_version + 1, updated_at = now()
		WHERE id::text = $1 RETURNING content_version`, id, size, checksum).Scan(&version); err != nil {
		return RotateResult{}, fmt.Errorf("save rotated photo: %w", err)
	}
	return RotateResult{ContentURL: distribution.MediaContentURL(id, version)}, nil
}

func (s *Service) rotateActivity(ctx context.Context, tx pgx.Tx, scheduleID, id string, degrees int) (RotateResult, error) {
	var storageKey, mimeType string
	var version int
	err := tx.QueryRow(ctx, `
		SELECT m.storage_key, m.mime_type, m.content_version
		FROM activity_media m
		JOIN program_schedules ps ON ps.program_id = m.program_id AND ps.regency_id = m.regency_id
		WHERE m.id::text = $1 AND ps.id::text = $2 AND m.status = 'active'
		FOR UPDATE OF m`, id, scheduleID).Scan(&storageKey, &mimeType, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return RotateResult{}, ErrRotateTarget
	}
	if err != nil {
		return RotateResult{}, fmt.Errorf("find activity file to rotate: %w", err)
	}
	if degrees == 0 {
		return RotateResult{ContentURL: activities.MediaContentURL(id, version)}, nil
	}
	size, checksum, err := s.rotateStored(ctx, storageKey, mimeType, degrees, maxActivityFile)
	if err != nil {
		return RotateResult{}, err
	}
	if err := tx.QueryRow(ctx, `UPDATE activity_media SET byte_size = $2, checksum = $3, content_version = content_version + 1, updated_at = now()
		WHERE id::text = $1 RETURNING content_version`, id, size, checksum).Scan(&version); err != nil {
		return RotateResult{}, fmt.Errorf("save rotated activity file: %w", err)
	}
	return RotateResult{ContentURL: activities.MediaContentURL(id, version)}, nil
}

// rotateBA rotates an uploaded BA: a full scan (PDF) or a signature page
// (PNG) kept in the database, or a signed sheet kept in the storage.
func (s *Service) rotateBA(ctx context.Context, tx pgx.Tx, scheduleID, key string, degrees int) (RotateResult, error) {
	items, err := s.baItems(ctx, tx, []string{scheduleID})
	if err != nil {
		return RotateResult{}, err
	}
	var item *BAItem
	for i := range items[scheduleID] {
		if items[scheduleID][i].Key == key {
			item = &items[scheduleID][i]
			break
		}
	}
	if item == nil {
		return RotateResult{}, ErrRotateTarget
	}
	kind, rest, _ := strings.Cut(key, ":")
	reread := func() (RotateResult, error) {
		items, err := s.baItems(ctx, tx, []string{scheduleID})
		if err != nil {
			return RotateResult{}, err
		}
		for _, it := range items[scheduleID] {
			if it.Key == key {
				return RotateResult{ContentURL: it.ContentURL}, nil
			}
		}
		return RotateResult{}, ErrRotateTarget
	}
	if degrees == 0 {
		return RotateResult{ContentURL: item.ContentURL}, nil
	}
	switch kind {
	case "scan", "wet":
		i := strings.LastIndex(rest, ":")
		if i < 0 {
			return RotateResult{}, ErrRotateTarget
		}
		document, day := rest[:i], rest[i+1:]
		table, column, mimeType := "bast_full_scans", "pdf", "application/pdf"
		if kind == "wet" {
			table, column, mimeType = "bast_wet_signature_scans", "image_png", "image/png"
		}
		var data []byte
		err := tx.QueryRow(ctx, `SELECT `+column+` FROM `+table+` WHERE schedule_id::text = $1 AND document_key = $2 AND document_date = $3::date FOR UPDATE`,
			scheduleID, document, day).Scan(&data)
		if errors.Is(err, pgx.ErrNoRows) {
			return RotateResult{}, ErrRotateTarget
		}
		if err != nil {
			return RotateResult{}, fmt.Errorf("find BA to rotate: %w", err)
		}
		rotated, err := media.RotateFile(data, mimeType, degrees, 0)
		if err != nil {
			return RotateResult{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET `+column+` = $4, byte_size = $5, content_version = content_version + 1
			WHERE schedule_id::text = $1 AND document_key = $2 AND document_date = $3::date`, scheduleID, document, day, rotated, len(rotated)); err != nil {
			return RotateResult{}, fmt.Errorf("save rotated BA: %w", err)
		}
	case "archive":
		var storageKey, mimeType string
		err := tx.QueryRow(ctx, `SELECT storage_key, mime_type FROM bast_rakorda_uploads WHERE id::text = $1 AND schedule_id::text = $2 FOR UPDATE`,
			rest, scheduleID).Scan(&storageKey, &mimeType)
		if errors.Is(err, pgx.ErrNoRows) {
			return RotateResult{}, ErrRotateTarget
		}
		if err != nil {
			return RotateResult{}, fmt.Errorf("find BA sheet to rotate: %w", err)
		}
		size, _, err := s.rotateStored(ctx, storageKey, mimeType, degrees, maxRotateRead)
		if err != nil {
			return RotateResult{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_rakorda_uploads SET byte_size = $2, content_version = content_version + 1 WHERE id::text = $1`, rest, size); err != nil {
			return RotateResult{}, fmt.Errorf("save rotated BA sheet: %w", err)
		}
	default:
		// BA Perorangan is drawn from the BA itself: nothing stored to turn.
		return RotateResult{}, media.ErrRotateUnsupported
	}
	return reread()
}
