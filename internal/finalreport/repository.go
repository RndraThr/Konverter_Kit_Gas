package finalreport

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const settingsColumns = `program_id::text, regency_id::text, leader_count, admin_count, technician_count, helper_count, machine_manufacturer_name, COALESCE(delivery_hose_date::text,''), COALESCE(delivery_machine_group_date::text,''), COALESCE(delivery_gas_cylinder_date::text,''), updated_at`

func (r *Repository) GetSettings(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Settings, error) {
	var s Settings
	err := r.pool.QueryRow(ctx, `
		SELECT `+settingsColumns+`
		FROM final_report_settings
		WHERE program_id=$1 AND regency_id=$2 AND ($3 OR regency_id::text=ANY($4))
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&s.ProgramID, &s.RegencyID, &s.LeaderCount, &s.AdminCount, &s.TechnicianCount, &s.HelperCount,
		&s.MachineManufacturerName, &s.DeliveryHoseDate, &s.DeliveryMachineGroupDate, &s.DeliveryGasCylinderDate, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{ProgramID: programID, RegencyID: regencyID}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get final report settings: %w", err)
	}
	return s, nil
}

func (r *Repository) PutSettings(ctx context.Context, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error) {
	var s Settings
	err := r.pool.QueryRow(ctx, `
		INSERT INTO final_report_settings (program_id, regency_id, leader_count, admin_count, technician_count, helper_count, machine_manufacturer_name, delivery_hose_date, delivery_machine_group_date, delivery_gas_cylinder_date, updated_by)
		SELECT $1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8,'')::date, NULLIF($9,'')::date, NULLIF($10,'')::date, NULLIF($11,'')::uuid
		WHERE $12 OR $2::text=ANY($13)
		ON CONFLICT (program_id, regency_id) DO UPDATE SET
			leader_count=EXCLUDED.leader_count, admin_count=EXCLUDED.admin_count, technician_count=EXCLUDED.technician_count,
			helper_count=EXCLUDED.helper_count, machine_manufacturer_name=EXCLUDED.machine_manufacturer_name,
			delivery_hose_date=EXCLUDED.delivery_hose_date, delivery_machine_group_date=EXCLUDED.delivery_machine_group_date,
			delivery_gas_cylinder_date=EXCLUDED.delivery_gas_cylinder_date, updated_by=EXCLUDED.updated_by, updated_at=now()
		RETURNING `+settingsColumns,
		input.ProgramID, input.RegencyID, input.LeaderCount, input.AdminCount, input.TechnicianCount, input.HelperCount,
		input.MachineManufacturerName, input.DeliveryHoseDate, input.DeliveryMachineGroupDate, input.DeliveryGasCylinderDate,
		actor.UserID, scope.Unrestricted, scope.RegencyIDs,
	).Scan(
		&s.ProgramID, &s.RegencyID, &s.LeaderCount, &s.AdminCount, &s.TechnicianCount, &s.HelperCount,
		&s.MachineManufacturerName, &s.DeliveryHoseDate, &s.DeliveryMachineGroupDate, &s.DeliveryGasCylinderDate, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, ErrRegencyOutOfScope
	}
	if err != nil {
		return Settings{}, fmt.Errorf("put final report settings: %w", err)
	}
	return s, nil
}

func (r *Repository) ListPhotoPicks(ctx context.Context, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id::text, p.program_id::text, p.regency_id::text, p.slide_key, p.activity_media_id::text, p.sort_order,
		       m.display_name, '/api/v1/activities/media/'||m.id::text||'/content', m.storage_key, p.created_at
		FROM final_report_photo_picks p
		JOIN activity_media m ON m.id = p.activity_media_id AND m.status='active'
		WHERE p.program_id=$1 AND p.regency_id=$2 AND p.slide_key=$3 AND ($4 OR p.regency_id::text=ANY($5))
		ORDER BY p.sort_order
	`, programID, regencyID, slideKey, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list photo picks: %w", err)
	}
	defer rows.Close()
	items := []PhotoPick{}
	for rows.Next() {
		var p PhotoPick
		if err := rows.Scan(&p.ID, &p.ProgramID, &p.RegencyID, &p.SlideKey, &p.ActivityMediaID, &p.SortOrder, &p.DisplayName, &p.ContentURL, &p.StorageKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// ReplacePhotoPicks swaps the entire pick list for one slide_key in a single
// transaction: validates every activity_media_id belongs to regencyID (and is
// within scope), deletes the old picks, inserts the new ones in order.
func (r *Repository) ReplacePhotoPicks(ctx context.Context, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error) {
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, mediaID := range activityMediaIDs {
		var belongs bool
		if err := tx.QueryRow(ctx, `SELECT regency_id=$2::uuid FROM activity_media WHERE id=$1::uuid AND status='active'`, mediaID, regencyID).Scan(&belongs); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPhotoOutOfScope
			}
			return nil, fmt.Errorf("check photo scope: %w", err)
		}
		if !belongs {
			return nil, ErrPhotoOutOfScope
		}
	}
	if !scope.Unrestricted {
		allowed := false
		for _, id := range scope.RegencyIDs {
			if id == regencyID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrRegencyOutOfScope
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM final_report_photo_picks WHERE program_id=$1 AND regency_id=$2 AND slide_key=$3`, programID, regencyID, slideKey); err != nil {
		return nil, fmt.Errorf("clear photo picks: %w", err)
	}
	for i, mediaID := range activityMediaIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO final_report_photo_picks (program_id, regency_id, slide_key, activity_media_id, sort_order) VALUES ($1::uuid,$2::uuid,$3,$4::uuid,$5)`, programID, regencyID, slideKey, mediaID, i); err != nil {
			return nil, fmt.Errorf("insert photo pick: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListPhotoPicks(ctx, programID, regencyID, slideKey, scope)
}
