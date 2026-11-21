package slotexamples

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"
	"konkit/internal/media"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationExamplesSaveReplaceAndDelete(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	storage, err := media.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, storage)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1000000000)
	code := "example_test_" + suffix
	templateCode := "EXAMPLE-TEST-" + suffix
	if _, err := pool.Exec(ctx, `
		WITH v AS (INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Uji contoh','farmer','published') RETURNING id)
		INSERT INTO documentation_template_slots(template_version_id,slot_code,label,stage,sort_order) SELECT id,$2,'Foto Uji Contoh','dokumen',10 FROM v`, templateCode, code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_slot_examples WHERE slot_code=$1`, code)
		_, _ = pool.Exec(context.Background(), `DELETE FROM documentation_template_versions WHERE template_code=$1`, templateCode)
	})
	actor := auth.Principal{}
	meta := auth.ClientMeta{UserAgent: "slotexamples-test"}

	slot := findSlot(t, service, code)
	if slot.Label != "Foto Uji Contoh" || slot.Stage != "dokumen" || slot.Example != nil {
		t.Fatalf("slot before example=%+v", slot)
	}

	// A first example needs a photo, and only an image is accepted.
	if _, err := service.Save(ctx, actor, SaveInput{SlotCode: code, Caption: "Dari depan"}, meta); !errors.Is(err, ErrImageRequired) {
		t.Fatalf("save without photo err=%v", err)
	}
	if _, err := service.Save(ctx, actor, SaveInput{SlotCode: code, Image: []byte("bukan gambar")}, meta); !errors.Is(err, ErrImageInvalid) {
		t.Fatalf("save with text err=%v", err)
	}
	if _, err := service.Save(ctx, actor, SaveInput{SlotCode: "Bukan Kode", Image: pngBytes(t)}, meta); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("save with bad code err=%v", err)
	}

	first, err := service.Save(ctx, actor, SaveInput{SlotCode: code, Caption: "  KTP dan nomor urut terlihat jelas  ", Image: pngBytes(t), OriginalFilename: "ktp.png"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if first.Caption != "KTP dan nomor urut terlihat jelas" || !strings.HasPrefix(first.ImageURL, "/api/v1/documentation-examples/"+code+"/image?v=") {
		t.Fatalf("first example=%+v", first)
	}
	firstKey := storedKey(t, pool, code)
	content, err := service.Open(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(content.Reader)
	content.Reader.Close()
	if content.MimeType != "image/png" || content.Filename != "ktp.png" || !bytes.Equal(data, pngBytes(t)) {
		t.Fatalf("content mime=%s name=%s bytes=%d", content.MimeType, content.Filename, len(data))
	}

	// Changing only the caption keeps the photo.
	if _, err := service.Save(ctx, actor, SaveInput{SlotCode: code, Caption: "Ambil dari depan"}, meta); err != nil {
		t.Fatal(err)
	}
	if key := storedKey(t, pool, code); key != firstKey {
		t.Fatalf("caption update replaced the photo: %s -> %s", firstKey, key)
	}
	if slot := findSlot(t, service, code); slot.Example == nil || slot.Example.Caption != "Ambil dari depan" {
		t.Fatalf("slot after caption update=%+v", slot)
	}

	// A new photo replaces the stored file.
	if _, err := service.Save(ctx, actor, SaveInput{SlotCode: code, Caption: "Ambil dari depan", Image: pngBytes(t), OriginalFilename: "ktp-baru.png"}, meta); err != nil {
		t.Fatal(err)
	}
	if key := storedKey(t, pool, code); key == firstKey {
		t.Fatal("new photo kept the old storage key")
	}
	if reader, err := storage.Open(ctx, firstKey); err == nil {
		reader.Close()
		t.Fatal("old photo was not removed from storage")
	}

	if err := service.Delete(ctx, actor, code, meta); err != nil {
		t.Fatal(err)
	}
	if slot := findSlot(t, service, code); slot.Example != nil {
		t.Fatalf("slot after delete=%+v", slot)
	}
	if err := service.Delete(ctx, actor, code, meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err=%v", err)
	}
	if _, err := service.Open(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open after delete err=%v", err)
	}
}

func findSlot(t *testing.T, service *Service, code string) Slot {
	t.Helper()
	slots, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range slots {
		if slot.Code == code {
			return slot
		}
	}
	t.Fatalf("slot %s not listed", code)
	return Slot{}
}

func storedKey(t *testing.T, pool *pgxpool.Pool, code string) string {
	t.Helper()
	var key string
	if err := pool.QueryRow(context.Background(), `SELECT storage_key FROM documentation_slot_examples WHERE slot_code=$1`, code).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require konkit_test, got %q", config.ConnConfig.Database)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
