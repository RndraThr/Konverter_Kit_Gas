package activities

import (
	"bytes"
	"context"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
	"konkit/internal/programs"
)

func TestTrainingPhotoWorkerCopiesDistributionTrainingPhotos(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	ctx := context.Background()
	var scheduleID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM program_schedules ORDER BY created_at LIMIT 1`).Scan(&scheduleID); err != nil {
		t.Skipf("no schedule seeded: %v", err)
	}
	storage, err := media.NewLocalStorage(t.TempDir())
	must(t, err)

	// 11 completed slots on one far-away date: 10% = the first 2 (ceil).
	base := int(time.Now().UnixNano()%1000000) + 2000000
	var slotIDs, mediaIDs []string
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM activity_media WHERE distribution_media_id = ANY($1::uuid[])`, mediaIDs)
		_, _ = pool.Exec(c, `DELETE FROM distribution_slots WHERE id = ANY($1::uuid[])`, slotIDs)
	})
	for i := 0; i < 11; i++ {
		var slotID, docID, mediaID, key string
		must(t, pool.QueryRow(ctx, `INSERT INTO distribution_slots(schedule_id,slot_number,status,distribution_date,distributed_at,completed_at) VALUES($1,$2,'completed','2099-01-05',now(),now()) RETURNING id::text`, scheduleID, base+i).Scan(&slotID))
		slotIDs = append(slotIDs, slotID)
		must(t, pool.QueryRow(ctx, `INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,status,sort_order) VALUES($1,'recipient_training','Foto Training','penyerahan',true,1,1,'both','complete',110) RETURNING id::text`, slotID).Scan(&docID))
		must(t, pool.QueryRow(ctx, `INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status) VALUES($1,gen_random_uuid(),'training.jpg','image/jpeg',4,repeat('a',64),'camera','accepted') RETURNING id::text, storage_key::text`, docID).Scan(&mediaID, &key))
		mediaIDs = append(mediaIDs, mediaID)
		_, _, _, err := storage.Put(ctx, key, nil, bytes.NewReader([]byte("jpeg")))
		must(t, err)
	}

	service := NewService(NewRepository(pool), storage, programs.NewService(programs.NewRepository(pool)))
	worker := NewTrainingPhotoWorker(service)
	run := func() {
		t.Helper()
		for {
			busy, err := worker.RunOnce(ctx)
			must(t, err)
			if !busy {
				return
			}
		}
	}
	count := func(activityType string) int {
		t.Helper()
		var n int
		must(t, pool.QueryRow(ctx, `SELECT count(*) FROM activity_media WHERE distribution_media_id = ANY($1::uuid[]) AND activity_type=$2 AND status='active' AND source='distribution'`, mediaIDs, activityType).Scan(&n))
		return n
	}

	run()
	if got, want := count("training_100"), 11; got != want {
		t.Fatalf("training_100 copies = %d, want %d", got, want)
	}
	if got, want := count("training_10"), 2; got != want {
		t.Fatalf("training_10 copies = %d, want %d", got, want)
	}
	var tenFirst string
	must(t, pool.QueryRow(ctx, `SELECT distribution_media_id::text FROM activity_media WHERE distribution_media_id = ANY($1::uuid[]) AND activity_type='training_10' AND status='active' ORDER BY display_name LIMIT 1`, mediaIDs).Scan(&tenFirst))
	if tenFirst != mediaIDs[0] {
		t.Fatalf("training_10 starts with %s, want the first slot's photo %s", tenFirst, mediaIDs[0])
	}

	// The first slot's photo is deleted: its copies go away.
	_, err = pool.Exec(ctx, `UPDATE media_files SET status='deleted' WHERE id=$1`, mediaIDs[0])
	must(t, err)
	run()
	if got := count("training_100"); got != 10 {
		t.Fatalf("training_100 after delete = %d, want 10", got)
	}
	if got := count("training_10"); got != 1 {
		t.Fatalf("training_10 after delete = %d, want 1", got)
	}

	// A copy the user deletes stays deleted.
	var copyID string
	must(t, pool.QueryRow(ctx, `SELECT id::text FROM activity_media WHERE distribution_media_id=$1 AND activity_type='training_100' AND status='active'`, mediaIDs[5]).Scan(&copyID))
	_, err = NewRepository(pool).SoftDelete(ctx, auth.Principal{}, copyID, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	must(t, err)
	run()
	if got := count("training_100"); got != 9 {
		t.Fatalf("training_100 after user delete = %d, want 9", got)
	}
}
