package bast

import (
	"bytes"
	"context"
	"io"
	"sync"

	"konkit/internal/media"
)

// logoCache menyimpan byte logo tender per storage key di memori. Berkas
// logo tidak pernah berubah di bawah key yang sama (unggahan baru mendapat
// key baru), sedangkan di Google Drive setiap baca adalah unduhan HTTPS
// yang lambat dan sebelumnya dilakukan untuk setiap preview BA.
var logoCache = newBytesCache(64)

// logoKey: storage key per storage backend (one per process in production).
type logoKey struct {
	storage media.Storage
	key     string
}

type bytesCache struct {
	mu    sync.Mutex
	max   int
	order []logoKey
	items map[logoKey][]byte
}

func newBytesCache(max int) *bytesCache {
	return &bytesCache{max: max, items: map[logoKey][]byte{}}
}

func (c *bytesCache) get(key logoKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.items[key]
	return data, ok
}

func (c *bytesCache) put(key logoKey, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = data
	c.order = append(c.order, key)
	for len(c.order) > c.max {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
}

// readStoredLogo membaca satu logo, dari cache bila sudah pernah dibaca.
func readStoredLogo(ctx context.Context, storage media.Storage, storageKey string) ([]byte, error) {
	key := logoKey{storage: storage, key: storageKey}
	if data, ok := logoCache.get(key); ok {
		return data, nil
	}
	reader, err := storage.Open(ctx, storageKey)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	logoCache.put(key, data)
	return data, nil
}

// openStoredLogo seperti readStoredLogo untuk pemanggil yang butuh reader.
func openStoredLogo(ctx context.Context, storage media.Storage, storageKey string) (io.ReadCloser, error) {
	data, err := readStoredLogo(ctx, storage, storageKey)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// loadLogoBytes membaca byte logo aktif dari storage, diindeks per AssetID.
func loadLogoBytes(ctx context.Context, storage media.Storage, logos []LogoSnapshot) (map[string][]byte, error) {
	logoBytes := map[string][]byte{}
	for _, logo := range logos {
		if _, exists := logoBytes[logo.AssetID]; exists {
			continue
		}
		data, err := readStoredLogo(ctx, storage, logo.StorageKey)
		if err != nil {
			return nil, err
		}
		logoBytes[logo.AssetID] = data
	}
	return logoBytes, nil
}
