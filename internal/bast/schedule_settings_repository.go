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
			COALESCE(st.pertamina_rep_name,''), COALESCE(st.rakorda_location,''),
			COALESCE(st.rakorda_row_count,$4), COALESCE(st.sosialisasi_location,''),
			COALESCE(st.sosialisasi_row_count,$5), COALESCE(st.training_10_location,''),
			COALESCE(st.training_10_row_count,$6), COALESCE(st.training_100_location,''),
			COALESCE(st.training_100_row_count,$7), COALESCE(st.servis_1_start::text,''),
			COALESCE(st.servis_1_end::text,''), COALESCE(st.servis_2_start::text,''),
			COALESCE(st.servis_2_end::text,''),
			COALESCE(st.updated_at, now())
		FROM program_schedules ps
		LEFT JOIN bast_schedule_settings st ON st.schedule_id = ps.id
		WHERE ps.id=$1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs, defaultRakordaRowCount, defaultSosialisasiRowCount, defaultTraining10RowCount, defaultTraining100RowCount).Scan(
		&settings.ScheduleID, &settings.HandoverLocation, &settings.ConsultantCompanyName,
		&settings.AgricultureOfficeName, &settings.AgricultureOfficeNIP,
		&settings.InstallerName, &settings.SupervisorName,
		&settings.PertaminaRepName, &settings.RakordaLocation,
		&settings.RakordaRowCount, &settings.SosialisasiLocation,
		&settings.SosialisasiRowCount, &settings.Training10Location,
		&settings.Training10RowCount, &settings.Training100Location,
		&settings.Training100RowCount, &settings.Servis1Start,
		&settings.Servis1End, &settings.Servis2Start,
		&settings.Servis2End, &settings.UpdatedAt,
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
		INSERT INTO bast_schedule_settings(schedule_id,handover_location,consultant_company_name,agriculture_office_name,agriculture_office_nip,installer_name,supervisor_name,pertamina_rep_name,rakorda_location,rakorda_row_count,sosialisasi_location,sosialisasi_row_count,training_10_location,training_10_row_count,training_100_location,training_100_row_count,servis_1_start,servis_1_end,servis_2_start,servis_2_end)
		VALUES($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13,$14,$15,$16,NULLIF($17,'')::date,NULLIF($18,'')::date,NULLIF($19,'')::date,NULLIF($20,'')::date)
		ON CONFLICT(schedule_id) DO UPDATE SET
			handover_location=EXCLUDED.handover_location,
			consultant_company_name=EXCLUDED.consultant_company_name,
			agriculture_office_name=EXCLUDED.agriculture_office_name,
			agriculture_office_nip=EXCLUDED.agriculture_office_nip,
			installer_name=EXCLUDED.installer_name,
			supervisor_name=EXCLUDED.supervisor_name,
			pertamina_rep_name=EXCLUDED.pertamina_rep_name,
			rakorda_location=EXCLUDED.rakorda_location,
			rakorda_row_count=EXCLUDED.rakorda_row_count,
			sosialisasi_location=EXCLUDED.sosialisasi_location,
			sosialisasi_row_count=EXCLUDED.sosialisasi_row_count,
			training_10_location=EXCLUDED.training_10_location,
			training_10_row_count=EXCLUDED.training_10_row_count,
			training_100_location=EXCLUDED.training_100_location,
			training_100_row_count=EXCLUDED.training_100_row_count,
			servis_1_start=EXCLUDED.servis_1_start,
			servis_1_end=EXCLUDED.servis_1_end,
			servis_2_start=EXCLUDED.servis_2_start,
			servis_2_end=EXCLUDED.servis_2_end,
			updated_at=now()
		RETURNING schedule_id::text,COALESCE(handover_location,''),COALESCE(consultant_company_name,''),COALESCE(agriculture_office_name,''),COALESCE(agriculture_office_nip,''),COALESCE(installer_name,''),COALESCE(supervisor_name,''),COALESCE(pertamina_rep_name,''),COALESCE(rakorda_location,''),rakorda_row_count,sosialisasi_location,sosialisasi_row_count,training_10_location,training_10_row_count,training_100_location,training_100_row_count,COALESCE(servis_1_start::text,''),COALESCE(servis_1_end::text,''),COALESCE(servis_2_start::text,''),COALESCE(servis_2_end::text,''),updated_at
	`, input.ScheduleID, input.HandoverLocation, input.ConsultantCompanyName, input.AgricultureOfficeName, input.AgricultureOfficeNIP, input.InstallerName, input.SupervisorName, input.PertaminaRepName, input.RakordaLocation, input.RakordaRowCount, input.SosialisasiLocation, input.SosialisasiRowCount, input.Training10Location, input.Training10RowCount, input.Training100Location, input.Training100RowCount, input.Servis1Start, input.Servis1End, input.Servis2Start, input.Servis2End).Scan(
		&settings.ScheduleID, &settings.HandoverLocation, &settings.ConsultantCompanyName,
		&settings.AgricultureOfficeName, &settings.AgricultureOfficeNIP,
		&settings.InstallerName, &settings.SupervisorName,
		&settings.PertaminaRepName, &settings.RakordaLocation,
		&settings.RakordaRowCount, &settings.SosialisasiLocation,
		&settings.SosialisasiRowCount, &settings.Training10Location,
		&settings.Training10RowCount, &settings.Training100Location,
		&settings.Training100RowCount, &settings.Servis1Start,
		&settings.Servis1End, &settings.Servis2Start,
		&settings.Servis2End, &settings.UpdatedAt,
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
