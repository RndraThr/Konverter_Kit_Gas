package activities

import (
	"context"
	"errors"
	"testing"

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

	page, err := repository.List(ctx, Filter{RegencyID: fixture.inScopeRegencyID, ActivityType: "rakor", Page: 1, PageSize: 20}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("page = %+v", page)
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
