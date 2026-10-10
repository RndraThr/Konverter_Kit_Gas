package activities

import (
	"context"
	"testing"
	"time"

	"konkit/internal/auth"
)

func TestActivitySyncDeltaIncludesDeletions(t *testing.T) {
	pool := activitiesIntegrationPool(t)
	fixture := seedActivityFixture(t, pool)
	repository := NewRepository(pool)
	ctx := context.Background()
	actor := auth.Principal{UserID: fixture.actorUserID}
	scope := auth.RegencyScope{RegencyIDs: []string{fixture.inScopeRegencyID}}
	insert := func(key string) ActivityMedia {
		t.Helper()
		item, err := repository.Insert(ctx, actor, insertInput{
			RegencyID: fixture.inScopeRegencyID, ActivityType: "ceremony_sosialisasi", StorageKey: key,
			DisplayName: key, OriginalFilename: key + ".jpg", MediaType: "image", MimeType: "image/jpeg",
			ByteSize: 10, Checksum: key, Source: "camera",
		}, auth.ClientMeta{})
		must(t, err)
		return item
	}
	kept, removed := insert("sync-kept"), insert("sync-removed")

	full, err := repository.Sync(ctx, "", fixture.inScopeRegencyID, nil, scope)
	must(t, err)
	if len(full.Items) != 2 || full.ServerTime.IsZero() || full.Items[0].ContentURL == "" {
		t.Fatalf("full sync = %+v", full)
	}

	cursor := full.ServerTime
	_, err = repository.SoftDelete(ctx, actor, removed.ID, auth.ClientMeta{}, scope)
	must(t, err)
	delta, err := repository.Sync(ctx, "", fixture.inScopeRegencyID, &cursor, scope)
	must(t, err)
	if len(delta.Items) != 1 || delta.Items[0].ID != removed.ID || delta.Items[0].Status != "deleted" {
		t.Fatalf("delta after delete = %+v, want only the deleted item", delta.Items)
	}

	// A full sync after the delete no longer lists it.
	full, err = repository.Sync(ctx, "", fixture.inScopeRegencyID, nil, scope)
	must(t, err)
	if len(full.Items) != 1 || full.Items[0].ID != kept.ID {
		t.Fatalf("full sync after delete = %+v", full.Items)
	}

	// Out of scope: nothing.
	future := time.Now().Add(-time.Hour)
	none, err := repository.Sync(ctx, "", fixture.inScopeRegencyID, &future, auth.RegencyScope{RegencyIDs: []string{fixture.outOfScopeRegencyID}})
	must(t, err)
	if len(none.Items) != 0 {
		t.Fatalf("out-of-scope sync = %+v", none.Items)
	}
}
