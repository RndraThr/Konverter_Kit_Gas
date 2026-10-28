package geo

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed data/indonesia-regencies.json
var regencyBoundaryAsset []byte

// BoundaryImportResult reports one EnsureRegencyBoundaries run. Imported is the number of
// boundary rows written (0 when the stored checksum already matches), Matched the master
// regencies linked to a boundary, Unmatched the master regency names that have no boundary.
type BoundaryImportResult struct {
	Imported  int
	Matched   int
	Unmatched []string
}

type boundaryAsset struct {
	Source    string `json:"source"`
	License   string `json:"license"`
	Regencies []struct {
		Name  string         `json:"n"`
		Rings [][][2]float64 `json:"r"`
	} `json:"regencies"`
}

// EnsureRegencyBoundaries loads the embedded boundary asset into regency_boundaries when the
// table does not already hold exactly this asset, then (re)links boundaries to master regencies.
func EnsureRegencyBoundaries(ctx context.Context, pool *pgxpool.Pool) (BoundaryImportResult, error) {
	var asset boundaryAsset
	if err := json.Unmarshal(regencyBoundaryAsset, &asset); err != nil {
		return BoundaryImportResult{}, fmt.Errorf("parse boundary asset: %w", err)
	}
	sum := sha256.Sum256(regencyBoundaryAsset)
	checksum := hex.EncodeToString(sum[:])

	tx, err := pool.Begin(ctx)
	if err != nil {
		return BoundaryImportResult{}, err
	}
	defer tx.Rollback(ctx)

	var total, current int
	if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE asset_checksum = $1) FROM regency_boundaries`, checksum).Scan(&total, &current); err != nil {
		return BoundaryImportResult{}, err
	}

	var result BoundaryImportResult
	if total != len(asset.Regencies) || current != total {
		if _, err := tx.Exec(ctx, `DELETE FROM regency_boundaries`); err != nil {
			return BoundaryImportResult{}, err
		}
		batch := &pgx.Batch{}
		for _, regency := range asset.Regencies {
			geojson, err := multiPolygonGeoJSON(regency.Rings)
			if err != nil {
				return BoundaryImportResult{}, fmt.Errorf("boundary %q: %w", regency.Name, err)
			}
			batch.Queue(`INSERT INTO regency_boundaries (boundary_name, normalized_key, geom, source, license, asset_checksum)
				VALUES ($1, $2, ST_Multi(ST_CollectionExtract(ST_MakeValid(ST_SetSRID(ST_GeomFromGeoJSON($3), 4326)), 3)), $4, $5, $6)`,
				regency.Name, NormalizeRegencyName(regency.Name), geojson, asset.Source, asset.License, checksum)
		}
		br := tx.SendBatch(ctx, batch)
		for range asset.Regencies {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return BoundaryImportResult{}, fmt.Errorf("insert boundary: %w", err)
			}
			result.Imported++
		}
		if err := br.Close(); err != nil {
			return BoundaryImportResult{}, err
		}
	}

	// Re-link on every run so regencies added or renamed after the import get picked up.
	if _, err := tx.Exec(ctx, `UPDATE regency_boundaries SET regency_id = NULL`); err != nil {
		return BoundaryImportResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text, name FROM regencies ORDER BY is_active DESC, province_name, name, id`)
	if err != nil {
		return BoundaryImportResult{}, err
	}
	type master struct{ id, name string }
	var masters []master
	for rows.Next() {
		var m master
		if err := rows.Scan(&m.id, &m.name); err != nil {
			rows.Close()
			return BoundaryImportResult{}, err
		}
		masters = append(masters, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return BoundaryImportResult{}, err
	}
	linked := map[string]bool{}
	for _, m := range masters {
		key := NormalizeRegencyName(m.name)
		if linked[key] {
			continue // same-named regency in another province: the first (active) one wins
		}
		tag, err := tx.Exec(ctx, `UPDATE regency_boundaries SET regency_id = $1::uuid WHERE normalized_key = $2`, m.id, key)
		if err != nil {
			return BoundaryImportResult{}, err
		}
		if tag.RowsAffected() > 0 {
			linked[key] = true
			result.Matched++
		} else {
			result.Unmatched = append(result.Unmatched, m.name)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BoundaryImportResult{}, err
	}
	return result, nil
}

// multiPolygonGeoJSON turns [lon,lat] rings into a MultiPolygon of single-ring polygons,
// closing rings whose first and last points differ.
func multiPolygonGeoJSON(rings [][][2]float64) (string, error) {
	polygons := make([][][][2]float64, 0, len(rings))
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		if ring[0] != ring[len(ring)-1] {
			ring = append(append([][2]float64(nil), ring...), ring[0])
		}
		if len(ring) < 4 {
			continue
		}
		polygons = append(polygons, [][][2]float64{ring})
	}
	if len(polygons) == 0 {
		return "", fmt.Errorf("no usable rings")
	}
	out, err := json.Marshal(map[string]any{"type": "MultiPolygon", "coordinates": polygons})
	return string(out), err
}
