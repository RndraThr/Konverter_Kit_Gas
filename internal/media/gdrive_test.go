package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type fakeDriveFilesAPI struct {
	foldersByParentAndName map[string]string // key: parentID+"/"+name -> folder ID
	createdFolders         []string          // names, in creation order
	uploaded               map[string][]byte // key: file ID -> content
	nextID                 int
	deleteErr              error
	deletedIDs             []string
}

func newFakeDriveFilesAPI() *fakeDriveFilesAPI {
	return &fakeDriveFilesAPI{
		foldersByParentAndName: map[string]string{},
		uploaded:               map[string][]byte{},
	}
}

func (f *fakeDriveFilesAPI) newID() string {
	f.nextID++
	return "fake-id-" + string(rune('a'+f.nextID))
}

func (f *fakeDriveFilesAPI) findFolder(_ context.Context, name, parentID string) (string, error) {
	id, ok := f.foldersByParentAndName[parentID+"/"+name]
	if !ok {
		return "", nil
	}
	return id, nil
}

func (f *fakeDriveFilesAPI) createFolder(_ context.Context, name, parentID string) (string, error) {
	id := f.newID()
	f.foldersByParentAndName[parentID+"/"+name] = id
	f.createdFolders = append(f.createdFolders, name)
	return id, nil
}

func (f *fakeDriveFilesAPI) uploadFile(_ context.Context, _ string, _ string, r io.Reader) (string, int64, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	id := f.newID()
	f.uploaded[id] = content
	return id, int64(len(content)), nil
}

func (f *fakeDriveFilesAPI) downloadFile(_ context.Context, id string) (io.ReadCloser, error) {
	content, ok := f.uploaded[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (f *fakeDriveFilesAPI) deleteFile(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedIDs = append(f.deletedIDs, id)
	delete(f.uploaded, id)
	return nil
}

type fakeFolderCache struct {
	entries map[string]string
	sets    int
}

func newFakeFolderCache() *fakeFolderCache {
	return &fakeFolderCache{entries: map[string]string{}}
}

func (c *fakeFolderCache) Get(_ context.Context, pathKey string) (string, bool, error) {
	id, ok := c.entries[pathKey]
	return id, ok, nil
}

func (c *fakeFolderCache) Set(_ context.Context, pathKey, driveFolderID string) error {
	c.entries[pathKey] = driveFolderID
	c.sets++
	return nil
}

func TestGoogleDriveStoragePutCreatesNestedFoldersAndCachesThem(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	_, size, checksum, err := storage.Put(context.Background(), "file-key-1", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("photo bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len("photo bytes")) || checksum == "" {
		t.Fatalf("size=%d checksum=%q", size, checksum)
	}
	// First Put creates 4 from the path + 2 reserved folders (BERITA ACARA, DOKUMEN PENDUKUNG)
	if len(api.createdFolders) != 6 {
		t.Fatalf("expected 6 folders created (4 path + 2 reserved), got %v", api.createdFolders)
	}
	// Cache gets 4 from the path + 2 from the reserved folders
	if cache.sets != 6 {
		t.Fatalf("expected 6 cache writes (4 path + 2 reserved), got %d", cache.sets)
	}

	// Second Put with the SAME folderPath must reuse cached folder IDs, not create new ones.
	if _, _, _, err := storage.Put(context.Background(), "file-key-2", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("more bytes")); err != nil {
		t.Fatal(err)
	}
	if len(api.createdFolders) != 6 {
		t.Fatalf("expected still 6 folders created after reusing cached path, got %v", api.createdFolders)
	}
}

func TestGoogleDriveStorageOpenAndDelete(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	storageKey, _, _, err := storage.Put(context.Background(), "file-key-3", []string{"Konkit 2026", "Wajo"}, bytes.NewBufferString("hello drive"))
	if err != nil {
		t.Fatal(err)
	}
	if storageKey == "file-key-3" {
		t.Fatal("expected storageKey to be the fake API's assigned file ID, not the caller-supplied key")
	}

	reader, err := storage.Open(context.Background(), storageKey)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "hello drive" {
		t.Fatalf("content=%q", content)
	}

	if err := storage.Delete(context.Background(), storageKey); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Open(context.Background(), storageKey); err == nil {
		t.Fatal("expected Open after Delete to fail")
	}
}

func TestGoogleDriveStoragePutCreatesReservedRegencyFoldersOnce(t *testing.T) {
	api := newFakeDriveFilesAPI()
	cache := newFakeFolderCache()
	storage := &GoogleDriveStorage{api: api, cache: cache, rootFolderID: "root-1"}

	if _, _, _, err := storage.Put(context.Background(), "file-key-1", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Rakor"}, bytes.NewBufferString("photo")); err != nil {
		t.Fatal(err)
	}
	wantReserved := map[string]bool{"BERITA ACARA (BA)": false, "DOKUMEN PENDUKUNG": false}
	for _, name := range api.createdFolders {
		if _, ok := wantReserved[name]; ok {
			wantReserved[name] = true
		}
	}
	for name, created := range wantReserved {
		if !created {
			t.Fatalf("expected reserved folder %q to be created alongside the regency folder, created folders: %v", name, api.createdFolders)
		}
	}
	countBefore := len(api.createdFolders)

	// A second Put for a DIFFERENT activity type under the SAME regency must
	// reuse the cached regency folder and must NOT recreate the reserved
	// siblings.
	if _, _, _, err := storage.Put(context.Background(), "file-key-2", []string{"Konkit 2026", "Wajo", "Dokumentasi Foto & Video", "Pelatihan Teknis"}, bytes.NewBufferString("photo 2")); err != nil {
		t.Fatal(err)
	}
	if len(api.createdFolders) != countBefore+1 { // +1 for the new "Pelatihan Teknis" activity folder only
		t.Fatalf("expected no reserved-folder recreation, createdFolders after second Put: %v", api.createdFolders)
	}
}
