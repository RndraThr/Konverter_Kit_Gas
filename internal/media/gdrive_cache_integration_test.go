package media

import (
	"context"
	"testing"
)

func TestPostgresFolderCacheGetSetRoundTrip(t *testing.T) {
	pool := mediaIntegrationPool(t)
	cache := NewPostgresFolderCache(pool)
	ctx := context.Background()

	_, ok, err := cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected cache miss before Set")
	}

	if err := cache.Set(ctx, "wajo/dokumentasi-foto-video/rakor-test", "drive-folder-abc"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM drive_folder_cache WHERE path_key = 'wajo/dokumentasi-foto-video/rakor-test'`); err != nil {
			t.Logf("cleanup: delete drive_folder_cache failed: %v", err)
		}
	})

	id, ok, err := cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || id != "drive-folder-abc" {
		t.Fatalf("id=%q ok=%v", id, ok)
	}

	// Set again with a different ID must overwrite (idempotent upsert), not error.
	if err := cache.Set(ctx, "wajo/dokumentasi-foto-video/rakor-test", "drive-folder-xyz"); err != nil {
		t.Fatal(err)
	}
	id, ok, err = cache.Get(ctx, "wajo/dokumentasi-foto-video/rakor-test")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || id != "drive-folder-xyz" {
		t.Fatalf("after overwrite: id=%q ok=%v", id, ok)
	}
}
