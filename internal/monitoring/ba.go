package monitoring

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/notifications"

	"github.com/jackc/pgx/v5"
)

// BA (berita acara) check. A BA appears once the field uploaded its signed
// version: a full wet-signed scan, a signature-page scan, or the signed
// attendance sheets of an archive BA. BA Perorangan is checked per recipient
// in the recipient's review. A check holds for the version it saw; a new
// scan or a re-finalized BA is a new version, not checked yet.

var ErrBAItemUnknown = errors.New("BA item does not belong to this kabupaten")

// baTypes are the BA checked in the BA tab, in the app's order, with the
// document key of their wet-signed scans and, for archive BA, the kind of
// their uploads. BA Perorangan ("individual") is checked per recipient.
var baTypes = []struct{ code, label, wetKey, archiveKind string }{
	{"gas-elpiji", "BA Gas LPG", "gas-elpiji", ""},
	{"rakorda", "Rakorda", "", "rakorda"},
	{"pemeriksaan", "BA Pemeriksaan", "ba-pemeriksaan", ""},
	{"sosialisasi", "Sosialisasi", "sosialisasi", "sosialisasi"},
	{"training-10", "Training 10%", "training-10", "training_10"},
	{"dp3", "DP3", "dp3", ""},
	{"daily-recap", "Rekap Harian", "rekap-harian", ""},
	{"training-100", "Training 100%", "training-100", "training_100"},
	{"closing-titik-serah", "Closing Titik Serah", "closing-titik-serah", ""},
	{"closing-kabupaten", "Closing Kabupaten", "closing-kabupaten", ""},
	{"servis-berkala", "Servis Berkala", "servis-berkala", ""},
	{"tkdn", "TKDN", "tkdn", ""},
}

const perorangan = "individual"

// baTypeOf is the BA type of a wet-scan document key or an archive kind.
func baTypeOf(wetKey, archiveKind string) (string, string) {
	if strings.HasPrefix(wetKey, "closing-titik-serah@") {
		wetKey = "closing-titik-serah"
	}
	for _, t := range baTypes {
		if (wetKey != "" && t.wetKey == wetKey) || (archiveKind != "" && t.archiveKind == archiveKind) {
			return t.code, t.label
		}
	}
	return "", ""
}

// BAItem is one uploaded BA to check.
type BAItem struct {
	Key        string       `json:"key"`
	Type       string       `json:"type"`
	Label      string       `json:"label"`
	Date       string       `json:"date"`
	Source     string       `json:"source"` // scan (full PDF), wet (signature page image), archive (signed sheets)
	ContentURL string       `json:"content_url"`
	MimeType   string       `json:"mime_type"`
	UploadedBy string       `json:"uploaded_by"`
	UploadedAt time.Time    `json:"uploaded_at"`
	Review     *MediaReview `json:"review"`
	// Individual: for BA Perorangan, what the BA preview needs to show the
	// recipient's page (the day's scan when uploaded, else the digital BA).
	Individual *IndividualRef `json:"individual,omitempty"`
	version    string
	uploaderID string
}

// IndividualRef names one recipient's BA Perorangan.
type IndividualRef struct {
	ProgramID  string `json:"program_id"`
	RegencyID  string `json:"regency_id"`
	LocalDate  string `json:"local_date"`
	SlotID     string `json:"slot_id"`
	SlotNumber int    `json:"slot_number"`
}

// BACell counts the uploaded BA of one type of a kabupaten.
type BACell struct {
	ScheduleID string `json:"schedule_id"`
	Type       string `json:"type"`
	Items      int    `json:"items"`
	Approved   int    `json:"approved"`
	Rejected   int    `json:"rejected"`
	Pending    int    `json:"pending"`
}

type BAApproveInput struct {
	ScheduleID string   `json:"schedule_id"`
	Keys       []string `json:"keys"`
}

type BARevisionInput struct {
	ScheduleID string `json:"schedule_id"`
	Flags      []struct {
		Key    string `json:"key"`
		Reason string `json:"reason"`
	} `json:"flags"`
	Note string `json:"note"`
}

// baItems lists the uploaded BA of the schedules with their current check.
// BA Perorangan: one item per recipient whose BA is final ("individual:<slot>"),
// its version following the BA and the day's wet-signed scan.
func (s *Service) baItems(ctx context.Context, q queryer, scheduleIDs []string) (map[string][]BAItem, error) {
	rows, err := q.Query(ctx, `
		WITH items AS (
			SELECT f.schedule_id, 'scan:' || f.document_key || ':' || f.document_date AS key, f.document_key AS wet_key, '' AS archive_kind,
				f.document_date::text AS day, 'scan' AS source, f.content_code || '@' || f.uploaded_at AS version,
				f.uploaded_by, f.uploaded_at, 'application/pdf' AS mime, '' AS upload_id
			FROM bast_full_scans f WHERE f.schedule_id = ANY($1::uuid[]) AND f.document_key <> 'ba-perorangan'
			UNION ALL
			SELECT w.schedule_id, 'wet:' || w.document_key || ':' || w.document_date, w.document_key, '', w.document_date::text, 'wet',
				w.content_code || '@' || w.uploaded_at, w.uploaded_by, w.uploaded_at, 'image/png', ''
			FROM bast_wet_signature_scans w
			WHERE w.schedule_id = ANY($1::uuid[]) AND w.document_key <> 'ba-perorangan'
				AND NOT EXISTS (SELECT 1 FROM bast_full_scans f WHERE f.schedule_id = w.schedule_id AND f.document_key = w.document_key AND f.document_date = w.document_date)
			UNION ALL
			SELECT u.schedule_id, 'archive:' || u.id, '', u.document_kind, u.event_date::text, 'archive', u.id::text,
				u.uploaded_by, u.created_at, u.mime_type, u.id::text
			FROM bast_rakorda_uploads u WHERE u.schedule_id = ANY($1::uuid[])
			UNION ALL
			SELECT ds.schedule_id, 'individual:' || d.distribution_slot_id, 'ba-perorangan', '', d.local_date::text,
				CASE WHEN f.schedule_id IS NULL THEN 'digital' ELSE 'scan' END,
				d.id || '@' || COALESCE(f.uploaded_at::text, ''), COALESCE(f.uploaded_by, d.finalized_by), COALESCE(f.uploaded_at, d.finalized_at), 'application/pdf',
				d.distribution_slot_id::text || '|' || d.program_id || '|' || d.regency_id || '|' || d.slot_number
			FROM bast_individual_documents d
			JOIN distribution_slots ds ON ds.id = d.distribution_slot_id
			LEFT JOIN bast_full_scans f ON f.schedule_id = ds.schedule_id AND f.document_key = 'ba-perorangan' AND f.document_date = d.local_date
			WHERE d.status = 'final' AND ds.schedule_id = ANY($1::uuid[])
		)
		SELECT i.schedule_id::text, i.key, i.wet_key, i.archive_kind, i.day, i.source, i.version, COALESCE(i.uploaded_by::text, ''), `+userName("up")+`,
			i.uploaded_at, i.mime, i.upload_id, rv.status, COALESCE(rv.reason, ''), `+userName("ru")+`, rv.reviewed_at
		FROM items i
		LEFT JOIN users up ON up.id = i.uploaded_by
		LEFT JOIN ba_reviews rv ON rv.schedule_id = i.schedule_id AND rv.item_key = i.key AND rv.version = i.version
		LEFT JOIN users ru ON ru.id = rv.reviewed_by
		ORDER BY i.day, i.key`, scheduleIDs)
	if err != nil {
		return nil, fmt.Errorf("list BA uploads: %w", err)
	}
	defer rows.Close()
	out := map[string][]BAItem{}
	for rows.Next() {
		var scheduleID, wetKey, archiveKind, uploadID string
		var it BAItem
		var status *string
		var reason, reviewer string
		var reviewedAt *time.Time
		if err := rows.Scan(&scheduleID, &it.Key, &wetKey, &archiveKind, &it.Date, &it.Source, &it.version, &it.uploaderID, &it.UploadedBy,
			&it.UploadedAt, &it.MimeType, &uploadID, &status, &reason, &reviewer, &reviewedAt); err != nil {
			return nil, fmt.Errorf("scan BA upload: %w", err)
		}
		if status != nil && reviewedAt != nil {
			it.Review = &MediaReview{Status: *status, Reason: reason, ReviewedBy: reviewer, ReviewedAt: *reviewedAt}
		}
		if wetKey == "ba-perorangan" {
			it.Type, it.Label = perorangan, "BA Perorangan"
			// program|regency|slot number: the recipient's page of the day's BA.
			parts := strings.Split(uploadID, "|")
			if len(parts) == 4 {
				ref := &IndividualRef{SlotID: parts[0], ProgramID: parts[1], RegencyID: parts[2], LocalDate: it.Date}
				_, _ = fmt.Sscanf(parts[3], "%d", &ref.SlotNumber)
				it.Individual = ref
				it.ContentURL = "/api/v1/bast/individual/bundles/preview"
			}
		} else {
			it.Type, it.Label = baTypeOf(wetKey, archiveKind)
			if it.Type == "" {
				continue
			}
			it.ContentURL = baContentURL(it.Source, wetKey, it.Type, scheduleID, it.Date, uploadID)
		}
		out[scheduleID] = append(out[scheduleID], it)
	}
	return out, rows.Err()
}

// baState is the state of a BA item: pending, approved or rejected.
func baState(r *MediaReview) string {
	if r == nil {
		return "pending"
	}
	return r.Status
}

// baContentURL is where the web reads an uploaded BA.
func baContentURL(source, wetKey, baType, scheduleID, day, uploadID string) string {
	if source == "archive" {
		return "/api/v1/bast/" + baType + "/uploads/" + uploadID + "/content"
	}
	q := url.Values{"schedule_id": {scheduleID}, "date": {day}}
	if location, ok := strings.CutPrefix(wetKey, "closing-titik-serah@"); ok {
		q.Set("document", "closing-titik-serah")
		q.Set("location_id", location)
	} else {
		q.Set("document", wetKey)
	}
	if source == "scan" {
		return "/api/v1/bast/full-scan/scan?" + q.Encode()
	}
	return "/api/v1/bast/wet-signature/scan?" + q.Encode()
}

// programSchedules lists the program's schedules in scope.
func (s *Service) programSchedules(ctx context.Context, programID string, scope auth.RegencyScope) ([]string, error) {
	var ids []string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(array_agg(id::text), '{}') FROM program_schedules
		WHERE program_id::text = $1 AND status <> 'cancelled' AND ($2 OR regency_id::text = ANY($3))`, strings.TrimSpace(programID), scope.Unrestricted, scope.RegencyIDs).Scan(&ids)
	if err != nil {
		return nil, fmt.Errorf("list program schedules: %w", err)
	}
	return ids, nil
}

// BAOverview counts the uploaded BA per kabupaten and type, BA Perorangan included.
func (s *Service) BAOverview(ctx context.Context, programID string, scope auth.RegencyScope) ([]BACell, error) {
	ids, err := s.programSchedules(ctx, programID, scope)
	if err != nil {
		return nil, err
	}
	items, err := s.baItems(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}
	out := []BACell{}
	for _, id := range ids {
		cells := map[string]*BACell{}
		var order []string
		for _, it := range items[id] {
			c, ok := cells[it.Type]
			if !ok {
				c = &BACell{ScheduleID: id, Type: it.Type}
				cells[it.Type] = c
				order = append(order, it.Type)
			}
			c.Items++
			switch {
			case it.Review == nil:
				c.Pending++
			case it.Review.Status == "approved":
				c.Approved++
			default:
				c.Rejected++
			}
		}
		for _, t := range order {
			out = append(out, *cells[t])
		}
	}
	return out, nil
}

// BAItems lists the uploaded BA of one type of a kabupaten.
func (s *Service) BAItems(ctx context.Context, scheduleID, baType string, scope auth.RegencyScope) ([]BAItem, error) {
	id, err := s.scheduleInScope(ctx, scheduleID, scope)
	if err != nil {
		return nil, err
	}
	items, err := s.baItems(ctx, s.pool, []string{id})
	if err != nil {
		return nil, err
	}
	out := []BAItem{}
	for _, it := range items[id] {
		if it.Type == baType {
			out = append(out, it)
		}
	}
	return out, nil
}

// SlotBA is the BA Perorangan of one recipient, or nil before it is final.
func (s *Service) SlotBA(ctx context.Context, scheduleID, slotID string) (*BAItem, error) {
	items, err := s.baItems(ctx, s.pool, []string{scheduleID})
	if err != nil {
		return nil, err
	}
	for _, it := range items[scheduleID] {
		if it.Key == "individual:"+slotID {
			return &it, nil
		}
	}
	return nil, nil
}

func (s *Service) scheduleInScope(ctx context.Context, scheduleID string, scope auth.RegencyScope) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM program_schedules WHERE id::text = $1 AND ($2 OR regency_id::text = ANY($3))`,
		strings.TrimSpace(scheduleID), scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrScheduleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find schedule: %w", err)
	}
	return id, nil
}

// reviewBA stores checks of the schedule's current BA items; the items must
// be among them.
func (s *Service) reviewBA(ctx context.Context, actor auth.Principal, scheduleID string, checks map[string]string, status string, after func(context.Context, pgx.Tx, []BAItem) error, meta auth.ClientMeta) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		items, err := s.baItems(ctx, tx, []string{scheduleID})
		if err != nil {
			return err
		}
		var checked []BAItem
		for key, reason := range checks {
			i := slices.IndexFunc(items[scheduleID], func(it BAItem) bool { return it.Key == key })
			if i < 0 {
				return ErrBAItemUnknown
			}
			it := items[scheduleID][i]
			if len(reason) > 300 {
				reason = reason[:300]
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ba_reviews (schedule_id, item_key, version, status, reason, reviewed_by, reviewed_at)
				VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6, '')::uuid, now())
				ON CONFLICT (schedule_id, item_key) DO UPDATE SET version = EXCLUDED.version, status = EXCLUDED.status, reason = EXCLUDED.reason,
					reviewed_by = EXCLUDED.reviewed_by, reviewed_at = now()`, scheduleID, key, it.version, status, reason, actor.UserID); err != nil {
				return fmt.Errorf("save BA review: %w", err)
			}
			checked = append(checked, it)
		}
		if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.ba_" + status, ResourceType: "program_schedule", ResourceID: scheduleID,
			Metadata: map[string]any{"items": checks}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
			return err
		}
		if after != nil {
			return after(ctx, tx, checked)
		}
		return nil
	})
}

// ApproveBA approves BA items of a kabupaten.
func (s *Service) ApproveBA(ctx context.Context, actor auth.Principal, input BAApproveInput, scope auth.RegencyScope, meta auth.ClientMeta) error {
	id, err := s.scheduleInScope(ctx, input.ScheduleID, scope)
	if err != nil {
		return err
	}
	checks := map[string]string{}
	for _, key := range input.Keys {
		if key = strings.TrimSpace(key); key != "" {
			checks[key] = ""
		}
	}
	if len(checks) == 0 {
		return ErrNothingToApprove
	}
	return s.reviewBA(ctx, actor, id, checks, "approved", nil, meta)
}

// RequestBARevision rejects BA items and tells who uploaded them and the
// kabupaten's leaders, with a link to the BA in the app.
func (s *Service) RequestBARevision(ctx context.Context, actor auth.Principal, input BARevisionInput, scope auth.RegencyScope, meta auth.ClientMeta) error {
	id, err := s.scheduleInScope(ctx, input.ScheduleID, scope)
	if err != nil {
		return err
	}
	checks := map[string]string{}
	for _, f := range input.Flags {
		if key := strings.TrimSpace(f.Key); key != "" {
			checks[key] = strings.TrimSpace(f.Reason)
		}
	}
	if len(checks) == 0 {
		return ErrRevisionEmpty
	}
	note := strings.TrimSpace(input.Note)
	err = s.reviewBA(ctx, actor, id, checks, "rejected", func(ctx context.Context, tx pgx.Tx, items []BAItem) error {
		var regencyID, regency string
		if err := tx.QueryRow(ctx, `SELECT r.id::text, r.name FROM program_schedules ps JOIN regencies r ON r.id = ps.regency_id WHERE ps.id::text = $1`, id).Scan(&regencyID, &regency); err != nil {
			return fmt.Errorf("find kabupaten: %w", err)
		}
		leaders, err := leadersOf(ctx, tx, id, regencyID)
		if err != nil {
			return err
		}
		for _, it := range items {
			users := slices.DeleteFunc(append([]string{it.uploaderID}, leaders...), func(u string) bool { return u == "" || u == actor.UserID })
			body := fmt.Sprintf("%s %s tanggal %s perlu diperbaiki", it.Label, regency, it.Date)
			if reason := strings.TrimSpace(strings.Join([]string{checks[it.Key], note}, "; ")); strings.Trim(reason, "; ") != "" {
				body += ": " + strings.Trim(reason, "; ")
			}
			if len(body) > 300 {
				body = body[:297] + "..."
			}
			link := fmt.Sprintf("/ba/%s/%s?schedule=%s&revisi=1", it.Type, it.Date, id)
			if it.Type == perorangan {
				slot := strings.TrimPrefix(it.Key, "individual:")
				var number int
				_ = tx.QueryRow(ctx, `SELECT slot_number FROM distribution_slots WHERE id::text = $1`, slot).Scan(&number)
				link = fmt.Sprintf("/nomor-bagi/%d?schedule=%s&revisi=1", number, id)
			}
			if err := notifications.Enqueue(ctx, tx, users, notifications.Message{
				Category: notifications.CategoryDistributionRevision, Title: "Revisi " + it.Label, Body: body, Link: link,
			}); err != nil {
				return err
			}
		}
		return nil
	}, meta)
	if err != nil {
		return err
	}
	if s.notifier != nil {
		s.notifier.Wake()
	}
	return nil
}
