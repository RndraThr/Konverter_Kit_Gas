package monitoring

import (
	"context"
	"errors"
	"slices"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

// A BA shows up once its signed version is uploaded, is checked per item, a
// revision tells the uploader and the leaders, and a new scan of the same BA
// is not checked yet. A signature-page scan is hidden by the full scan of the
// same BA and date.
func TestBAReview(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	admin := auth.Principal{UserID: f.admin}
	must(t, service.SaveAssignments(ctx, admin, AssignmentInput{ScheduleID: f.scheduleID, Leaders: []string{f.leader}}, all, auth.ClientMeta{}))
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		must(t, err)
	}
	exec(`INSERT INTO bast_full_scans(schedule_id,document_key,document_date,content_code,page_count,pdf,byte_size,uploaded_by) VALUES($1,'dp3','2026-11-28','c1',2,'%PDF',4,$2)`, f.scheduleID, f.uploader)
	exec(`INSERT INTO bast_wet_signature_scans(schedule_id,document_key,document_date,content_code,image_png,byte_size,uploaded_by) VALUES($1,'dp3','2026-11-28','c1','png',3,$2)`, f.scheduleID, f.uploader)
	exec(`INSERT INTO bast_wet_signature_scans(schedule_id,document_key,document_date,content_code,image_png,byte_size,uploaded_by) VALUES($1,'ba-pemeriksaan','2026-11-27','c2','png',3,$2)`, f.scheduleID, f.field)
	exec(`INSERT INTO bast_rakorda_uploads(schedule_id,document_kind,event_date,original_name,mime_type,byte_size,storage_key,uploaded_by) VALUES($1,'rakorda','2026-11-26','hadir.jpg','image/jpeg',10,'k',$2)`, f.scheduleID, f.field)
	t.Cleanup(func() {
		c := context.Background()
		for _, table := range []string{"bast_full_scans", "bast_wet_signature_scans", "bast_rakorda_uploads", "ba_reviews"} {
			_, _ = pool.Exec(c, `DELETE FROM `+table+` WHERE schedule_id = $1`, f.scheduleID)
		}
	})

	cells, err := service.BAOverview(ctx, f.programID, all)
	must(t, err)
	byType := map[string]BACell{}
	for _, c := range cells {
		byType[c.Type] = c
	}
	if len(cells) != 3 || byType["dp3"].Items != 1 || byType["pemeriksaan"].Pending != 1 || byType["rakorda"].Items != 1 {
		t.Fatalf("cells = %+v", cells)
	}
	dp3, err := service.BAItems(ctx, f.scheduleID, "dp3", all)
	must(t, err)
	if len(dp3) != 1 || dp3[0].Source != "scan" || dp3[0].ContentURL == "" || dp3[0].Review != nil {
		t.Fatalf("dp3 = %+v", dp3)
	}
	rakorda, _ := service.BAItems(ctx, f.scheduleID, "rakorda", all)

	if err := service.ApproveBA(ctx, admin, BAApproveInput{ScheduleID: f.scheduleID, Keys: []string{"scan:nope"}}, all, auth.ClientMeta{}); !errors.Is(err, ErrBAItemUnknown) {
		t.Fatalf("unknown item: %v", err)
	}
	must(t, service.ApproveBA(ctx, admin, BAApproveInput{ScheduleID: f.scheduleID, Keys: []string{dp3[0].Key}}, all, auth.ClientMeta{}))
	revision := BARevisionInput{ScheduleID: f.scheduleID}
	revision.Flags = append(revision.Flags, struct {
		Key    string `json:"key"`
		Reason string `json:"reason"`
	}{rakorda[0].Key, "Daftar hadir belum ditandatangani"})
	must(t, service.RequestBARevision(ctx, admin, revision, all, auth.ClientMeta{}))

	var notified []string
	var link string
	must(t, pool.QueryRow(ctx, `SELECT array_agg(user_id::text), min(link) FROM notifications WHERE user_id = ANY($1::uuid[]) AND title LIKE 'Revisi %'`,
		[]string{f.field, f.leader, f.outsider, f.admin}).Scan(&notified, &link))
	if !slices.Contains(notified, f.field) || !slices.Contains(notified, f.leader) || slices.Contains(notified, f.outsider) {
		t.Fatalf("notified = %v", notified)
	}
	if link != "/ba/rakorda/2026-11-26?schedule="+f.scheduleID+"&revisi=1" {
		t.Fatalf("link = %q", link)
	}
	overview, err := service.Overview(ctx, f.programID, "2026-11-28", all)
	must(t, err)
	if overview.Rows[0].BAPending != 1 || overview.Rows[0].BARevision != 1 {
		t.Fatalf("row BA = %d pending, %d revision", overview.Rows[0].BAPending, overview.Rows[0].BARevision)
	}

	// A new scan of the approved DP3 needs a new check.
	exec(`UPDATE bast_full_scans SET content_code = 'c3', uploaded_at = now() + interval '1 second' WHERE schedule_id = $1`, f.scheduleID)
	if dp3, _ = service.BAItems(ctx, f.scheduleID, "dp3", all); len(dp3) != 1 || dp3[0].Review != nil {
		t.Fatalf("dp3 after a new scan = %+v", dp3)
	}
	if _, err := service.BAItems(ctx, f.scheduleID, "dp3", auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("out of scope: %v", err)
	}
}
