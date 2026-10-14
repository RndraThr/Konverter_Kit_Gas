package bast

import (
	"bytes"
	"context"
	"io"
	"testing"
)

// countingStorage counts reads, like a slow remote backend would be billed.
type countingStorage struct {
	fakeStorage
	opens map[string]int
}

func (c *countingStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	c.opens[key]++
	return io.NopCloser(bytes.NewReader([]byte("logo:" + key))), nil
}

func TestLogosAreReadFromStorageOnce(t *testing.T) {
	storage := &countingStorage{opens: map[string]int{}}
	logos := []LogoSnapshot{{AssetID: "a", StorageKey: "k-a"}, {AssetID: "b", StorageKey: "k-b"}}
	for range 3 {
		got, err := loadLogoBytes(context.Background(), storage, logos)
		if err != nil {
			t.Fatal(err)
		}
		if string(got["a"]) != "logo:k-a" || string(got["b"]) != "logo:k-b" {
			t.Fatalf("logos=%q", got)
		}
	}
	if storage.opens["k-a"] != 1 || storage.opens["k-b"] != 1 {
		t.Fatalf("opens=%v, want each logo read once", storage.opens)
	}
	// Another storage backend never sees this one's logos.
	other := &countingStorage{opens: map[string]int{}}
	if _, err := loadLogoBytes(context.Background(), other, logos[:1]); err != nil || other.opens["k-a"] != 1 {
		t.Fatalf("other storage opens=%v err=%v", other.opens, err)
	}
}

func TestLogoCacheEvictsOldest(t *testing.T) {
	cache := newBytesCache(2)
	storage := &fakeStorage{}
	for _, k := range []string{"1", "2", "3"} {
		cache.put(logoKey{storage: storage, key: k}, []byte(k))
	}
	if _, ok := cache.get(logoKey{storage: storage, key: "1"}); ok {
		t.Fatal("oldest logo must be evicted")
	}
	if data, ok := cache.get(logoKey{storage: storage, key: "3"}); !ok || string(data) != "3" {
		t.Fatal("newest logo must stay")
	}
}
