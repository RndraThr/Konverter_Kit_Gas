package monitoring

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/distribution"
	"konkit/internal/media"
)

// Rotating saves the turned file itself (storage and database copy) and gives
// a new URL; files of another schedule or of an unknown kind are refused.
func TestRotateSavesTheFile(t *testing.T) {
	pool := testPool(t)
	f := seed(t, pool)
	ctx := context.Background()
	storage, err := media.NewLocalStorage(t.TempDir())
	must(t, err)
	service := NewService(pool, distribution.NewRepository(pool), nil).WithStorage(storage)
	admin := auth.Principal{UserID: f.admin}

	photoID := addPhoto(t, pool, f, "ktp", f.uploader)
	var storageKey string
	must(t, pool.QueryRow(ctx, `SELECT storage_key::text FROM media_files WHERE id = $1`, photoID).Scan(&storageKey))
	var picture bytes.Buffer
	must(t, jpeg.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil))
	_, _, _, err = storage.Put(ctx, storageKey, nil, bytes.NewReader(picture.Bytes()))
	must(t, err)

	result, err := service.Rotate(ctx, admin, RotateInput{ScheduleID: f.scheduleID, Kind: "photo", ID: photoID, Degrees: 90}, all, auth.ClientMeta{})
	must(t, err)
	if result.ContentURL != "/api/v1/distribution/media/"+photoID+"/content?v=1" {
		t.Fatalf("url = %q", result.ContentURL)
	}
	reader, err := storage.Open(ctx, storageKey)
	must(t, err)
	stored, _ := io.ReadAll(reader)
	_ = reader.Close()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(stored))
	must(t, err)
	if cfg.Width != 20 || cfg.Height != 40 {
		t.Fatalf("stored photo %dx%d, want 20x40", cfg.Width, cfg.Height)
	}
	var size int64
	must(t, pool.QueryRow(ctx, `SELECT byte_size FROM media_files WHERE id = $1`, photoID).Scan(&size))
	if size != int64(len(stored)) {
		t.Fatalf("byte_size %d, stored %d", size, len(stored))
	}
	// The slot detail (web, app) now reads the new URL.
	var slotNumber int
	must(t, pool.QueryRow(ctx, `SELECT slot_number FROM distribution_slots WHERE id = $1`, f.slotID).Scan(&slotNumber))
	detail, err := service.Slot(ctx, f.scheduleID, slotNumber, all)
	must(t, err)
	found := false
	for _, doc := range detail.Slot.Documentation {
		for _, file := range doc.Files {
			found = found || (file.ID == photoID && strings.HasSuffix(file.ContentURL, "?v=1"))
		}
	}
	if !found {
		t.Fatal("slot detail still shows the old photo URL")
	}

	// A signature page kept in the database turns too.
	var page bytes.Buffer
	must(t, png.Encode(&page, image.NewRGBA(image.Rect(0, 0, 30, 10))))
	_, err = pool.Exec(ctx, `INSERT INTO bast_wet_signature_scans(schedule_id,document_key,document_date,content_code,image_png,byte_size,uploaded_by) VALUES($1,'dp3','2026-11-28','c1',$2,$3,$4)`,
		f.scheduleID, page.Bytes(), page.Len(), f.uploader)
	must(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bast_wet_signature_scans WHERE schedule_id = $1`, f.scheduleID)
	})
	ba, err := service.Rotate(ctx, admin, RotateInput{ScheduleID: f.scheduleID, Kind: "ba", ID: "wet:dp3:2026-11-28", Degrees: -90}, all, auth.ClientMeta{})
	must(t, err)
	if !strings.Contains(ba.ContentURL, "v=1") {
		t.Fatalf("BA url = %q", ba.ContentURL)
	}
	var turned []byte
	must(t, pool.QueryRow(ctx, `SELECT image_png FROM bast_wet_signature_scans WHERE schedule_id = $1`, f.scheduleID).Scan(&turned))
	if c, err := png.DecodeConfig(bytes.NewReader(turned)); err != nil || c.Width != 10 || c.Height != 30 {
		t.Fatalf("signature page %+v, %v", c, err)
	}

	if _, err := service.Rotate(ctx, admin, RotateInput{ScheduleID: f.scheduleID, Kind: "activity", ID: photoID, Degrees: 90}, all, auth.ClientMeta{}); !errors.Is(err, ErrRotateTarget) {
		t.Fatalf("photo as activity: %v", err)
	}
	if _, err := service.Rotate(ctx, admin, RotateInput{ScheduleID: f.scheduleID, Kind: "photo", ID: photoID, Degrees: 45}, all, auth.ClientMeta{}); !errors.Is(err, media.ErrRotateDegrees) {
		t.Fatalf("45 degrees: %v", err)
	}
}
