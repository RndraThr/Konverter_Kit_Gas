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
