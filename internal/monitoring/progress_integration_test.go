package monitoring

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProgressNotifiesWatchingPhonesOnly(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, nil, nil)
	suffix := fmt.Sprint(time.Now().UnixNano())

	// admin watches the kabupaten from one phone; outsider (also allowed to
	// monitor) has a phone that chose no kabupaten.
	for _, id := range []string{f.admin, f.outsider} {
		_, err := pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1, id FROM roles WHERE code = 'super_admin'`, id)
		must(t, err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO device_tokens(token,user_id,regency_ids) VALUES($1,$2,ARRAY[$3::uuid]), ($4,$5,NULL)`,
		"watch-"+suffix, f.admin, f.regencyID, "none-"+suffix, f.outsider)
	must(t, err)
	_, err = pool.Exec(ctx, `UPDATE program_schedules SET slot_quota = 2 WHERE id = $1`, f.scheduleID)
	must(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM device_tokens WHERE token = ANY($1)`, []string{"watch-" + suffix, "none-" + suffix})
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_roles WHERE user_id = ANY($1::uuid[])`, []string{f.admin, f.outsider})
	})

	titles := func(user string) []string {
		rows, err := pool.Query(ctx, `SELECT title FROM notifications WHERE user_id = $1 AND category = 'distribution_progress' ORDER BY created_at, title`, user)
		must(t, err)
		defer rows.Close()
		out := []string{}
		for rows.Next() {
			var s string
			must(t, rows.Scan(&s))
			out = append(out, s)
		}
		return out
	}
	at := func(clock string) time.Time {
		v, err := time.Parse(time.RFC3339, "2026-11-20T"+clock+"+07:00")
		must(t, err)
		return v
	}

	// The first look only records where the kabupaten stands (1 of 2 = 50%).
	must(t, service.SendProgress(ctx, at("10:05:00")))
	if got := titles(f.admin); len(got) != 0 {
		t.Fatalf("first look sent %v", got)
	}

	// Pretend the 50% and one completion are still to be told.
	_, err = pool.Exec(ctx, `UPDATE distribution_progress_notices SET live_completed = 0, half_sent = false WHERE schedule_id = $1`, f.scheduleID)
	must(t, err)
	must(t, service.SendProgress(ctx, at("11:00:00")))
	got := titles(f.admin)
	has := func(list []string, suffix string) bool {
		for _, v := range list {
			if strings.HasSuffix(v, suffix) {
				return true
			}
		}
		return false
	}
	if len(got) != 2 || !has(got, ": 1 selesai hari ini") || !has(got, " sudah 50%") {
		t.Fatalf("want a live and a 50%% notice, got %v", got)
	}
	live := func() (count int, tag string, silent bool) {
		must(t, pool.QueryRow(ctx, `SELECT count(*), max(push_tag), bool_or(push_silent) FROM notifications
			WHERE user_id = $1 AND push_tag <> ''`, f.admin).Scan(&count, &tag, &silent))
		return
	}
	if count, tag, silent := live(); count != 1 || tag != "progress-"+f.scheduleID || silent {
		t.Fatalf("first live update count=%d tag=%q silent=%v (the day's first rings)", count, tag, silent)
	}
	// Nothing new: no update.
	must(t, service.SendProgress(ctx, at("11:00:15")))
	if again := titles(f.admin); len(again) != 2 {
		t.Fatalf("no new completion still sent %v", again)
	}
	// A later completion replaces the line, silently.
	_, err = pool.Exec(ctx, `UPDATE distribution_progress_notices SET live_completed = 0 WHERE schedule_id = $1`, f.scheduleID)
	must(t, err)
	must(t, service.SendProgress(ctx, at("11:30:00")))
	if count, _, silent := live(); count != 1 || !silent {
		t.Fatalf("later update count=%d silent=%v, want one silent line", count, silent)
	}

	// Evening: the recap of the day, once.
	must(t, service.SendProgress(ctx, at("20:30:00")))
	must(t, service.SendProgress(ctx, at("20:40:00")))
	got = titles(f.admin)
	recaps := 0
	for _, v := range got {
		if strings.HasPrefix(v, "Rekap hari ini") {
			recaps++
		}
	}
	if len(got) != 3 || recaps != 1 {
		t.Fatalf("after recap titles = %v", got)
	}
	if other := titles(f.outsider); len(other) != 0 {
		t.Fatalf("phone without kabupaten got %v", other)
	}
	var regency string
	must(t, pool.QueryRow(ctx, `SELECT COALESCE(regency_id::text,'') FROM notifications WHERE user_id = $1 LIMIT 1`, f.admin).Scan(&regency))
	if regency != f.regencyID {
		t.Fatalf("notification regency = %q", regency)
	}
}
