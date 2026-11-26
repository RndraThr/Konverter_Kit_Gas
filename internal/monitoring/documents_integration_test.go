package monitoring

import (
	"context"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
)

// Supporting documents are checked like activity photos; each file is matched
// to the kabupaten's list by its name, and listed documents without a file
// are counted missing.
func TestSupportingDocumentReview(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	service := NewService(pool, distribution.NewRepository(pool), nil)
	_, err := pool.Exec(ctx, `INSERT INTO supporting_document_slots(regency_id,name,sort_order) VALUES($1,'DO Mesin',1),($1,'LO Gas, Tabung',2)`, f.regencyID)
	must(t, err)
	var doc string
	must(t, pool.QueryRow(ctx, `INSERT INTO activity_media(program_id,regency_id,activity_type,storage_key,display_name,original_filename,media_type,mime_type,byte_size,checksum,source,uploaded_by)
		VALUES($1,$2,'dokumen_pendukung',gen_random_uuid()::text,'MON-DO-MESIN-20261126-101010','do.pdf','document','application/pdf',1024,$3,'gallery',$4) RETURNING id::text`,
		f.programID, f.regencyID, strings.Repeat("c", 64), f.field).Scan(&doc))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activity_media WHERE regency_id = $1`, f.regencyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM supporting_document_slots WHERE regency_id = $1`, f.regencyID)
	})

	cells, err := service.ActivityOverview(ctx, f.programID, all)
	must(t, err)
	if len(cells) != 1 || cells[0].ActivityType != "dokumen_pendukung" || cells[0].Photos != 1 || cells[0].Pending != 1 || cells[0].Missing != 1 {
		t.Fatalf("cells = %+v", cells)
	}
	album, err := service.ActivityAlbum(ctx, ActivityRef{ScheduleID: f.scheduleID, ActivityType: "dokumen_pendukung"}, all)
	must(t, err)
	if len(album.Photos) != 1 || album.Photos[0].Slot != "DO Mesin" || len(album.Slots) != 2 || album.Slots[0].Files != 1 || album.Slots[1].Files != 0 {
		t.Fatalf("album = %+v", album)
	}
	if _, err := service.ApproveActivity(ctx, auth.Principal{UserID: f.admin}, ActivityApproveInput{ActivityRef: ActivityRef{ScheduleID: f.scheduleID, ActivityType: "dokumen_pendukung"}}, all, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if got := documentSegment("BKB-SURAT-KUASA-KAB-BANGKA-20261126-045955"); got != "SURAT-KUASA-KAB-BANGKA" {
		t.Fatalf("segment = %q", got)
	}
}
