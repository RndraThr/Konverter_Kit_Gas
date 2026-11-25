package monitoring

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

// Activity photos are counted per kabupaten and type, checked per photo, and
// a revision tells the uploader and the leaders, who see the rejected photos
// until they replace them. Copies of distribution photos are not checked again.
func TestActivityPhotoReview(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	admin := auth.Principal{UserID: f.admin}
	must(t, service.SaveAssignments(ctx, admin, AssignmentInput{ScheduleID: f.scheduleID, Leaders: []string{f.leader}}, all, auth.ClientMeta{}))

	photo := func(activityType, source, uploader string) string {
		var id string
		must(t, pool.QueryRow(ctx, `INSERT INTO activity_media(program_id,regency_id,activity_type,storage_key,display_name,original_filename,media_type,mime_type,byte_size,checksum,source,uploaded_by)
			VALUES($1,$2,$3,gen_random_uuid()::text,'foto','foto.jpg','image','image/jpeg',1024,$4,$5,$6) RETURNING id::text`,
			f.programID, f.regencyID, activityType, strings.Repeat("b", 64), source, uploader).Scan(&id))
		return id
	}
	rakor1, rakor2 := photo("rakor", "camera", f.uploader), photo("rakor", "gallery", f.field)
	copied := photo("training_100", "distribution", f.field)
	photo("dokumen_pendukung", "camera", f.field)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activity_media WHERE regency_id = $1`, f.regencyID)
	})

	cells, err := service.ActivityOverview(ctx, f.programID, all)
	must(t, err)
	byType := map[string]ActivityCell{}
	for _, c := range cells {
		byType[c.ActivityType] = c
	}
	if len(cells) != 2 || byType["rakor"].Photos != 2 || byType["rakor"].Pending != 2 || byType["training_100"].Approved != 1 || byType["training_100"].Pending != 0 {
		t.Fatalf("cells = %+v", cells)
	}

	ref := ActivityRef{ScheduleID: f.scheduleID, ActivityType: "rakor"}
	if _, err := service.ApproveActivity(ctx, admin, ActivityApproveInput{ActivityRef: ActivityRef{ScheduleID: f.scheduleID, ActivityType: "training_100"}, MediaIDs: []string{copied}}, all, auth.ClientMeta{}); !errors.Is(err, ErrActivityFlagUnknown) {
		t.Fatalf("approve copied photo: %v", err)
	}
	album, err := service.ApproveActivity(ctx, admin, ActivityApproveInput{ActivityRef: ref, MediaIDs: []string{rakor1}}, all, auth.ClientMeta{})
	must(t, err)
	if len(album.Photos) != 2 || album.Photos[0].Review == nil || album.Photos[0].Review.Status != "approved" || album.Photos[1].Review != nil {
		t.Fatalf("album after approve = %+v", album.Photos)
	}

	album, err = service.RequestActivityRevision(ctx, admin, ActivityRevisionInput{ActivityRef: ref, Flags: []FlagInput{{MediaID: rakor2, Reason: "Bukan foto rakor"}}}, all, auth.ClientMeta{})
	must(t, err)
	if album.Photos[1].Review == nil || album.Photos[1].Review.Status != "rejected" || album.Photos[1].Review.Reason != "Bukan foto rakor" {
		t.Fatalf("album after revision = %+v", album.Photos)
	}
	var notified []string
	var link string
	must(t, pool.QueryRow(ctx, `SELECT array_agg(user_id::text ORDER BY user_id), min(link) FROM notifications WHERE user_id = ANY($1::uuid[])`,
		[]string{f.field, f.uploader, f.leader, f.outsider, f.admin}).Scan(&notified, &link))
	if !slices.Contains(notified, f.field) || !slices.Contains(notified, f.leader) || slices.Contains(notified, f.outsider) || slices.Contains(notified, f.admin) {
		t.Fatalf("notified = %v (field %s leader %s)", notified, f.field, f.leader)
	}
	if link != "/kegiatan/rakor?schedule="+f.scheduleID+"&revisi=1" {
		t.Fatalf("link = %q", link)
	}

	own := auth.RegencyScope{RegencyIDs: []string{f.regencyID}}
	rejections, err := service.ActivityRejections(ctx, f.scheduleID, own)
	must(t, err)
	if len(rejections) != 1 || rejections[0].MediaID != rakor2 || rejections[0].ActivityType != "rakor" {
		t.Fatalf("rejections = %+v", rejections)
	}
	// Replaced (deleted) in the app: nothing left to show, the replacement is new.
	_, err = pool.Exec(ctx, `UPDATE activity_media SET status = 'deleted' WHERE id = $1`, rakor2)
	must(t, err)
	photo("rakor", "camera", f.field)
	if rejections, err = service.ActivityRejections(ctx, f.scheduleID, own); err != nil || len(rejections) != 0 {
		t.Fatalf("rejections after replace = %+v %v", rejections, err)
	}
	if cells, _ = service.ActivityOverview(ctx, f.programID, all); !slices.ContainsFunc(cells, func(c ActivityCell) bool {
		return c.ActivityType == "rakor" && c.Photos == 2 && c.Approved == 1 && c.Pending == 1 && c.Rejected == 0
	}) {
		t.Fatalf("cells after replace = %+v", cells)
	}
	if _, err := service.ActivityRejections(ctx, f.scheduleID, auth.RegencyScope{RegencyIDs: []string{"00000000-0000-0000-0000-000000000000"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActivityAlbum(ctx, ActivityRef{ScheduleID: f.scheduleID, ActivityType: "dokumen_pendukung"}, all); !errors.Is(err, ErrActivityAlbumNotFound) {
		t.Fatalf("supporting documents album: %v", err)
	}
}
