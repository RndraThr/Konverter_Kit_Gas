package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type activityFixture struct {
	inScopeRegencyID    string
	outOfScopeRegencyID string
	actorUserID         string
}

func seedActivityFixture(t *testing.T, pool *pgxpool.Pool) activityFixture {
	t.Helper()
	ctx := context.Background()
	var fixture activityFixture

	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Wajo Activities Test','WAT',true) RETURNING id::text`).Scan(&fixture.inScopeRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan','Bone Activities Test','BAT',true) RETURNING id::text`).Scan(&fixture.outOfScopeRegencyID))
	must(t, pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES('activities-test-actor','activities-test-actor@example.test','x') RETURNING id::text`).Scan(&fixture.actorUserID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM activity_media WHERE regency_id IN ($1,$2)`, fixture.inScopeRegencyID, fixture.outOfScopeRegencyID); err != nil {
			t.Logf("cleanup: delete activity_media failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, fixture.actorUserID); err != nil {
			t.Logf("cleanup: delete users failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id IN ($1,$2)`, fixture.inScopeRegencyID, fixture.outOfScopeRegencyID); err != nil {
			t.Logf("cleanup: delete regencies failed: %v", err)
		}
	})
	return fixture
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// seedScheduleForTest creates a minimal program_schedules row (with its required program,
// package template version, documentation template version, and regency) so slot repository
// tests can scope rows by schedule_id. It returns both the schedule id and the regency id it
// belongs to, so callers can build an auth.RegencyScope that is or isn't in scope for it.
func seedScheduleForTest(t *testing.T, pool *pgxpool.Pool) (scheduleID, regencyID string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(t.Name(), "/", "-") + "-" + fmt.Sprintf("%d", time.Now().UnixNano())

	var regencyCode string
	must(t, pool.QueryRow(ctx, `
		SELECT code FROM (
			SELECT chr(65+(g%26)::int)||chr(65+((g/26)%26)::int)||chr(65+((g/676)%26)::int) AS code
			FROM generate_series(0,17575) AS g
		) candidates
		WHERE NOT EXISTS (SELECT 1 FROM regencies WHERE document_code=candidates.code)
		ORDER BY code DESC LIMIT 1
	`).Scan(&regencyCode))
	must(t, pool.QueryRow(ctx, `INSERT INTO regencies(province_name,name,document_code,is_active) VALUES('Sulawesi Selatan',$1,$2,true) RETURNING id::text`, "Slot Test "+suffix, regencyCode).Scan(&regencyID))

	var programID string
	must(t, pool.QueryRow(ctx, `INSERT INTO programs(code,name,program_type,fiscal_year,status) VALUES($1,'Program Test Slot','farmer',2026,'active') RETURNING id::text`, "SLOT-TEST-"+suffix).Scan(&programID))
	var packageID, documentationID string
	must(t, pool.QueryRow(ctx, `INSERT INTO package_template_versions(template_code,version,name,program_type,values_json,status) VALUES($1,1,'Paket Test Slot','farmer','{}'::jsonb,'published') RETURNING id::text`, "PKG-SLOT-"+suffix).Scan(&packageID))
	must(t, pool.QueryRow(ctx, `INSERT INTO documentation_template_versions(template_code,version,name,program_type,status) VALUES($1,1,'Dok Test Slot','farmer','published') RETURNING id::text`, "DOC-SLOT-"+suffix).Scan(&documentationID))
	must(t, pool.QueryRow(ctx, `INSERT INTO program_schedules(program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,receipt_policy_json) VALUES($1,$2,$3,$4,'Jadwal Test Slot','2026-01-01','2026-12-31','active','{}'::jsonb) RETURNING id::text`, programID, regencyID, packageID, documentationID).Scan(&scheduleID))

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM supporting_document_slots WHERE schedule_id=$1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM program_schedules WHERE id=$1`, scheduleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM programs WHERE id=$1`, programID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM package_template_versions WHERE id=$1`, packageID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM documentation_template_versions WHERE id=$1`, documentationID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM regencies WHERE id=$1`, regencyID)
	})
	return scheduleID, regencyID
}

func TestInsertAndListSlotsScopedBySchedule(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	scheduleAID, regencyAID := seedScheduleForTest(t, pool)
	scheduleBID, regencyBID := seedScheduleForTest(t, pool)
	unrestricted := auth.RegencyScope{Unrestricted: true}

	_, err := repo.InsertSlot(ctx, auth.Principal{}, scheduleAID, "Surat Tugas", auth.ClientMeta{}, unrestricted)
	if err != nil {
		t.Fatalf("InsertSlot() error = %v", err)
	}
	slotB, err := repo.InsertSlot(ctx, auth.Principal{}, scheduleBID, "SK Penetapan", auth.ClientMeta{}, unrestricted)
	if err != nil {
		t.Fatalf("InsertSlot() error = %v", err)
	}

	listA, err := repo.ListSlots(ctx, scheduleAID, unrestricted)
	if err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if len(listA) != 1 || listA[0].Name != "Surat Tugas" {
		t.Fatalf("listA = %+v, want exactly one slot named Surat Tugas", listA)
	}

	// Cross-regency IDOR check (security review finding): a caller scoped only to regency A must
	// not be able to list, create, or patch slots belonging to schedule B's regency B.
	scopedToA := auth.RegencyScope{RegencyIDs: []string{regencyAID}}
	if regencyAID == regencyBID {
		t.Fatalf("test fixtures produced the same regency for schedule A and B, scope assertions below are meaningless")
	}

	listBFromA, err := repo.ListSlots(ctx, scheduleBID, scopedToA)
	if err != nil {
		t.Fatalf("ListSlots() out-of-scope error = %v, want nil (empty list, not an error)", err)
	}
	if len(listBFromA) != 0 {
		t.Fatalf("ListSlots() out-of-scope = %+v, want empty list", listBFromA)
	}

	if _, err := repo.InsertSlot(ctx, auth.Principal{}, scheduleBID, "Should Not Insert", auth.ClientMeta{}, scopedToA); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("InsertSlot() out-of-scope err = %v, want ErrScheduleNotFound", err)
	}

	renamed := "Should Not Rename"
	if _, err := repo.PatchSlotRow(ctx, auth.Principal{}, slotB.ID, &renamed, nil, auth.ClientMeta{}, scopedToA); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("PatchSlotRow() out-of-scope err = %v, want ErrSlotNotFound", err)
	}

	// Confirm slot B is genuinely untouched by the rejected patch attempt above.
	listB, err := repo.ListSlots(ctx, scheduleBID, unrestricted)
	if err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if len(listB) != 1 || listB[0].Name != "SK Penetapan" {
		t.Fatalf("listB = %+v, want slot B name unchanged by rejected out-of-scope patch", listB)
	}
}

func TestGetRegencyEnforcesScope(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}

	info, err := repository.GetRegency(ctx, fixture.inScopeRegencyID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if info.DocumentCode != "WAT" || info.Name != "Wajo Activities Test" {
		t.Fatalf("info = %+v", info)
	}

	if _, err := repository.GetRegency(ctx, fixture.outOfScopeRegencyID, scope); !errors.Is(err, ErrRegencyNotFound) {
		t.Fatalf("err = %v, want ErrRegencyNotFound", err)
	}
}

func TestInsertListGetByIDAndSoftDeleteRoundTrip(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.actorUserID}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "test"}
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}

	created, err := repository.Insert(ctx, actor, insertInput{
		RegencyID: fixture.inScopeRegencyID, ActivityType: "rakor", StorageKey: "storage-key-1",
		DisplayName: "WAT-RAKOR-20260916-154500", OriginalFilename: "foto.jpg",
		MediaType: "image", MimeType: "image/jpeg", ByteSize: 100, Checksum: "checksum-1", Source: "gallery",
	}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if created.RegencyName != "Wajo Activities Test" || created.Status != "active" || created.DisplayName != "WAT-RAKOR-20260916-154500" {
		t.Fatalf("created = %+v", created)
	}
	if created.ProgramID != nil {
		t.Fatalf("legacy row program_id=%v, want nil", created.ProgramID)
	}

	page, err := repository.List(ctx, Filter{RegencyID: fixture.inScopeRegencyID, ActivityType: "rakor", Page: 1, PageSize: 20}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("page = %+v", page)
	}
	if page.Items[0].ProgramID != nil {
		t.Fatalf("legacy list must preserve nullable program_id, got %v", page.Items[0].ProgramID)
	}

	outOfScope := auth.RegencyScope{RegencyIDs: []string{fixture.outOfScopeRegencyID}}
	if _, err := repository.GetByID(ctx, created.ID, outOfScope); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for out-of-scope access", err)
	}

	storageKey, err := repository.SoftDelete(ctx, actor, created.ID, meta, scope)
	if err != nil {
		t.Fatal(err)
	}
	if storageKey != "storage-key-1" {
		t.Fatalf("storageKey = %q", storageKey)
	}

	if _, err := repository.GetByID(ctx, created.ID, scope); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound after soft delete", err)
	}

	if err := repository.restoreAfterFailedStorageDelete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetByID(ctx, created.ID, scope); err != nil {
		t.Fatal(err)
	}
}

// TestInsertDokumenPendukungDocumentRow proves migrations 00066-00068 (which widened the
// activity_media CHECK constraints on activity_type, media_type, and mime_type to accept
// "dokumen_pendukung" / "document" / "application/pdf") actually compose with repository.Insert's
// real INSERT statement against the fully-migrated activity_media table. Each migration's own
// test only exercised its single CHECK constraint on a throwaway temp-table copy, and the
// pre-existing roundtrip test above only ever inserted with "rakor"/"image"/"image/jpeg" values —
// so nothing in the committed suite proved this exact combination round-trips end to end. This was
// previously only verified manually via a throwaway probe script during the end-to-end smoke pass.
func TestInsertDokumenPendukungDocumentRow(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.actorUserID}
	meta := auth.ClientMeta{IPAddress: "127.0.0.1", UserAgent: "test"}
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}

	created, err := repository.Insert(ctx, actor, insertInput{
		RegencyID: fixture.inScopeRegencyID, ActivityType: "dokumen_pendukung", StorageKey: "storage-key-dokumen-1",
		DisplayName: "WAT-DOKPEND-20260916-154500", OriginalFilename: "scan.pdf",
		MediaType: "document", MimeType: "application/pdf", ByteSize: 4096, Checksum: "checksum-dokumen-1", Source: "camera",
	}, meta)
	if err != nil {
		t.Fatalf("Insert() error = %v, want nil (migrations 00066-00068 should allow this combination)", err)
	}
	if created.ActivityType != "dokumen_pendukung" || created.MediaType != "document" || created.MimeType != "application/pdf" {
		t.Fatalf("created = %+v, want activity_type=dokumen_pendukung media_type=document mime_type=application/pdf", created)
	}
	if created.RegencyName != "Wajo Activities Test" || created.Status != "active" || created.DisplayName != "WAT-DOKPEND-20260916-154500" {
		t.Fatalf("created = %+v", created)
	}

	fetched, err := repository.GetByID(ctx, created.ID, scope)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if fetched.ID != created.ID || fetched.ActivityType != "dokumen_pendukung" || fetched.MediaType != "document" || fetched.MimeType != "application/pdf" {
		t.Fatalf("fetched = %+v, want round-trip of created row", fetched)
	}
}
