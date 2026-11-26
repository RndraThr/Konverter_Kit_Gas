package monitoring

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"konkit/internal/notifications"

	"github.com/jackc/pgx/v5"
)

// Progress notifications tell the phones that watch a kabupaten (Mode Pusat)
// how its distribution goes: an hourly digest while recipients are completed,
// once at 50% and 100% of the quota, and a recap of the day in the evening.
const (
	progressInterval  = 5 * time.Minute
	progressFirstHour = 7
	progressLastHour  = 21
	progressRecapHour = 20
)

// RunProgress sends progress notifications until ctx ends.
func (s *Service) RunProgress(ctx context.Context) {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	for {
		if err := s.SendProgress(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Printf("monitoring: progress notifications: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type progressState struct {
	hourlyAt           time.Time
	hourlyCompleted    int
	halfSent, fullSent bool
	dailyOn            *time.Time
}

// SendProgress sends what is due at now for every kabupaten with recipients
// completed today. The first look at a kabupaten only records where it stands.
func (s *Service) SendProgress(ctx context.Context, now time.Time) error {
	local := now.In(jakarta)
	today := local.Format(dateLayout)
	rows, err := s.pool.Query(ctx, `
		SELECT ps.id::text, r.id::text, r.name, COALESCE(ps.slot_quota, 0),
			count(*) FILTER (WHERE ds.status = 'completed'),
			count(*) FILTER (WHERE ds.status = 'completed' AND (ds.completed_at AT TIME ZONE 'Asia/Jakarta')::date = $1::date)
		FROM program_schedules ps
		JOIN regencies r ON r.id = ps.regency_id
		JOIN distribution_slots ds ON ds.schedule_id = ps.id
		WHERE ps.status <> 'cancelled'
		GROUP BY ps.id, r.id
		HAVING count(*) FILTER (WHERE ds.status = 'completed' AND (ds.completed_at AT TIME ZONE 'Asia/Jakarta')::date = $1::date) > 0`, today)
	if err != nil {
		return fmt.Errorf("list distribution progress: %w", err)
	}
	type schedule struct {
		id, regencyID, regency  string
		quota, completed, today int
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (schedule, error) {
		var v schedule
		return v, row.Scan(&v.id, &v.regencyID, &v.regency, &v.quota, &v.completed, &v.today)
	})
	if err != nil {
		return fmt.Errorf("list distribution progress: %w", err)
	}
	sent := false
	for _, v := range list {
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			var st progressState
			err := tx.QueryRow(ctx, `SELECT hourly_at, hourly_completed, half_sent, full_sent, daily_on FROM distribution_progress_notices
				WHERE schedule_id = $1::uuid FOR UPDATE`, v.id).Scan(&st.hourlyAt, &st.hourlyCompleted, &st.halfSent, &st.fullSent, &st.dailyOn)
			if errors.Is(err, pgx.ErrNoRows) {
				_, err = tx.Exec(ctx, `INSERT INTO distribution_progress_notices (schedule_id, hourly_at, hourly_completed, half_sent, full_sent)
					VALUES ($1::uuid, $2, $3, $4, $5) ON CONFLICT (schedule_id) DO NOTHING`,
					v.id, now, v.completed, v.quota > 0 && v.completed*2 >= v.quota, v.quota > 0 && v.completed >= v.quota)
				return err
			}
			if err != nil {
				return fmt.Errorf("read progress notices: %w", err)
			}
			name := shortRegencyName(v.regency)
			totals := fmt.Sprintf("Total %s/%s (%d%%)", thousands(v.completed), thousands(v.quota), percent(v.completed, v.quota))
			if v.quota == 0 {
				totals = "Total " + thousands(v.completed)
			}
			var messages []notifications.Message
			add := func(title, body string) {
				messages = append(messages, notifications.Message{Category: notifications.CategoryDistributionProgress, Title: title, Body: body,
					Link: "/monitoring-distribusi?schedule=" + v.id, RegencyID: v.regencyID})
			}
			switch {
			case v.quota > 0 && !st.fullSent && v.completed >= v.quota:
				add(name+" selesai 100%", fmt.Sprintf("Semua %s penerima sudah selesai distribusi.", thousands(v.quota)))
				st.fullSent, st.halfSent = true, true
			case v.quota > 0 && !st.halfSent && v.completed*2 >= v.quota:
				add(name+" sudah 50%", fmt.Sprintf("%s dari %s penerima sudah selesai.", thousands(v.completed), thousands(v.quota)))
				st.halfSent = true
			}
			hour := local.Hour()
			if now.Sub(st.hourlyAt) >= time.Hour && v.completed > st.hourlyCompleted && hour >= progressFirstHour && hour <= progressLastHour {
				since := "1 jam terakhir"
				if now.Sub(st.hourlyAt) > 90*time.Minute {
					since = "Sejak " + st.hourlyAt.In(jakarta).Format("15.04")
				}
				add(fmt.Sprintf("%s +%s selesai", name, thousands(v.completed-st.hourlyCompleted)),
					fmt.Sprintf("%s. %s, hari ini %s.", since, totals, thousands(v.today)))
				st.hourlyAt, st.hourlyCompleted = now, v.completed
			}
			if hour >= progressRecapHour && (st.dailyOn == nil || st.dailyOn.Format(dateLayout) != today) {
				add("Rekap hari ini "+name, fmt.Sprintf("%s penerima selesai hari ini. %s.", thousands(v.today), totals))
				day, _ := time.Parse(dateLayout, today)
				st.dailyOn = &day
			}
			if len(messages) > 0 {
				users, err := watchers(ctx, tx, v.regencyID)
				if err != nil {
					return err
				}
				for _, m := range messages {
					if err := notifications.Enqueue(ctx, tx, users, m); err != nil {
						return err
					}
				}
				sent = sent || len(users) > 0
			}
			_, err = tx.Exec(ctx, `UPDATE distribution_progress_notices SET hourly_at = $2, hourly_completed = $3, half_sent = $4, full_sent = $5, daily_on = $6
				WHERE schedule_id = $1::uuid`, v.id, st.hourlyAt, st.hourlyCompleted, st.halfSent, st.fullSent, st.dailyOn)
			return err
		})
		if err != nil {
			return fmt.Errorf("progress of %s: %w", v.regency, err)
		}
	}
	if sent && s.notifier != nil {
		s.notifier.Wake()
	}
	return nil
}

// watchers: active monitoring users with a phone that watches the kabupaten.
func watchers(ctx context.Context, tx pgx.Tx, regencyID string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT u.id::text FROM device_tokens d JOIN users u ON u.id = d.user_id
		WHERE u.is_active AND $1::uuid = ANY(d.regency_ids) AND EXISTS (
			SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id
			LEFT JOIN role_permissions rp ON rp.role_id = ro.id LEFT JOIN permissions p ON p.id = rp.permission_id
			WHERE ur.user_id = u.id AND (ro.code = 'super_admin' OR p.code = 'monitoring.view'))`, regencyID)
	if err != nil {
		return nil, fmt.Errorf("list kabupaten watchers: %w", err)
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list kabupaten watchers: %w", err)
	}
	return users, nil
}

func shortRegencyName(name string) string {
	name = strings.TrimSpace(name)
	for _, prefix := range []string{"KAB. ", "KABUPATEN "} {
		if strings.HasPrefix(strings.ToUpper(name), prefix) {
			return strings.TrimSpace(name[len(prefix):])
		}
	}
	return name
}

func percent(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	return part * 100 / whole
}

// thousands writes n with dots between thousands, as Indonesian does.
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}
