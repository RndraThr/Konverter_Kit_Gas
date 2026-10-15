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

// previewSourceData is the raw (possibly-empty) values pulled live from
// sibling domains' tables via direct SQL — finalreport never imports the
// bast/distribution/activities Go packages, the same way bast's own
// repository reads program_schedules directly instead of importing
// internal/programs.
type previewSourceData struct {
	regencyName, provinceName                                       string
	fiscalYear                                                      int
	agricultureOfficeName, consultantCompanyName, consultantDefault string
	handoverLocation                                                string
	rakordaDate, sosialisasiDate                                    string
	distributionStart, distributionEnd                              string
	distributionTotal                                               int
}

func (r *Repository) GetPreviewSourceData(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (previewSourceData, error) {
	var data previewSourceData
	err := r.pool.QueryRow(ctx, `
		SELECT reg.name, reg.province_name, p.fiscal_year
		FROM regencies reg, programs p
		WHERE reg.id=$1::uuid AND p.id=$2::uuid AND ($3 OR reg.id::text=ANY($4))
	`, regencyID, programID, scope.Unrestricted, scope.RegencyIDs).Scan(&data.regencyName, &data.provinceName, &data.fiscalYear)
	if errors.Is(err, pgx.ErrNoRows) {
		return previewSourceData{}, ErrNotFound
	}
	if err != nil {
		return previewSourceData{}, fmt.Errorf("get regency/program: %w", err)
	}

	var scheduleID string
	err = r.pool.QueryRow(ctx, `
		SELECT id::text FROM program_schedules
		WHERE program_id=$1::uuid AND regency_id=$2::uuid
		ORDER BY (status='active') DESC, start_date DESC LIMIT 1
	`, programID, regencyID).Scan(&scheduleID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return previewSourceData{}, fmt.Errorf("resolve schedule: %w", err)
	}
	if scheduleID == "" {
		return data, nil // no schedule yet — every remaining field stays blank
	}

	_ = r.pool.QueryRow(ctx, `
		SELECT COALESCE(agriculture_office_name,''), COALESCE(consultant_company_name,''), COALESCE(handover_location,'')
		FROM bast_schedule_settings WHERE schedule_id=$1::uuid
	`, scheduleID).Scan(&data.agricultureOfficeName, &data.consultantCompanyName, &data.handoverLocation)

	_ = r.pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_settings WHERE key='organization_name'`).Scan(&data.consultantDefault)
	if data.consultantCompanyName == "" {
		data.consultantCompanyName = data.consultantDefault
	}

	// Rakorda and Sosialisasi share one table (bast_rakorda_uploads), told
	// apart by document_kind (migration 00043_bast_sosialisasi.sql) — not
	// two separate tables.
	_ = r.pool.QueryRow(ctx, `SELECT to_char(min(event_date),'DD Month YYYY') FROM bast_rakorda_uploads WHERE schedule_id=$1::uuid AND document_kind='rakorda'`, scheduleID).Scan(&data.rakordaDate)
	_ = r.pool.QueryRow(ctx, `SELECT to_char(min(event_date),'DD Month YYYY') FROM bast_rakorda_uploads WHERE schedule_id=$1::uuid AND document_kind='sosialisasi'`, scheduleID).Scan(&data.sosialisasiDate)

	_ = r.pool.QueryRow(ctx, `
		SELECT to_char(min(completed_at),'DD Month YYYY'), to_char(max(completed_at),'DD Month YYYY'), count(*)
		FROM distribution_slots WHERE schedule_id=$1::uuid AND status='completed'
	`, scheduleID).Scan(&data.distributionStart, &data.distributionEnd, &data.distributionTotal)

	return data, nil
}

const documentColumns = `id::text, program_id::text, regency_id::text, version, filename, checksum, storage_key, status, finalized_at`

func (r *Repository) ListDocuments(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+documentColumns+` FROM final_report_documents
		WHERE program_id=$1 AND regency_id=$2 AND ($3 OR regency_id::text=ANY($4))
		ORDER BY version DESC
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list final report documents: %w", err)
	}
	defer rows.Close()
	items := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.ProgramID, &d.RegencyID, &d.Version, &d.Filename, &d.Checksum, &d.StorageKey, &d.Status, &d.FinalizedAt); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// InsertDocumentVersion locks on (programID, regencyID) via an advisory
// transaction lock — same pattern as bast's ActivateAggregate — so two
// concurrent "Generate PPTX" clicks produce sequential versions, never a
// collision. Returns the previous active document's storage_key (empty if
// this is the first version) so the caller can delete the superseded file
// from Drive.
func (r *Repository) InsertDocumentVersion(ctx context.Context, actor auth.Principal, programID, regencyID, filename, checksum, storageKey string, scope auth.RegencyScope) (Document, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Document{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, programID+":"+regencyID); err != nil {
		return Document{}, "", err
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
			return Document{}, "", ErrRegencyOutOfScope
		}
	}

	var previousKey string
	err = tx.QueryRow(ctx, `SELECT storage_key FROM final_report_documents WHERE program_id=$1 AND regency_id=$2 AND status='active'`, programID, regencyID).Scan(&previousKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Document{}, "", fmt.Errorf("find previous active document: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE final_report_documents SET status='superseded' WHERE program_id=$1 AND regency_id=$2 AND status='active'`, programID, regencyID); err != nil {
		return Document{}, "", fmt.Errorf("supersede previous document: %w", err)
	}

	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM final_report_documents WHERE program_id=$1 AND regency_id=$2`, programID, regencyID).Scan(&version); err != nil {
		return Document{}, "", fmt.Errorf("next version: %w", err)
	}

	var d Document
	err = tx.QueryRow(ctx, `
		INSERT INTO final_report_documents (program_id, regency_id, version, filename, checksum, storage_key, status, finalized_by)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,'active',NULLIF($7,'')::uuid)
		RETURNING `+documentColumns,
		programID, regencyID, version, filename, checksum, storageKey, actor.UserID,
	).Scan(&d.ID, &d.ProgramID, &d.RegencyID, &d.Version, &d.Filename, &d.Checksum, &d.StorageKey, &d.Status, &d.FinalizedAt)
	if err != nil {
		return Document{}, "", fmt.Errorf("insert document version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, "", err
	}
	return d, previousKey, nil
}
