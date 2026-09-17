package media

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type folderCache interface {
	Get(ctx context.Context, pathKey string) (driveFolderID string, ok bool, err error)
	Set(ctx context.Context, pathKey, driveFolderID string) error
}

type postgresFolderCache struct {
	pool *pgxpool.Pool
}

func NewPostgresFolderCache(pool *pgxpool.Pool) *postgresFolderCache {
	return &postgresFolderCache{pool: pool}
}

func (c *postgresFolderCache) Get(ctx context.Context, pathKey string) (string, bool, error) {
	var driveFolderID string
	err := c.pool.QueryRow(ctx, `SELECT drive_folder_id FROM drive_folder_cache WHERE path_key = $1`, pathKey).Scan(&driveFolderID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get folder cache entry: %w", err)
	}
	return driveFolderID, true, nil
}

func (c *postgresFolderCache) Set(ctx context.Context, pathKey, driveFolderID string) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO drive_folder_cache (path_key, drive_folder_id) VALUES ($1, $2)
		ON CONFLICT (path_key) DO UPDATE SET drive_folder_id = EXCLUDED.drive_folder_id
	`, pathKey, driveFolderID)
	if err != nil {
		return fmt.Errorf("set folder cache entry: %w", err)
	}
	return nil
}
