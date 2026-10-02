package bast

import (
	"context"
	"io"

	"konkit/internal/media"
)

// loadLogoBytes membaca byte logo aktif dari storage, diindeks per AssetID.
func loadLogoBytes(ctx context.Context, storage media.Storage, logos []LogoSnapshot) (map[string][]byte, error) {
	logoBytes := map[string][]byte{}
	for _, logo := range logos {
		if _, exists := logoBytes[logo.AssetID]; exists {
			continue
		}
		reader, err := storage.Open(ctx, logo.StorageKey)
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
		logoBytes[logo.AssetID] = data
	}
	return logoBytes, nil
}
