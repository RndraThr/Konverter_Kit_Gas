package monitoring

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/notifications"

	"github.com/jackc/pgx/v5"
)

// Activity photos (rakor, sosialisasi, unloading, ...) are checked per photo
// like the distribution photos, grouped per kabupaten and activity type.
// Supporting documents are not part of it, and photos copied from the
// distribution (source "distribution") were checked there already.

var (
	ErrActivityAlbumNotFound = errors.New("activity album not found")
	ErrActivityFlagUnknown   = errors.New("marked photo does not belong to this activity")
)

// ActivityCell counts the photos of one activity type of a kabupaten.
type ActivityCell struct {
	ScheduleID   string     `json:"schedule_id"`
	RegencyID    string     `json:"regency_id"`
	ActivityType string     `json:"activity_type"`
	Photos       int        `json:"photos"`
	Approved     int        `json:"approved"`
	Rejected     int        `json:"rejected"`
	Pending      int        `json:"pending"`
	LastUploadAt *time.Time `json:"last_upload_at"`
}

// ActivityPhoto is one photo or video of an album, with its check.
type ActivityPhoto struct {
	ID          string       `json:"id"`
	ContentURL  string       `json:"content_url"`
	MimeType    string       `json:"mime_type"`
	DisplayName string       `json:"display_name"`
	Source      string       `json:"source"`
	UploadedAt  time.Time    `json:"uploaded_at"`
	UploadedBy  string       `json:"uploaded_by"`
	Review      *MediaReview `json:"review"`
}

// ActivityAlbum is every current photo of one activity type of a kabupaten.
type ActivityAlbum struct {
	ScheduleID   string          `json:"schedule_id"`
	RegencyID    string          `json:"regency_id"`
	RegencyName  string          `json:"regency_name"`
	ActivityType string          `json:"activity_type"`
	Photos       []ActivityPhoto `json:"photos"`
}

// ActivityRef names an album: the program and kabupaten of a schedule and the activity type.
type ActivityRef struct {
	ScheduleID   string `json:"schedule_id"`
	ActivityType string `json:"activity_type"`
}

type ActivityApproveInput struct {
	ActivityRef
	MediaIDs []string `json:"media_ids"`
}

type ActivityRevisionInput struct {
	ActivityRef
	Flags []FlagInput `json:"flags"`
	Note  string      `json:"note"`
}

// ActivityRejection is a rejected photo the field still has to replace.
type ActivityRejection struct {
	MediaID      string    `json:"media_id"`
	ActivityType string    `json:"activity_type"`
	Reason       string    `json:"reason"`
	ReviewedBy   string    `json:"reviewed_by"`
	ReviewedAt   time.Time `json:"reviewed_at"`
}

// activityReviewable is the condition on activity_media m of the photos the
// central team checks.
const activityReviewable = `m.status = 'active' AND m.activity_type <> 'dokumen_pendukung'`

// ActivityOverview counts, per kabupaten and activity type of the program,
// the photos and how many are approved, rejected or not checked yet.
func (s *Service) ActivityOverview(ctx context.Context, programID string, scope auth.RegencyScope) ([]ActivityCell, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ps.id::text, ps.regency_id::text, m.activity_type, count(*),
			count(*) FILTER (WHERE m.source = 'distribution' OR rv.status = 'approved'),
			count(*) FILTER (WHERE m.source <> 'distribution' AND rv.status = 'rejected'),
			count(*) FILTER (WHERE m.source <> 'distribution' AND rv.status IS NULL),
			max(m.uploaded_at)
		FROM activity_media m
		JOIN program_schedules ps ON ps.program_id = m.program_id AND ps.regency_id = m.regency_id AND ps.status <> 'cancelled'
		LEFT JOIN activity_media_reviews rv ON rv.media_id = m.id
		WHERE m.program_id::text = $1 AND `+activityReviewable+` AND ($2 OR ps.regency_id::text = ANY($3))
		GROUP BY ps.id, m.activity_type`, strings.TrimSpace(programID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("count activity photos: %w", err)
	}
	defer rows.Close()
	out := []ActivityCell{}
	for rows.Next() {
		var c ActivityCell
		if err := rows.Scan(&c.ScheduleID, &c.RegencyID, &c.ActivityType, &c.Photos, &c.Approved, &c.Rejected, &c.Pending, &c.LastUploadAt); err != nil {
			return nil, fmt.Errorf("scan activity photos: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type activityAlbumMeta struct{ scheduleID, programID, regencyID, regencyName string }

func activityAlbumOf(ctx context.Context, q queryer, ref ActivityRef, scope auth.RegencyScope) (activityAlbumMeta, error) {
	var m activityAlbumMeta
	if !isActivityType(ref.ActivityType) {
		return m, ErrActivityAlbumNotFound
	}
	err := q.QueryRow(ctx, `SELECT ps.id::text, ps.program_id::text, r.id::text, r.name FROM program_schedules ps JOIN regencies r ON r.id = ps.regency_id
		WHERE ps.id::text = $1 AND ($2 OR ps.regency_id::text = ANY($3))`, strings.TrimSpace(ref.ScheduleID), scope.Unrestricted, scope.RegencyIDs).
		Scan(&m.scheduleID, &m.programID, &m.regencyID, &m.regencyName)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrScheduleNotFound
	}
	if err != nil {
		return m, fmt.Errorf("get activity album: %w", err)
	}
	return m, nil
}

// ActivityAlbum lists the photos of one activity type of a kabupaten, oldest first.
func (s *Service) ActivityAlbum(ctx context.Context, ref ActivityRef, scope auth.RegencyScope) (ActivityAlbum, error) {
	m, err := activityAlbumOf(ctx, s.pool, ref, scope)
	if err != nil {
		return ActivityAlbum{}, err
	}
	out := ActivityAlbum{ScheduleID: m.scheduleID, RegencyID: m.regencyID, RegencyName: m.regencyName, ActivityType: ref.ActivityType, Photos: []ActivityPhoto{}}
	rows, err := s.pool.Query(ctx, `
		SELECT m.id::text, m.mime_type, m.display_name, m.source, m.uploaded_at, `+userName("up")+`,
			rv.status, COALESCE(rv.reason, ''), `+userName("ru")+`, rv.reviewed_at
		FROM activity_media m
		LEFT JOIN users up ON up.id = m.uploaded_by
		LEFT JOIN activity_media_reviews rv ON rv.media_id = m.id
		LEFT JOIN users ru ON ru.id = rv.reviewed_by
		WHERE m.program_id::text = $1 AND m.regency_id::text = $2 AND m.activity_type = $3 AND `+activityReviewable+`
		ORDER BY m.uploaded_at, m.id`, m.programID, m.regencyID, ref.ActivityType)
	if err != nil {
		return out, fmt.Errorf("list activity photos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p ActivityPhoto
		var status *string
		var reason, reviewer string
		var reviewedAt *time.Time
		if err := rows.Scan(&p.ID, &p.MimeType, &p.DisplayName, &p.Source, &p.UploadedAt, &p.UploadedBy, &status, &reason, &reviewer, &reviewedAt); err != nil {
			return out, fmt.Errorf("scan activity photo: %w", err)
		}
		p.ContentURL = "/api/v1/activities/media/" + p.ID + "/content"
		if status != nil && reviewedAt != nil {
			p.Review = &MediaReview{Status: *status, Reason: reason, ReviewedBy: reviewer, ReviewedAt: *reviewedAt}
		}
		out.Photos = append(out.Photos, p)
	}
	return out, rows.Err()
}

// albumMedia maps the album's photos that can be checked (not copied from the
// distribution) to their check status ("" when not checked) and uploader.
func albumMedia(ctx context.Context, q queryer, m activityAlbumMeta, activityType string) (map[string][2]string, error) {
	rows, err := q.Query(ctx, `
		SELECT m.id::text, COALESCE(rv.status, ''), COALESCE(m.uploaded_by::text, '')
		FROM activity_media m LEFT JOIN activity_media_reviews rv ON rv.media_id = m.id
		WHERE m.program_id::text = $1 AND m.regency_id::text = $2 AND m.activity_type = $3 AND m.source <> 'distribution' AND `+activityReviewable,
		m.programID, m.regencyID, activityType)
	if err != nil {
		return nil, fmt.Errorf("list album photos: %w", err)
	}
	defer rows.Close()
	out := map[string][2]string{}
	for rows.Next() {
		var id, status, uploader string
		if err := rows.Scan(&id, &status, &uploader); err != nil {
			return nil, fmt.Errorf("scan album photo: %w", err)
		}
		out[id] = [2]string{status, uploader}
	}
	return out, rows.Err()
}

// ApproveActivity approves photos of an album: the ones given, or every
// photo not checked yet.
func (s *Service) ApproveActivity(ctx context.Context, actor auth.Principal, input ActivityApproveInput, scope auth.RegencyScope, meta auth.ClientMeta) (ActivityAlbum, error) {
	m, err := activityAlbumOf(ctx, s.pool, input.ActivityRef, scope)
	if err != nil {
		return ActivityAlbum{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := albumMedia(ctx, tx, m, input.ActivityType)
		if err != nil {
			return err
		}
		ids := []string{}
		if len(input.MediaIDs) == 0 {
			for id, v := range current {
				if v[0] == "" {
					ids = append(ids, id)
				}
			}
		} else {
			for _, id := range input.MediaIDs {
				id = strings.TrimSpace(id)
				if _, ok := current[id]; !ok {
					return ErrActivityFlagUnknown
				}
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) == 0 {
			return ErrNothingToApprove
		}
		for _, id := range ids {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_media_reviews (media_id, status, reason, reviewed_by, reviewed_at) VALUES ($1::uuid, 'approved', '', NULLIF($2, '')::uuid, now())
				ON CONFLICT (media_id) DO UPDATE SET status = 'approved', reason = '', reviewed_by = EXCLUDED.reviewed_by, reviewed_at = now()`, id, actor.UserID); err != nil {
				return fmt.Errorf("approve activity photo: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.activity_photos_approved", ResourceType: "program_schedule", ResourceID: m.scheduleID,
			Metadata: map[string]any{"activity_type": input.ActivityType, "media_ids": ids}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent})
	})
	if err != nil {
		return ActivityAlbum{}, err
	}
	return s.ActivityAlbum(ctx, input.ActivityRef, scope)
}

// RequestActivityRevision rejects photos of an album and tells who uploaded
// them and the kabupaten's leaders, who replace them in the app.
func (s *Service) RequestActivityRevision(ctx context.Context, actor auth.Principal, input ActivityRevisionInput, scope auth.RegencyScope, meta auth.ClientMeta) (ActivityAlbum, error) {
	if len(input.Flags) == 0 {
		return ActivityAlbum{}, ErrRevisionEmpty
	}
	m, err := activityAlbumOf(ctx, s.pool, input.ActivityRef, scope)
	if err != nil {
		return ActivityAlbum{}, err
	}
	note := strings.TrimSpace(input.Note)
	label := ActivityLabel(input.ActivityType)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := albumMedia(ctx, tx, m, input.ActivityType)
		if err != nil {
			return err
		}
		users := []string{}
		reasons := []string{}
		seen := map[string]bool{}
		for _, f := range input.Flags {
			id := strings.TrimSpace(f.MediaID)
			v, ok := current[id]
			if !ok {
				return ErrActivityFlagUnknown
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			reason := strings.TrimSpace(f.Reason)
			if len(reason) > 300 {
				reason = reason[:300]
			}
			if reason != "" && !slices.Contains(reasons, reason) {
				reasons = append(reasons, reason)
			}
			users = append(users, v[1])
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_media_reviews (media_id, status, reason, reviewed_by, reviewed_at) VALUES ($1::uuid, 'rejected', $2, NULLIF($3, '')::uuid, now())
				ON CONFLICT (media_id) DO UPDATE SET status = 'rejected', reason = EXCLUDED.reason, reviewed_by = EXCLUDED.reviewed_by, reviewed_at = now()`, id, reason, actor.UserID); err != nil {
				return fmt.Errorf("reject activity photo: %w", err)
			}
		}
		leaders, err := leadersOf(ctx, tx, m.scheduleID, m.regencyID)
		if err != nil {
			return err
		}
		users = slices.DeleteFunc(append(users, leaders...), func(id string) bool { return id == "" || id == actor.UserID })
		detail := strings.Join(reasons, "; ")
		if note != "" {
			detail = strings.TrimPrefix(detail+"; "+note, "; ")
		}
		body := fmt.Sprintf("%s %s: %d foto perlu diganti", label, m.regencyName, len(seen))
		if detail != "" {
			body += " (" + detail + ")"
		}
		if len(body) > 300 {
			body = body[:297] + "..."
		}
		if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.activity_photos_rejected", ResourceType: "program_schedule", ResourceID: m.scheduleID,
			Metadata: map[string]any{"activity_type": input.ActivityType, "flags": input.Flags, "note": note}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, users, notifications.Message{
			Category: notifications.CategoryDistributionRevision,
			Title:    "Revisi foto " + label,
			Body:     body,
			Link:     fmt.Sprintf("/kegiatan/%s?schedule=%s&revisi=1", input.ActivityType, m.scheduleID),
		})
	})
	if err != nil {
		return ActivityAlbum{}, err
	}
	if s.notifier != nil {
		s.notifier.Wake()
	}
	return s.ActivityAlbum(ctx, input.ActivityRef, scope)
}

// ActivityRejections lists the rejected photos of a kabupaten that are still
// there, so the app can show which ones to replace.
func (s *Service) ActivityRejections(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]ActivityRejection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id::text, m.activity_type, rv.reason, `+userName("ru")+`, rv.reviewed_at
		FROM activity_media m
		JOIN program_schedules ps ON ps.program_id = m.program_id AND ps.regency_id = m.regency_id
		JOIN activity_media_reviews rv ON rv.media_id = m.id AND rv.status = 'rejected'
		LEFT JOIN users ru ON ru.id = rv.reviewed_by
		WHERE ps.id::text = $1 AND `+activityReviewable+` AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY rv.reviewed_at DESC`, strings.TrimSpace(scheduleID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list rejected activity photos: %w", err)
	}
	defer rows.Close()
	out := []ActivityRejection{}
	for rows.Next() {
		var r ActivityRejection
		if err := rows.Scan(&r.MediaID, &r.ActivityType, &r.Reason, &r.ReviewedBy, &r.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan rejected activity photo: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// activityLabels are the activity types the central team checks, as the app names them.
var activityLabels = map[string]string{
	"kunjungan_dinas": "Kunjungan Kantor Dinas", "gudang_titik_serah": "Gudang Titik Serah",
	"unloading_konkit": "Unloading Konkit", "unloading_mesin_pompa": "Unloading Mesin Pompa", "unloading_oli": "Unloading Oli",
	"unloading_selang": "Unloading Selang", "unloading_tabung_gas": "Unloading Tabung Gas", "perakitan_mesin": "Perakitan Mesin",
	"rakor": "Rakor", "pemeriksaan_barang": "Pemeriksaan Barang",
	"ceremony_sosialisasi": "Ceremony & Sosialisasi", "training_10": "Training 10%", "pelatihan_teknis": "Pelatihan Teknis",
	"kegiatan_distribusi": "Kegiatan Pendistribusian", "training_100": "Training 100%",
	"closing_kabupaten": "Closing Kabupaten", "kegiatan": "Kegiatan Lainnya",
}

func isActivityType(code string) bool { _, ok := activityLabels[code]; return ok }

// ActivityLabel is the name of an activity type.
func ActivityLabel(code string) string {
	if label, ok := activityLabels[code]; ok {
		return label
	}
	return code
}
