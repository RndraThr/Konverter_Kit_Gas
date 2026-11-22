package monitoring

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/distribution"
	"konkit/internal/notifications"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Slots is the distribution side the monitoring needs: one slot with its
// photos, and reopening a slot with extra work in the same transaction.
type Slots interface {
	SearchSlot(ctx context.Context, scheduleID, query string, scope auth.RegencyScope) (distribution.DistributionSlot, error)
	ReopenSlotThen(ctx context.Context, actor auth.Principal, input distribution.ReopenSlotInput, meta auth.ClientMeta, scope auth.RegencyScope, then func(context.Context, pgx.Tx, string) error) (distribution.DistributionSlot, error)
}

// Notifier wakes the push worker after notifications were committed.
type Notifier interface{ Wake() }

type Service struct {
	pool     *pgxpool.Pool
	slots    Slots
	notifier Notifier
}

func NewService(pool *pgxpool.Pool, slots Slots, notifier Notifier) *Service {
	return &Service{pool: pool, slots: slots, notifier: notifier}
}

// photoCounts counts, per slot, the current photos and how many are approved
// or rejected (the rest is not checked yet).
const photoCounts = `
	SELECT dcs.distribution_slot_id AS slot_id, count(*) AS photos,
		count(*) FILTER (WHERE mr.status = 'approved') AS approved,
		count(*) FILTER (WHERE mr.status = 'rejected') AS rejected,
		max(mf.uploaded_at) AS last_upload
	FROM media_files mf
	JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
	LEFT JOIN media_reviews mr ON mr.media_id = mf.id
	WHERE mf.status = 'accepted'
	GROUP BY dcs.distribution_slot_id`

// userName is the display name of a user row aliased as the given letter.
func userName(alias string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%[1]s.full_name, ''), %[1]s.username, '')", alias)
}

// Overview: every kabupaten of the program (default: the newest active one)
// with its phase days, assignments and live numbers on date (default: today WIB).
func (s *Service) Overview(ctx context.Context, programID, date string, scope auth.RegencyScope) (Overview, error) {
	date, err := parseDate(date)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Date: date, Programs: []Program{}, Rakornas: []string{}, Rows: []ScheduleRow{}, Reviewers: []Person{}, Users: []Person{}}
	if out.Programs, programID, err = s.pickProgram(ctx, programID); err != nil || programID == "" {
		return out, err
	}
	out.ProgramID = programID

	rows, err := s.pool.Query(ctx, `
		WITH photos AS (`+photoCounts+`)
		SELECT ps.id::text, ps.name, r.id::text, r.name, r.province_name, COALESCE(z.name, ''), COALESCE(z.sort_order, 0),
			ps.start_date::text, ps.end_date::text, COALESCE(ps.slot_quota, 0), COALESCE(sr.sheet_no, 0),
			count(ds.id) FILTER (WHERE ds.status = 'completed'),
			count(ds.id) FILTER (WHERE ds.status = 'completed' AND (ds.completed_at AT TIME ZONE 'Asia/Jakarta')::date = $2::date),
			count(ds.id) FILTER (WHERE ds.status = 'completed' AND m.photos > 0 AND m.approved = m.photos),
			count(ds.id) FILTER (WHERE m.photos - m.approved - m.rejected > 0),
			count(ds.id) FILTER (WHERE m.rejected > 0 AND m.photos - m.approved - m.rejected = 0),
			COALESCE(sum(m.photos - m.approved - m.rejected), 0),
			max(GREATEST(ds.updated_at, m.last_upload))
		FROM program_schedules ps
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN program_regency_assignments pra ON pra.program_id = ps.program_id AND pra.regency_id = ps.regency_id
		LEFT JOIN program_zones z ON z.id = pra.zone_id
		LEFT JOIN schedule_sheet_rows sr ON sr.schedule_id = ps.id
		LEFT JOIN distribution_slots ds ON ds.schedule_id = ps.id
		LEFT JOIN photos m ON m.slot_id = ds.id
		WHERE ps.program_id = $1 AND ps.status <> 'cancelled' AND ($3 OR ps.regency_id::text = ANY($4))
		GROUP BY ps.id, r.id, z.name, z.sort_order, sr.sort_order, sr.sheet_no
		ORDER BY COALESCE(z.sort_order, 0), z.name, sr.sort_order NULLS LAST, r.province_name, r.name`, programID, date, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return out, fmt.Errorf("list monitoring schedules: %w", err)
	}
	index := map[string]int{}
	for rows.Next() {
		row := ScheduleRow{Days: []PhaseDay{}, Reviewers: []Person{}, Leaders: []Person{}}
		if err := rows.Scan(&row.ScheduleID, &row.Name, &row.RegencyID, &row.RegencyName, &row.ProvinceName, &row.ZoneName, &row.ZoneSort,
			&row.StartDate, &row.EndDate, &row.Quota, &row.SheetNo, &row.Completed, &row.CompletedToday, &row.Approved, &row.Pending, &row.Revision, &row.PendingPhotos, &row.LastActivityAt); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan monitoring schedule: %w", err)
		}
		index[row.ScheduleID] = len(out.Rows)
		out.Rows = append(out.Rows, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("list monitoring schedules: %w", err)
	}

	rows, err = s.pool.Query(ctx, `SELECT d.schedule_id::text, d.day::text, d.phase FROM schedule_phase_days d
		JOIN program_schedules ps ON ps.id = d.schedule_id WHERE ps.program_id = $1 ORDER BY d.day`, programID)
	if err != nil {
		return out, fmt.Errorf("list phase days: %w", err)
	}
	for rows.Next() {
		var id string
		var d PhaseDay
		if err := rows.Scan(&id, &d.Day, &d.Phase); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan phase day: %w", err)
		}
		if i, ok := index[id]; ok {
			out.Rows[i].Days = append(out.Rows[i].Days, d)
		}
	}
	rows.Close()
	for i := range out.Rows {
		out.Rows[i].TodayPhase, out.Rows[i].DistributionDay, out.Rows[i].DistributionOf = todayPhase(out.Rows[i].Days, date)
	}

	rows, err = s.pool.Query(ctx, `SELECT a.schedule_id::text, a.kind, u.id::text, `+userName("u")+` FROM monitoring_assignments a
		JOIN users u ON u.id = a.user_id JOIN program_schedules ps ON ps.id = a.schedule_id
		WHERE ps.program_id = $1 ORDER BY 4`, programID)
	if err != nil {
		return out, fmt.Errorf("list monitoring assignments: %w", err)
	}
	for rows.Next() {
		var id, kind string
		var p Person
		if err := rows.Scan(&id, &kind, &p.ID, &p.Name); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan monitoring assignment: %w", err)
		}
		if i, ok := index[id]; ok {
			if kind == KindReviewer {
				out.Rows[i].Reviewers = append(out.Rows[i].Reviewers, p)
			} else {
				out.Rows[i].Leaders = append(out.Rows[i].Leaders, p)
			}
		}
	}
	rows.Close()

	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(array_agg(day::text ORDER BY day), '{}') FROM program_calendar_days WHERE program_id = $1 AND kind = 'rakornas'`,
		programID).Scan(&out.Rakornas); err != nil {
		return out, fmt.Errorf("list rakornas days: %w", err)
	}
	if out.Reviewers, err = s.people(ctx, `SELECT DISTINCT u.id::text, `+userName("u")+` FROM users u
		JOIN user_roles ur ON ur.user_id = u.id JOIN roles ro ON ro.id = ur.role_id
		LEFT JOIN role_permissions rp ON rp.role_id = ro.id LEFT JOIN permissions p ON p.id = rp.permission_id
		WHERE u.is_active AND (ro.code = 'super_admin' OR p.code = 'monitoring.review') ORDER BY 2`); err != nil {
		return out, err
	}
	if out.Users, err = s.people(ctx, `SELECT u.id::text, `+userName("u")+` FROM users u WHERE u.is_active ORDER BY 2`); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) people(ctx context.Context, query string) ([]Person, error) {
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaveCalendar replaces the phase days of the given kabupaten (and the
// Rakornas days when sent). The schedule's own start and end dates stay as
// they are: DP3 and the final report print them.
func (s *Service) SaveCalendar(ctx context.Context, actor auth.Principal, input CalendarInput, scope auth.RegencyScope, meta auth.ClientMeta) error {
	programID := strings.TrimSpace(input.ProgramID)
	if input.Rakornas != nil && !scope.Unrestricted {
		return ErrRakornasForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save calendar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM programs WHERE id::text = $1)`, programID).Scan(&exists); err != nil {
		return fmt.Errorf("find program: %w", err)
	}
	if !exists {
		return ErrProgramNotFound
	}
	changed := []string{}
	for _, row := range input.Rows {
		days, err := cleanDays(row.Days)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `SELECT 1 FROM program_schedules WHERE id::text = $1 AND program_id::text = $2 AND ($3 OR regency_id::text = ANY($4))`,
			row.ScheduleID, programID, scope.Unrestricted, scope.RegencyIDs)
		if err != nil {
			return fmt.Errorf("find schedule: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrScheduleNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schedule_phase_days WHERE schedule_id = $1::uuid`, row.ScheduleID); err != nil {
			return fmt.Errorf("clear phase days: %w", err)
		}
		for _, d := range days {
			if _, err := tx.Exec(ctx, `INSERT INTO schedule_phase_days (schedule_id, day, phase) VALUES ($1::uuid, $2::date, $3)`, row.ScheduleID, d.Day, d.Phase); err != nil {
				return fmt.Errorf("save phase day: %w", err)
			}
		}
		changed = append(changed, row.ScheduleID)
	}
	if input.Rakornas != nil {
		dates, err := cleanDates(*input.Rakornas)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM program_calendar_days WHERE program_id = $1::uuid AND kind = 'rakornas'`, programID); err != nil {
			return fmt.Errorf("clear rakornas: %w", err)
		}
		for _, d := range dates {
			if _, err := tx.Exec(ctx, `INSERT INTO program_calendar_days (program_id, day, kind) VALUES ($1::uuid, $2::date, 'rakornas')`, programID, d); err != nil {
				return fmt.Errorf("save rakornas: %w", err)
			}
		}
		// Rakornas concerns every kabupaten: one signal per schedule refreshes the pages.
		if _, err := tx.Exec(ctx, `SELECT konkit_notify_schedule('phases', id) FROM program_schedules WHERE program_id = $1::uuid`, programID); err != nil {
			return fmt.Errorf("signal rakornas: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.calendar_saved", ResourceType: "program", ResourceID: programID,
		Metadata: map[string]any{"schedules": changed, "rakornas": input.Rakornas != nil}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SaveAssignments replaces who checks a kabupaten and who leads it in the field.
func (s *Service) SaveAssignments(ctx context.Context, actor auth.Principal, input AssignmentInput, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save assignments: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `SELECT 1 FROM program_schedules WHERE id::text = $1 AND ($2 OR regency_id::text = ANY($3))`,
		strings.TrimSpace(input.ScheduleID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return fmt.Errorf("find schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrScheduleNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM monitoring_assignments WHERE schedule_id = $1::uuid`, input.ScheduleID); err != nil {
		return fmt.Errorf("clear assignments: %w", err)
	}
	for kind, ids := range map[string][]string{KindReviewer: input.Reviewers, KindLeader: input.Leaders} {
		seen := map[string]bool{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			tag, err := tx.Exec(ctx, `INSERT INTO monitoring_assignments (schedule_id, user_id, kind)
				SELECT $1::uuid, u.id, $3 FROM users u WHERE u.id::text = $2 AND u.is_active ON CONFLICT DO NOTHING`, input.ScheduleID, id, kind)
			if err != nil {
				return fmt.Errorf("save assignment: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return ErrInvalidUser
			}
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.assignments_saved", ResourceType: "program_schedule", ResourceID: input.ScheduleID,
		Metadata: map[string]any{"reviewers": input.Reviewers, "leaders": input.Leaders}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Queue lists every recipient of a kabupaten that has photos, with how many
// are approved, rejected and not checked yet. Photos are checked as they come
// (POS Mesin a day ahead), so a recipient need not be completed.
func (s *Service) Queue(ctx context.Context, scheduleID string, scope auth.RegencyScope) ([]QueueItem, error) {
	rows, err := s.pool.Query(ctx, `
		WITH photos AS (`+photoCounts+`)
		SELECT ds.slot_number, COALESCE(p.full_name, ''), COALESCE(p.nik, ''), COALESCE(p.village, ''), COALESCE(ds.machine_serial_number, ''),
			COALESCE(ds.distribution_date::text, ''), ds.status, ds.completed_at, `+userName("cu")+`,
			m.photos, m.approved, m.rejected, m.last_upload, COALESCE(lr.name, ''), lr.reviewed_at
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN photos m ON m.slot_id = ds.id
		LEFT JOIN people p ON p.id = ds.recipient_person_id
		LEFT JOIN users cu ON cu.id = ds.distributed_by
		LEFT JOIN LATERAL (
			SELECT `+userName("ru")+` AS name, mr.reviewed_at FROM media_reviews mr
			JOIN media_files mf ON mf.id = mr.media_id AND mf.status = 'accepted'
			JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
			LEFT JOIN users ru ON ru.id = mr.reviewed_by
			WHERE dcs.distribution_slot_id = ds.id ORDER BY mr.reviewed_at DESC LIMIT 1
		) lr ON true
		WHERE ds.schedule_id::text = $1 AND ($2 OR ps.regency_id::text = ANY($3))
		ORDER BY ds.slot_number`, strings.TrimSpace(scheduleID), scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list check queue: %w", err)
	}
	defer rows.Close()
	out := []QueueItem{}
	for rows.Next() {
		var item QueueItem
		if err := rows.Scan(&item.SlotNumber, &item.FullName, &item.NIK, &item.Village, &item.MachineSerial, &item.DistributionDate, &item.Status,
			&item.CompletedAt, &item.CompletedBy, &item.Photos, &item.ApprovedPhotos, &item.RejectedPhotos, &item.LastUploadAt, &item.ReviewedBy, &item.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan check queue: %w", err)
		}
		item.PendingPhotos = item.Photos - item.ApprovedPhotos - item.RejectedPhotos
		item.State = reviewState(item.Photos, item.ApprovedPhotos, item.RejectedPhotos, item.Status == "completed")
		out = append(out, item)
	}
	return out, rows.Err()
}

// slotMeta is what the check needs about a slot beyond its photos.
type slotMeta struct {
	id, status, scheduleName, regencyName, regencyID, completedBy, completedByID string
	completedAt                                                                  *time.Time
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func slotMetaOf(ctx context.Context, q queryer, scheduleID string, slotNumber int, scope auth.RegencyScope, lock bool) (slotMeta, error) {
	var m slotMeta
	query := `
		SELECT ds.id::text, ds.status, ds.completed_at, ps.name, r.name, r.id::text, ` + userName("cu") + `, COALESCE(ds.distributed_by::text, '')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN regencies r ON r.id = ps.regency_id
		LEFT JOIN users cu ON cu.id = ds.distributed_by
		WHERE ds.schedule_id::text = $1 AND ds.slot_number = $2 AND ($3 OR ps.regency_id::text = ANY($4))`
	if lock {
		query += ` FOR UPDATE OF ds`
	}
	err := q.QueryRow(ctx, query, scheduleID, slotNumber, scope.Unrestricted, scope.RegencyIDs).
		Scan(&m.id, &m.status, &m.completedAt, &m.scheduleName, &m.regencyName, &m.regencyID, &m.completedBy, &m.completedByID)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, distribution.ErrSlotNotFound
	}
	if err != nil {
		return m, fmt.Errorf("get checked slot: %w", err)
	}
	return m, nil
}

// Slot is one recipient for the review dialog: photos, data and the check of each photo.
func (s *Service) Slot(ctx context.Context, scheduleID string, slotNumber int, scope auth.RegencyScope) (SlotDetail, error) {
	m, err := slotMetaOf(ctx, s.pool, scheduleID, slotNumber, scope, false)
	if err != nil {
		return SlotDetail{}, err
	}
	slot, err := s.slots.SearchSlot(ctx, scheduleID, strconv.Itoa(slotNumber), scope)
	if err != nil {
		return SlotDetail{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT mr.media_id::text, mr.status, mr.reason, `+userName("ru")+`, mr.reviewed_at
		FROM media_reviews mr
		JOIN media_files mf ON mf.id = mr.media_id AND mf.status = 'accepted'
		JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
		LEFT JOIN users ru ON ru.id = mr.reviewed_by
		WHERE dcs.distribution_slot_id = $1::uuid`, m.id)
	if err != nil {
		return SlotDetail{}, fmt.Errorf("list photo reviews: %w", err)
	}
	defer rows.Close()
	reviews := map[string]MediaReview{}
	for rows.Next() {
		var id string
		var r MediaReview
		if err := rows.Scan(&id, &r.Status, &r.Reason, &r.ReviewedBy, &r.ReviewedAt); err != nil {
			return SlotDetail{}, fmt.Errorf("scan photo review: %w", err)
		}
		reviews[id] = r
	}
	if err := rows.Err(); err != nil {
		return SlotDetail{}, fmt.Errorf("list photo reviews: %w", err)
	}
	photos, approved, rejected := 0, 0, 0
	for _, doc := range slot.Documentation {
		for _, f := range doc.Files {
			photos++
			switch reviews[f.ID].Status {
			case "approved":
				approved++
			case "rejected":
				rejected++
			}
		}
	}
	return SlotDetail{Slot: slot, ScheduleName: m.scheduleName, RegencyName: m.regencyName, CompletedAt: m.completedAt, CompletedBy: m.completedBy,
		State: reviewState(photos, approved, rejected, m.status == "completed"), Reviews: reviews}, nil
}

// currentMedia maps the slot's current photos to their check status ("" when not checked).
func currentMedia(ctx context.Context, q queryer, slotID string) (map[string]string, error) {
	rows, err := q.Query(ctx, `
		SELECT mf.id::text, COALESCE(mr.status, '')
		FROM media_files mf
		JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
		LEFT JOIN media_reviews mr ON mr.media_id = mf.id
		WHERE dcs.distribution_slot_id = $1::uuid AND mf.status = 'accepted'`, slotID)
	if err != nil {
		return nil, fmt.Errorf("list current photos: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, fmt.Errorf("scan current photo: %w", err)
		}
		out[id] = status
	}
	return out, rows.Err()
}

// Approve approves photos of a recipient: the ones given, or every photo not
// checked yet. Photos added later still need their own check.
func (s *Service) Approve(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, input ApproveInput, scope auth.RegencyScope, meta auth.ClientMeta) (SlotDetail, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SlotDetail{}, fmt.Errorf("begin approve: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	m, err := slotMetaOf(ctx, tx, scheduleID, slotNumber, scope, true)
	if err != nil {
		return SlotDetail{}, err
	}
	current, err := currentMedia(ctx, tx, m.id)
	if err != nil {
		return SlotDetail{}, err
	}
	ids := []string{}
	if len(input.MediaIDs) == 0 {
		for id, status := range current {
			if status == "" {
				ids = append(ids, id)
			}
		}
	} else {
		for _, id := range input.MediaIDs {
			id = strings.TrimSpace(id)
			if _, ok := current[id]; !ok {
				return SlotDetail{}, ErrFlagUnknown
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return SlotDetail{}, ErrNothingToApprove
	}
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `
			INSERT INTO media_reviews (media_id, status, reason, reviewed_by, reviewed_at) VALUES ($1::uuid, 'approved', '', NULLIF($2, '')::uuid, now())
			ON CONFLICT (media_id) DO UPDATE SET status = 'approved', reason = '', reviewed_by = EXCLUDED.reviewed_by, reviewed_at = now()`, id, actor.UserID); err != nil {
			return SlotDetail{}, fmt.Errorf("approve photo: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.photos_approved", ResourceType: "distribution_slot", ResourceID: m.id,
		Metadata: map[string]any{"schedule_id": scheduleID, "slot_number": slotNumber, "media_ids": ids}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SlotDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SlotDetail{}, fmt.Errorf("commit approve: %w", err)
	}
	return s.Slot(ctx, scheduleID, slotNumber, scope)
}

// RequestRevision rejects photos of a recipient and tells the field officers
// and the kabupaten's leaders. A completed recipient is reopened at the
// earliest stage among the rejected photos, so the photos can be replaced.
func (s *Service) RequestRevision(ctx context.Context, actor auth.Principal, scheduleID string, slotNumber int, input RevisionInput, scope auth.RegencyScope, meta auth.ClientMeta) (SlotDetail, error) {
	input.Note = strings.TrimSpace(input.Note)
	if len(input.Flags) == 0 {
		return SlotDetail{}, ErrRevisionEmpty
	}
	m, err := slotMetaOf(ctx, s.pool, scheduleID, slotNumber, scope, false)
	if err != nil {
		return SlotDetail{}, err
	}
	flags, uploaders, err := s.resolveFlags(ctx, m.id, input.Flags)
	if err != nil {
		return SlotDetail{}, err
	}
	reason := revisionReason(flags, input.Note)
	record := func(ctx context.Context, tx pgx.Tx) error {
		for _, f := range flags {
			if _, err := tx.Exec(ctx, `
				INSERT INTO media_reviews (media_id, status, reason, reviewed_by, reviewed_at) VALUES ($1::uuid, 'rejected', $2, NULLIF($3, '')::uuid, now())
				ON CONFLICT (media_id) DO UPDATE SET status = 'rejected', reason = EXCLUDED.reason, reviewed_by = EXCLUDED.reviewed_by, reviewed_at = now()`,
				f.MediaID, f.Reason, actor.UserID); err != nil {
				return fmt.Errorf("reject photo: %w", err)
			}
		}
		leaders, err := leadersOf(ctx, tx, scheduleID, m.regencyID)
		if err != nil {
			return err
		}
		users := slices.DeleteFunc(append(append([]string{m.completedByID}, uploaders...), leaders...), func(id string) bool { return id == "" || id == actor.UserID })
		body := fmt.Sprintf("Nomor bagi #%d %s: %s", slotNumber, m.regencyName, reason)
		if len(body) > 300 {
			body = body[:297] + "..."
		}
		if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "monitoring.photos_rejected", ResourceType: "distribution_slot", ResourceID: m.id,
			Metadata: map[string]any{"schedule_id": scheduleID, "slot_number": slotNumber, "flags": flags, "note": input.Note}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, users, notifications.Message{
			Category: notifications.CategoryDistributionRevision,
			Title:    fmt.Sprintf("Revisi dokumentasi #%d", slotNumber),
			Body:     body,
			Link:     fmt.Sprintf("/nomor-bagi/%d?schedule=%s&revisi=1", slotNumber, scheduleID),
		})
	}
	if m.status == "completed" {
		_, err = s.slots.ReopenSlotThen(ctx, actor, distribution.ReopenSlotInput{ScheduleID: scheduleID, SlotNumber: slotNumber, Stage: earliestStage(flags), Reason: reason}, meta, scope,
			func(ctx context.Context, tx pgx.Tx, _ string) error { return record(ctx, tx) })
	} else {
		err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return record(ctx, tx) })
	}
	if err != nil {
		return SlotDetail{}, err
	}
	if s.notifier != nil {
		s.notifier.Wake()
	}
	return s.Slot(ctx, scheduleID, slotNumber, scope)
}

// resolveFlags turns marked media into flags with their slot and stage, and
// lists who uploaded them. Every media must be a current photo of the slot.
func (s *Service) resolveFlags(ctx context.Context, slotID string, inputs []FlagInput) ([]distribution.RevisionFlag, []string, error) {
	flags := []distribution.RevisionFlag{}
	uploaders := []string{}
	for _, in := range inputs {
		var f distribution.RevisionFlag
		var uploader string
		err := s.pool.QueryRow(ctx, `
			SELECT mf.id::text, dcs.slot_code, dcs.label_snapshot, COALESCE(dcs.stage, ''), COALESCE(mf.uploaded_by::text, '')
			FROM media_files mf JOIN documentation_slots dcs ON dcs.id = mf.documentation_slot_id
			WHERE mf.id::text = $1 AND dcs.distribution_slot_id = $2::uuid AND mf.status = 'accepted'`, strings.TrimSpace(in.MediaID), slotID).
			Scan(&f.MediaID, &f.SlotCode, &f.Label, &f.Stage, &uploader)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrFlagUnknown
		}
		if err != nil {
			return nil, nil, fmt.Errorf("find marked photo: %w", err)
		}
		f.Reason = strings.TrimSpace(in.Reason)
		if len(f.Reason) > 300 {
			f.Reason = f.Reason[:300]
		}
		if !slices.ContainsFunc(flags, func(x distribution.RevisionFlag) bool { return x.MediaID == f.MediaID }) {
			flags = append(flags, f)
		}
		uploaders = append(uploaders, uploader)
	}
	return flags, uploaders, nil
}

// leadersOf: the field leaders assigned to the kabupaten, else the leaders of
// the finance teams that cover it.
func leadersOf(ctx context.Context, tx pgx.Tx, scheduleID, regencyID string) ([]string, error) {
	var ids []string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(a.user_id::text), '{}') FROM monitoring_assignments a JOIN users u ON u.id = a.user_id
		WHERE a.schedule_id::text = $1 AND a.kind = 'leader' AND u.is_active`, scheduleID).Scan(&ids); err != nil {
		return nil, fmt.Errorf("list field leaders: %w", err)
	}
	if len(ids) > 0 {
		return ids, nil
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT m.user_id::text), '{}') FROM finance_team_regencies tr
		JOIN finance_teams t ON t.id = tr.team_id AND t.is_active
		JOIN finance_team_members m ON m.team_id = tr.team_id AND m.member_role = 'leader'
		JOIN users u ON u.id = m.user_id AND u.is_active
		WHERE tr.regency_id::text = $1`, regencyID).Scan(&ids); err != nil {
		return nil, fmt.Errorf("list team leaders: %w", err)
	}
	return ids, nil
}
