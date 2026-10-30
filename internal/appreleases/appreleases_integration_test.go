package appreleases

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"testing"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/database/migrations"
	"konkit/internal/media"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func integrationService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
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
	// Releases are global: start from none in the test database.
	if _, err := pool.Exec(context.Background(), `DELETE FROM app_releases`); err != nil {
		t.Fatal(err)
	}
	storage, err := media.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(pool, storage), pool
}

func fakeAPK(body string) io.Reader { return bytes.NewReader(append([]byte("PK\x03\x04"), body...)) }

// makePatches does what RunPatches does in the background.
func makePatches(t *testing.T, service *Service) {
	t.Helper()
	ctx := context.Background()
	for {
		job, err := service.missingPatch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if job == nil {
			return
		}
		if err := service.makePatch(ctx, *job); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntegrationReleasesOfferNewestAndMandatory(t *testing.T) {
	service, _ := integrationService(t)
	ctx := context.Background()
	actor := auth.Principal{}

	if update, err := service.Latest(ctx, "android", 1); err != nil || update != nil {
		t.Fatalf("no releases: update=%+v err=%v", update, err)
	}
	v2, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.1.0", VersionCode: 2, Notes: "Perbaikan scan", Mandatory: true}, fakeAPK("v2"), auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.2.0", VersionCode: 3}, fakeAPK("v3"), auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	// 3 waits for its patch from 2: phones are offered 2 meanwhile.
	if update, err := service.Latest(ctx, "android", 1); err != nil || update == nil || update.VersionCode != 2 {
		t.Fatalf("while 3 waits for patches: %+v err=%v", update, err)
	}
	if list, err := service.List(ctx); err != nil || !list[0].PatchesPending || list[1].PatchesPending {
		t.Fatalf("pending in list: %+v err=%v", list, err)
	}
	makePatches(t, service)

	// A phone on 1 skips the mandatory 2: still must update, to the newest.
	update, err := service.Latest(ctx, "android", 1)
	if err != nil || update == nil || update.VersionCode != 3 || !update.Mandatory || update.DownloadPath == "" {
		t.Fatalf("phone on 1: %+v err=%v", update, err)
	}
	// A phone on 2 is past the mandatory one: 3 is optional.
	if update, err := service.Latest(ctx, "android", 2); err != nil || update == nil || update.Mandatory {
		t.Fatalf("phone on 2: %+v err=%v", update, err)
	}
	// Up to date: nothing to offer.
	if update, err := service.Latest(ctx, "android", 3); err != nil || update != nil {
		t.Fatalf("phone on 3: %+v err=%v", update, err)
	}

	content, err := service.Open(ctx, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(content.Reader)
	_ = content.Reader.Close()
	if string(data) != "PK\x03\x04v2" || content.Size != int64(len(data)) {
		t.Fatalf("apk content %q size %d", data, content.Size)
	}

	// Mandatory off, then the release removed.
	if r, err := service.SetMandatory(ctx, actor, v2.ID, false, auth.ClientMeta{}); err != nil || r.Mandatory {
		t.Fatalf("set mandatory: %+v err=%v", r, err)
	}
	if update, _ := service.Latest(ctx, "android", 1); update == nil || update.Mandatory {
		t.Fatalf("after unsetting mandatory: %+v", update)
	}
	if err := service.Delete(ctx, actor, v2.ID, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(ctx, v2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted release: %v", err)
	}
}

func TestIntegrationUploadRejectsOldVersionsAndNonAPK(t *testing.T) {
	service, _ := integrationService(t)
	ctx := context.Background()
	if _, err := service.Upload(ctx, auth.Principal{}, UploadInput{VersionName: "1.0.5", VersionCode: 5}, fakeAPK("x"), auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(ctx, auth.Principal{}, UploadInput{VersionName: "1.0.4", VersionCode: 4}, fakeAPK("x"), auth.ClientMeta{}); !errors.Is(err, ErrVersionTooLow) {
		t.Fatalf("older version: %v", err)
	}
	if _, err := service.Upload(ctx, auth.Principal{}, UploadInput{VersionName: "1.0.6", VersionCode: 6}, bytes.NewReader([]byte("not a zip")), auth.ClientMeta{}); !errors.Is(err, ErrNotAnAPK) {
		t.Fatalf("not an APK: %v", err)
	}
	if _, err := service.Upload(ctx, auth.Principal{}, UploadInput{VersionName: "", VersionCode: 7}, fakeAPK("x"), auth.ClientMeta{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing version name: %v", err)
	}
}

func TestIntegrationDeltaPatches(t *testing.T) {
	service, pool := integrationService(t)
	ctx := context.Background()
	actor := auth.Principal{}

	body := make([]byte, 200_000)
	for i := range body {
		body[i] = byte(i * 7 % 251)
	}
	oldBody := append([]byte("PK\x03\x04"), body...)
	newBody := append([]byte{}, oldBody...)
	copy(newBody[1000:], "release 3 changes these bytes")
	v2, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.1.0", VersionCode: 2}, bytes.NewReader(oldBody), auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	v3, err := service.Upload(ctx, actor, UploadInput{VersionName: "1.2.0", VersionCode: 3}, bytes.NewReader(newBody), auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	// Not offered before its patch is made.
	if update, err := service.Latest(ctx, "android", 2); err != nil || update != nil {
		t.Fatalf("before the patch is made: %+v err=%v", update, err)
	}
	// Unless making it takes too long.
	if _, err := pool.Exec(ctx, `UPDATE app_releases SET created_at = now() - interval '11 minutes' WHERE id::text = $1`, v3.ID); err != nil {
		t.Fatal(err)
	}
	if update, err := service.Latest(ctx, "android", 2); err != nil || update == nil || update.VersionCode != 3 || update.Patch != nil {
		t.Fatalf("patch late: %+v err=%v", update, err)
	}

	job, err := service.missingPatch(ctx)
	if err != nil || job == nil || job.releaseID != v3.ID || job.fromID != v2.ID {
		t.Fatalf("missing patch: %+v err=%v", job, err)
	}
	if err := service.makePatch(ctx, *job); err != nil {
		t.Fatal(err)
	}
	if job, err := service.missingPatch(ctx); err != nil || job != nil {
		t.Fatalf("patch made twice: %+v err=%v", job, err)
	}

	update, err := service.Latest(ctx, "android", 2)
	if err != nil || update == nil || update.Patch == nil || update.Patch.FromChecksum != v2.Checksum || update.Patch.SizeBytes >= v3.SizeBytes/10 {
		t.Fatalf("phone on 2: %+v err=%v", update, err)
	}
	if update, err := service.Latest(ctx, "android", 1); err != nil || update == nil || update.Patch != nil {
		t.Fatalf("phone on 1 has no patch: %+v err=%v", update, err)
	}
	content, err := service.OpenPatch(ctx, v3.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyPatch(oldBody, content.Reader)
	_ = content.Reader.Close()
	if err != nil || !bytes.Equal(got, newBody) {
		t.Fatalf("patched APK differs: err=%v", err)
	}
	list, err := service.List(ctx)
	if err != nil || len(list) != 2 || len(list[0].Patches) != 1 || !list[0].Patches[0].Ready || list[0].Patches[0].FromVersionCode != 2 {
		t.Fatalf("list: %+v err=%v", list, err)
	}

	// Deleting the older release removes its patch too.
	if err := service.Delete(ctx, actor, v2.ID, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	var patches int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM app_release_patches`).Scan(&patches); err != nil || patches != 0 {
		t.Fatalf("patches after delete: %d err=%v", patches, err)
	}
	if _, err := service.OpenPatch(ctx, v3.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted patch opened: %v", err)
	}
}
