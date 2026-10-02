package bast

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetScheduleSettings(ctx context.Context, scheduleID string, scope auth.RegencyScope) (ScheduleSettings, error) {
	var settings ScheduleSettings
	err := r.pool.QueryRow(ctx, `
		SELECT ps.id::text,
			COALESCE(st.handover_location,''), COALESCE(st.consultant_company_name,''),
			COALESCE(st.agriculture_office_name,''), COALESCE(st.agriculture_office_nip,''),
			COALESCE(st.installer_name,''), COALESCE(st.supervisor_name,''),
			COALESCE(st.pertamina_rep_name,''), COALESCE(st.updated_at, now())
		FROM program_schedules ps
		LEFT JOIN bast_schedule_settings st ON st.schedule_id = ps.id
		WHERE ps.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(
		&settings.ScheduleID, &settings.HandoverLocation, &settings.ConsultantCompanyName,
		&settings.AgricultureOfficeName, &settings.AgricultureOfficeNIP,
		&settings.InstallerName, &settings.SupervisorName,
		&settings.PertaminaRepName, &settings.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ScheduleSettings{}, ErrNotFound
	}
	if err != nil {
		return ScheduleSettings{}, fmt.Errorf("get BA schedule settings: %w", err)
	}
	return settings, nil
}

func (r *Repository) UpsertScheduleSettings(ctx context.Context, actor auth.Principal, input ScheduleSettingsInput, scope auth.RegencyScope, meta auth.ClientMeta) (ScheduleSettings, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ScheduleSettings{}, fmt.Errorf("begin schedule settings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var scheduleID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM program_schedules WHERE id=$1 AND ($2 OR regency_id::text = ANY($3)) FOR UPDATE`, input.ScheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&scheduleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScheduleSettings{}, ErrNotFound
		}
		return ScheduleSettings{}, fmt.Errorf("lock schedule settings: %w", err)
	}

	var settings ScheduleSettings
	err = tx.QueryRow(ctx, `
		INSERT INTO bast_schedule_settings(schedule_id,handover_location,consultant_company_name,agriculture_office_name,agriculture_office_nip,installer_name,supervisor_name,pertamina_rep_name)
		VALUES($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''))
		ON CONFLICT(schedule_id) DO UPDATE SET
			handover_location=EXCLUDED.handover_location,
			consultant_company_name=EXCLUDED.consultant_company_name,
			agriculture_office_name=EXCLUDED.agriculture_office_name,
			agriculture_office_nip=EXCLUDED.agriculture_office_nip,
			installer_name=EXCLUDED.installer_name,
			supervisor_name=EXCLUDED.supervisor_name,
			pertamina_rep_name=EXCLUDED.pertamina_rep_name,
			updated_at=now()
		RETURNING schedule_id::text,COALESCE(handover_location,''),COALESCE(consultant_company_name,''),COALESCE(agriculture_office_name,''),COALESCE(agriculture_office_nip,''),COALESCE(installer_name,''),COALESCE(supervisor_name,''),COALESCE(pertamina_rep_name,''),updated_at
	`, input.ScheduleID, input.HandoverLocation, input.ConsultantCompanyName, input.AgricultureOfficeName, input.AgricultureOfficeNIP, input.InstallerName, input.SupervisorName, input.PertaminaRepName).Scan(
		&settings.ScheduleID, &settings.HandoverLocation, &settings.ConsultantCompanyName,
		&settings.AgricultureOfficeName, &settings.AgricultureOfficeNIP,
		&settings.InstallerName, &settings.SupervisorName,
		&settings.PertaminaRepName, &settings.UpdatedAt,
	)
	if err != nil {
		return ScheduleSettings{}, fmt.Errorf("upsert schedule settings: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.schedule_settings_updated", ResourceType: "bast_schedule_setting", ResourceID: scheduleID, Metadata: map[string]any{"schedule_id": scheduleID}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return ScheduleSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleSettings{}, fmt.Errorf("commit schedule settings: %w", err)
	}
	return settings, nil
}
