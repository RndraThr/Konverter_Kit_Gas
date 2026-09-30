package programs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) ListRegencies(ctx context.Context, scope auth.RegencyScope) ([]Regency, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, province_name, name, document_code, is_active, COALESCE(notes, ''), created_at, updated_at
		FROM regencies
		WHERE ($1 OR id::text = ANY($2))
		ORDER BY province_name, name
	`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list regencies: %w", err)
	}
	defer rows.Close()
	items := []Regency{}
	for rows.Next() {
		var item Regency
		if err := rows.Scan(&item.ID, &item.ProvinceName, &item.Name, &item.DocumentCode, &item.IsActive, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan regency: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SaveRegency(ctx context.Context, actor auth.Principal, input RegencyInput, meta auth.ClientMeta) (Regency, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Regency{}, fmt.Errorf("begin save regency: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	if input.ID == "" {
		err = tx.QueryRow(ctx, `INSERT INTO regencies (province_name, name, document_code, is_active, notes) VALUES ($1,$2,$3,$4,NULLIF($5,'')) RETURNING id::text`, input.ProvinceName, input.Name, input.DocumentCode, input.IsActive, input.Notes).Scan(&id)
	} else {
		id = input.ID
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE regencies SET province_name=$2,name=$3,document_code=$4,is_active=$5,notes=NULLIF($6,''),updated_at=now() WHERE id=$1`, id, input.ProvinceName, input.Name, input.DocumentCode, input.IsActive, input.Notes)
		if err == nil && tag.RowsAffected() == 0 {
			return Regency{}, ErrNotFound
		}
	}
	if isUniqueViolation(err) {
		return Regency{}, ErrDocumentCodeInUse
	}
	if err != nil {
		return Regency{}, fmt.Errorf("save regency: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "regency.saved", "regency", id, map[string]any{"document_code": input.DocumentCode, "name": input.Name}); err != nil {
		return Regency{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Regency{}, fmt.Errorf("commit save regency: %w", err)
	}
	return r.regencyByID(ctx, id)
}

func (r *Repository) ListPrograms(ctx context.Context) ([]Program, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,code,name,program_type,fiscal_year,status,COALESCE(notes,''),created_at,updated_at FROM programs ORDER BY fiscal_year DESC,name`)
	if err != nil {
		return nil, fmt.Errorf("list programs: %w", err)
	}
	defer rows.Close()
	items := []Program{}
	for rows.Next() {
		var item Program
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.ProgramType, &item.FiscalYear, &item.Status, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SaveProgram(ctx context.Context, actor auth.Principal, input ProgramInput, meta auth.ClientMeta) (Program, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Program{}, fmt.Errorf("begin save program: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id := input.ID
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO programs (code,name,program_type,fiscal_year,status,notes) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')) RETURNING id::text`, input.Code, input.Name, input.ProgramType, input.FiscalYear, input.Status, input.Notes).Scan(&id)
	} else {
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE programs SET code=$2,name=$3,program_type=$4,fiscal_year=$5,status=$6,notes=NULLIF($7,''),updated_at=now() WHERE id=$1`, id, input.Code, input.Name, input.ProgramType, input.FiscalYear, input.Status, input.Notes)
		if err == nil && tag.RowsAffected() == 0 {
			return Program{}, ErrNotFound
		}
	}
	if isUniqueViolation(err) {
		return Program{}, ErrCodeInUse
	}
	if err != nil {
		return Program{}, fmt.Errorf("save program: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO program_zones (program_id, code, name, sort_order, is_placeholder)
		VALUES ($1, 'UNASSIGNED', 'ZONA BELUM DIATUR', 0, true)
		ON CONFLICT (program_id, code) DO NOTHING
	`, id); err != nil {
		return Program{}, fmt.Errorf("ensure program placeholder zone: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "program.saved", "program", id, map[string]any{"code": input.Code, "program_type": input.ProgramType}); err != nil {
		return Program{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Program{}, fmt.Errorf("commit save program: %w", err)
	}
	return r.programByID(ctx, id)
}

func (r *Repository) ListPackageTemplates(ctx context.Context) ([]PackageTemplate, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,template_code,version,name,program_type,values_json,status,published_at,created_at,updated_at FROM package_template_versions ORDER BY template_code,version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list package templates: %w", err)
	}
	defer rows.Close()
	items := []PackageTemplate{}
	for rows.Next() {
		item, err := scanPackageTemplate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SavePackageTemplate(ctx context.Context, actor auth.Principal, input PackageTemplateInput, meta auth.ClientMeta) (PackageTemplate, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PackageTemplate{}, fmt.Errorf("begin save package template: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	values, err := json.Marshal(input.Values)
	if err != nil {
		return PackageTemplate{}, ErrInvalidInput
	}
	id := input.ID
	version := 1
	status := input.Status
	if id != "" {
		var existingStatus, existingCode string
		if err := tx.QueryRow(ctx, `SELECT status,template_code FROM package_template_versions WHERE id=$1 FOR UPDATE`, id).Scan(&existingStatus, &existingCode); errors.Is(err, pgx.ErrNoRows) {
			return PackageTemplate{}, ErrNotFound
		} else if err != nil {
			return PackageTemplate{}, fmt.Errorf("lock package template: %w", err)
		}
		if existingCode != input.TemplateCode {
			return PackageTemplate{}, ErrCodeInUse
		}
		if existingStatus == "draft" {
			_, err = tx.Exec(ctx, `UPDATE package_template_versions SET name=$2,program_type=$3,values_json=$4,status=$5,published_at=CASE WHEN $5='published' THEN COALESCE(published_at,now()) ELSE NULL END,updated_at=now() WHERE id=$1`, id, input.Name, input.ProgramType, values, status)
		} else {
			status = "draft"
			err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM package_template_versions WHERE template_code=$1`, input.TemplateCode).Scan(&version)
			if err == nil {
				err = tx.QueryRow(ctx, `INSERT INTO package_template_versions (template_code,version,name,program_type,values_json,status) VALUES ($1,$2,$3,$4,$5,'draft') RETURNING id::text`, input.TemplateCode, version, input.Name, input.ProgramType, values).Scan(&id)
			}
		}
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO package_template_versions (template_code,version,name,program_type,values_json,status,published_at) VALUES ($1,1,$2,$3,$4,$5,CASE WHEN $5='published' THEN now() END) RETURNING id::text`, input.TemplateCode, input.Name, input.ProgramType, values, status).Scan(&id)
	}
	if isUniqueViolation(err) {
		return PackageTemplate{}, ErrCodeInUse
	}
	if err != nil {
		return PackageTemplate{}, fmt.Errorf("save package template: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "package_template.saved", "package_template", id, map[string]any{"template_code": input.TemplateCode, "version": version, "status": status}); err != nil {
		return PackageTemplate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PackageTemplate{}, fmt.Errorf("commit package template: %w", err)
	}
	return r.packageTemplateByID(ctx, id)
}

func (r *Repository) ListDocumentationTemplates(ctx context.Context) ([]DocumentationTemplate, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,template_code,version,name,program_type,status,published_at,created_at,updated_at FROM documentation_template_versions ORDER BY template_code,version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list documentation templates: %w", err)
	}
	defer rows.Close()
	items := []DocumentationTemplate{}
	for rows.Next() {
		var item DocumentationTemplate
		if err := rows.Scan(&item.ID, &item.TemplateCode, &item.Version, &item.Name, &item.ProgramType, &item.Status, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan documentation template: %w", err)
		}
		item.Slots, err = r.documentationSlots(ctx, r.pool, item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SaveDocumentationTemplate(ctx context.Context, actor auth.Principal, input DocumentationTemplateInput, meta auth.ClientMeta) (DocumentationTemplate, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DocumentationTemplate{}, fmt.Errorf("begin save documentation template: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id := input.ID
	version := 1
	status := input.Status
	if id != "" {
		var existingStatus, existingCode string
		if err := tx.QueryRow(ctx, `SELECT status,template_code FROM documentation_template_versions WHERE id=$1 FOR UPDATE`, id).Scan(&existingStatus, &existingCode); errors.Is(err, pgx.ErrNoRows) {
			return DocumentationTemplate{}, ErrNotFound
		} else if err != nil {
			return DocumentationTemplate{}, fmt.Errorf("lock documentation template: %w", err)
		}
		if existingCode != input.TemplateCode {
			return DocumentationTemplate{}, ErrCodeInUse
		}
		if existingStatus == "draft" {
			_, err = tx.Exec(ctx, `UPDATE documentation_template_versions SET name=$2,program_type=$3,status=$4,published_at=CASE WHEN $4='published' THEN COALESCE(published_at,now()) ELSE NULL END,updated_at=now() WHERE id=$1`, id, input.Name, input.ProgramType, status)
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM documentation_template_slots WHERE template_version_id=$1`, id)
			}
		} else {
			status = "draft"
			err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM documentation_template_versions WHERE template_code=$1`, input.TemplateCode).Scan(&version)
			if err == nil {
				err = tx.QueryRow(ctx, `INSERT INTO documentation_template_versions (template_code,version,name,program_type,status) VALUES ($1,$2,$3,$4,'draft') RETURNING id::text`, input.TemplateCode, version, input.Name, input.ProgramType).Scan(&id)
			}
		}
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO documentation_template_versions (template_code,version,name,program_type,status,published_at) VALUES ($1,1,$2,$3,$4,CASE WHEN $4='published' THEN now() END) RETURNING id::text`, input.TemplateCode, input.Name, input.ProgramType, status).Scan(&id)
	}
	if isUniqueViolation(err) {
		return DocumentationTemplate{}, ErrCodeInUse
	}
	if err != nil {
		return DocumentationTemplate{}, fmt.Errorf("save documentation template: %w", err)
	}
	for _, slot := range input.Slots {
		_, err = tx.Exec(ctx, `INSERT INTO documentation_template_slots (template_version_id,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,instructions,sort_order) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12)`, id, slot.SlotCode, slot.Label, slot.Stage, slot.IsRequired, slot.MinFiles, slot.MaxFiles, slot.InputSource, slot.RequireLocation, slot.RequireCapturedAt, slot.Instructions, slot.SortOrder)
		if err != nil {
			return DocumentationTemplate{}, fmt.Errorf("save documentation slot: %w", err)
		}
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "documentation_template.saved", "documentation_template", id, map[string]any{"template_code": input.TemplateCode, "version": version, "status": status, "slot_count": len(input.Slots)}); err != nil {
		return DocumentationTemplate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentationTemplate{}, fmt.Errorf("commit documentation template: %w", err)
	}
	return r.documentationTemplateByID(ctx, id)
}

func (r *Repository) ListSchedules(ctx context.Context, scope auth.RegencyScope) ([]Schedule, error) {
	rows, err := r.pool.Query(ctx, scheduleSelect+` WHERE ($1 OR s.regency_id::text = ANY($2)) ORDER BY s.start_date DESC,s.name`, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	items := []Schedule{}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SaveSchedule(ctx context.Context, actor auth.Principal, input ScheduleInput, meta auth.ClientMeta) (Schedule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Schedule{}, fmt.Errorf("begin save schedule: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	policy, err := json.Marshal(input.ReceiptPolicy)
	if err != nil {
		return Schedule{}, ErrInvalidInput
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO program_regency_assignments (program_id, regency_id, zone_id)
		SELECT $1::uuid, $2::uuid, z.id
		FROM program_zones z
		WHERE z.program_id=$1 AND z.is_placeholder=true
		ON CONFLICT (program_id, regency_id) DO NOTHING
	`, input.ProgramID, input.RegencyID); err != nil {
		if isForeignKeyViolation(err) {
			return Schedule{}, ErrNotFound
		}
		return Schedule{}, fmt.Errorf("ensure schedule regency assignment: %w", err)
	}
	id := input.ID
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO program_schedules (program_id,regency_id,package_template_version_id,documentation_template_version_id,name,start_date,end_date,status,distribution_number_padding,receipt_policy_json,notes,supervisor_name,slot_quota) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13 FROM programs p JOIN package_template_versions pt ON pt.id=$3 JOIN documentation_template_versions dt ON dt.id=$4 WHERE p.id=$1 AND p.program_type=pt.program_type AND p.program_type=dt.program_type RETURNING id::text`, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName, input.SlotQuota).Scan(&id)
	} else {
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE program_schedules s SET program_id=$2,regency_id=$3,package_template_version_id=$4,documentation_template_version_id=$5,name=$6,start_date=$7,end_date=$8,status=$9,distribution_number_padding=$10,receipt_policy_json=$11,notes=NULLIF($12,''),supervisor_name=NULLIF($13,''),slot_quota=$14,updated_at=now() WHERE s.id=$1 AND EXISTS (SELECT 1 FROM programs p JOIN package_template_versions pt ON pt.id=$4 JOIN documentation_template_versions dt ON dt.id=$5 WHERE p.id=$2 AND p.program_type=pt.program_type AND p.program_type=dt.program_type)`, id, input.ProgramID, input.RegencyID, input.PackageTemplateVersionID, input.DocumentationTemplateVersionID, input.Name, input.StartDate, input.EndDate, input.Status, input.DistributionNumberPadding, policy, input.Notes, input.SupervisorName, input.SlotQuota)
		if err == nil && tag.RowsAffected() == 0 {
			return Schedule{}, ErrNotFound
		}
	}
	if errors.Is(err, pgx.ErrNoRows) || isForeignKeyViolation(err) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("save schedule: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "schedule.saved", "program_schedule", id, map[string]any{"name": input.Name, "status": input.Status}); err != nil {
		return Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Schedule{}, fmt.Errorf("commit schedule: %w", err)
	}
	return r.scheduleByID(ctx, id)
}

func (r *Repository) ListZones(ctx context.Context, programID string, scope auth.RegencyScope) ([]ProgramZone, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, program_id::text, code, name, sort_order, is_placeholder, created_at, updated_at
		FROM program_zones
		WHERE program_id = $1
		ORDER BY sort_order, name
	`, programID)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	items := []ProgramZone{}
	for rows.Next() {
		var item ProgramZone
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.Code, &item.Name, &item.SortOrder, &item.IsPlaceholder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		regencies, err := r.zoneRegencies(ctx, r.pool, items[index].ID, scope)
		if err != nil {
			return nil, err
		}
		items[index].Regencies = regencies
	}
	return items, nil
}

func (r *Repository) zoneRegencies(ctx context.Context, db queryer, zoneID string, scope auth.RegencyScope) ([]Regency, error) {
	rows, err := db.Query(ctx, `
		SELECT r.id::text, r.province_name, r.name, r.document_code, r.is_active, COALESCE(r.notes,''), r.created_at, r.updated_at
		FROM program_regency_assignments a
		JOIN regencies r ON r.id = a.regency_id
		WHERE a.zone_id = $1 AND ($2 OR a.regency_id::text = ANY($3))
		ORDER BY r.province_name, r.name
	`, zoneID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list zone regencies: %w", err)
	}
	defer rows.Close()
	items := []Regency{}
	for rows.Next() {
		var item Regency
		if err := rows.Scan(&item.ID, &item.ProvinceName, &item.Name, &item.DocumentCode, &item.IsActive, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan zone regency: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SaveZone(ctx context.Context, actor auth.Principal, input ZoneInput, meta auth.ClientMeta) (ProgramZone, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProgramZone{}, fmt.Errorf("begin save zone: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id := input.ID
	if id == "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO program_zones (program_id, code, name, sort_order, is_placeholder)
			VALUES ($1,$2,$3,$4,false)
			RETURNING id::text
		`, input.ProgramID, input.Code, input.Name, input.SortOrder).Scan(&id)
	} else {
		var isPlaceholder bool
		err = tx.QueryRow(ctx, `
			SELECT is_placeholder FROM program_zones
			WHERE id=$1 AND program_id=$2
			FOR UPDATE
		`, id, input.ProgramID).Scan(&isPlaceholder)
		if errors.Is(err, pgx.ErrNoRows) {
			return ProgramZone{}, ErrNotFound
		}
		if err != nil {
			return ProgramZone{}, fmt.Errorf("lock zone: %w", err)
		}
		if isPlaceholder {
			return ProgramZone{}, ErrZonePlaceholderImmutable
		}
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `
			UPDATE program_zones
			SET code=$3, name=$4, sort_order=$5, updated_at=now()
			WHERE id=$1 AND program_id=$2
		`, id, input.ProgramID, input.Code, input.Name, input.SortOrder)
		if err == nil && tag.RowsAffected() == 0 {
			return ProgramZone{}, ErrNotFound
		}
	}
	if isUniqueViolation(err) {
		return ProgramZone{}, ErrCodeInUse
	}
	if isForeignKeyViolation(err) {
		return ProgramZone{}, ErrNotFound
	}
	if err != nil {
		return ProgramZone{}, fmt.Errorf("save zone: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "zone.saved", "program_zone", id, map[string]any{"program_id": input.ProgramID, "code": input.Code}); err != nil {
		return ProgramZone{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProgramZone{}, fmt.Errorf("commit save zone: %w", err)
	}
	return r.zoneByID(ctx, id, auth.RegencyScope{Unrestricted: true})
}

func (r *Repository) zoneByID(ctx context.Context, id string, scope auth.RegencyScope) (ProgramZone, error) {
	var item ProgramZone
	err := r.pool.QueryRow(ctx, `SELECT id::text, program_id::text, code, name, sort_order, is_placeholder, created_at, updated_at FROM program_zones WHERE id = $1`, id).
		Scan(&item.ID, &item.ProgramID, &item.Code, &item.Name, &item.SortOrder, &item.IsPlaceholder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProgramZone{}, ErrNotFound
	}
	if err != nil {
		return ProgramZone{}, fmt.Errorf("zone by id: %w", err)
	}
	item.Regencies, err = r.zoneRegencies(ctx, r.pool, item.ID, scope)
	return item, err
}

func (r *Repository) AssignRegency(ctx context.Context, actor auth.Principal, input RegencyAssignmentInput, scope auth.RegencyScope, meta auth.ClientMeta) (ProgramZone, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProgramZone{}, fmt.Errorf("begin assign regency: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var zoneID string
	err = tx.QueryRow(ctx, `
		INSERT INTO program_regency_assignments (program_id, regency_id, zone_id)
		SELECT $1::uuid, $2::uuid, $3::uuid
		WHERE ($4 OR $2::text = ANY($5))
		ON CONFLICT (program_id, regency_id) DO UPDATE
			SET zone_id = EXCLUDED.zone_id, updated_at = now()
		RETURNING zone_id::text
	`, input.ProgramID, input.RegencyID, input.ZoneID, scope.Unrestricted, scope.RegencyIDs).Scan(&zoneID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProgramZone{}, ErrNotFound
	}
	if isForeignKeyViolation(err) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && strings.Contains(pgErr.ConstraintName, "zone") {
			return ProgramZone{}, ErrZoneProgramMismatch
		}
		return ProgramZone{}, ErrNotFound
	}
	if err != nil {
		return ProgramZone{}, fmt.Errorf("assign regency: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "regency.assigned", "program_regency_assignment", input.ProgramID+":"+input.RegencyID, map[string]any{"program_id": input.ProgramID, "regency_id": input.RegencyID, "zone_id": input.ZoneID}); err != nil {
		return ProgramZone{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProgramZone{}, fmt.Errorf("commit assign regency: %w", err)
	}
	return r.zoneByID(ctx, zoneID, scope)
}

func (r *Repository) ResolveStorageContext(ctx context.Context, programID string, regencyID string, scope auth.RegencyScope) (StorageContext, error) {
	var item StorageContext
	var isPlaceholder bool
	err := r.pool.QueryRow(ctx, `
		SELECT p.id::text, p.program_type, z.id::text, z.name, z.is_placeholder, r.id::text, r.name
		FROM program_regency_assignments a
		JOIN programs p ON p.id = a.program_id
		JOIN program_zones z ON z.id = a.zone_id
		JOIN regencies r ON r.id = a.regency_id
		WHERE a.program_id = $1 AND a.regency_id = $2 AND ($3 OR a.regency_id::text = ANY($4))
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs).
		Scan(&item.ProgramID, &item.ProgramType, &item.ZoneID, &item.ZoneName, &isPlaceholder, &item.RegencyID, &item.RegencyName)
	if errors.Is(err, pgx.ErrNoRows) {
		return StorageContext{}, ErrNotFound
	}
	if err != nil {
		return StorageContext{}, fmt.Errorf("resolve storage context: %w", err)
	}
	if isPlaceholder {
		return StorageContext{}, ErrZoneNotConfigured
	}
	return item, nil
}

func (r *Repository) regencyByID(ctx context.Context, id string) (Regency, error) {
	var item Regency
	err := r.pool.QueryRow(ctx, `SELECT id::text,province_name,name,document_code,is_active,COALESCE(notes,''),created_at,updated_at FROM regencies WHERE id=$1`, id).Scan(&item.ID, &item.ProvinceName, &item.Name, &item.DocumentCode, &item.IsActive, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Regency{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) programByID(ctx context.Context, id string) (Program, error) {
	var item Program
	err := r.pool.QueryRow(ctx, `SELECT id::text,code,name,program_type,fiscal_year,status,COALESCE(notes,''),created_at,updated_at FROM programs WHERE id=$1`, id).Scan(&item.ID, &item.Code, &item.Name, &item.ProgramType, &item.FiscalYear, &item.Status, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Program{}, ErrNotFound
	}
	return item, err
}

type rowScanner interface{ Scan(...any) error }

func scanPackageTemplate(row rowScanner) (PackageTemplate, error) {
	var item PackageTemplate
	var values []byte
	if err := row.Scan(&item.ID, &item.TemplateCode, &item.Version, &item.Name, &item.ProgramType, &values, &item.Status, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return PackageTemplate{}, fmt.Errorf("scan package template: %w", err)
	}
	if err := json.Unmarshal(values, &item.Values); err != nil {
		return PackageTemplate{}, fmt.Errorf("decode package template: %w", err)
	}
	return item, nil
}

func (r *Repository) packageTemplateByID(ctx context.Context, id string) (PackageTemplate, error) {
	item, err := scanPackageTemplate(r.pool.QueryRow(ctx, `SELECT id::text,template_code,version,name,program_type,values_json,status,published_at,created_at,updated_at FROM package_template_versions WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return PackageTemplate{}, ErrNotFound
	}
	return item, err
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *Repository) documentationSlots(ctx context.Context, db queryer, templateID string) ([]DocumentationTemplateSlot, error) {
	rows, err := db.Query(ctx, `SELECT id::text,slot_code,label,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,COALESCE(instructions,''),sort_order FROM documentation_template_slots WHERE template_version_id=$1 ORDER BY sort_order,slot_code`, templateID)
	if err != nil {
		return nil, fmt.Errorf("list documentation slots: %w", err)
	}
	defer rows.Close()
	items := []DocumentationTemplateSlot{}
	for rows.Next() {
		var item DocumentationTemplateSlot
		if err := rows.Scan(&item.ID, &item.SlotCode, &item.Label, &item.Stage, &item.IsRequired, &item.MinFiles, &item.MaxFiles, &item.InputSource, &item.RequireLocation, &item.RequireCapturedAt, &item.Instructions, &item.SortOrder); err != nil {
			return nil, fmt.Errorf("scan documentation slot: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) documentationTemplateByID(ctx context.Context, id string) (DocumentationTemplate, error) {
	var item DocumentationTemplate
	err := r.pool.QueryRow(ctx, `SELECT id::text,template_code,version,name,program_type,status,published_at,created_at,updated_at FROM documentation_template_versions WHERE id=$1`, id).Scan(&item.ID, &item.TemplateCode, &item.Version, &item.Name, &item.ProgramType, &item.Status, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentationTemplate{}, ErrNotFound
	}
	if err != nil {
		return DocumentationTemplate{}, err
	}
	item.Slots, err = r.documentationSlots(ctx, r.pool, id)
	return item, err
}

const scheduleSelect = `
SELECT s.id::text,s.program_id::text,s.regency_id::text,s.package_template_version_id::text,s.documentation_template_version_id::text,s.name,s.start_date,s.end_date,s.status,s.distribution_number_padding,s.slot_quota,s.receipt_policy_json,COALESCE(s.notes,''),COALESCE(s.supervisor_name,''),s.created_at,s.updated_at,
p.id::text,p.code,p.name,p.program_type,p.fiscal_year,p.status,COALESCE(p.notes,''),p.created_at,p.updated_at,
r.id::text,r.province_name,r.name,r.document_code,r.is_active,COALESCE(r.notes,''),r.created_at,r.updated_at
FROM program_schedules s JOIN programs p ON p.id=s.program_id JOIN regencies r ON r.id=s.regency_id`

func scanSchedule(row rowScanner) (Schedule, error) {
	var item Schedule
	var policy []byte
	item.Program = &Program{}
	item.Regency = &Regency{}
	err := row.Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.PackageTemplateVersionID, &item.DocumentationTemplateVersionID, &item.Name, &item.StartDate, &item.EndDate, &item.Status, &item.DistributionNumberPadding, &item.SlotQuota, &policy, &item.Notes, &item.SupervisorName, &item.CreatedAt, &item.UpdatedAt,
		&item.Program.ID, &item.Program.Code, &item.Program.Name, &item.Program.ProgramType, &item.Program.FiscalYear, &item.Program.Status, &item.Program.Notes, &item.Program.CreatedAt, &item.Program.UpdatedAt,
		&item.Regency.ID, &item.Regency.ProvinceName, &item.Regency.Name, &item.Regency.DocumentCode, &item.Regency.IsActive, &item.Regency.Notes, &item.Regency.CreatedAt, &item.Regency.UpdatedAt)
	if err != nil {
		return Schedule{}, fmt.Errorf("scan schedule: %w", err)
	}
	if err := json.Unmarshal(policy, &item.ReceiptPolicy); err != nil {
		return Schedule{}, fmt.Errorf("decode receipt policy: %w", err)
	}
	return item, nil
}

func (r *Repository) scheduleByID(ctx context.Context, id string) (Schedule, error) {
	item, err := scanSchedule(r.pool.QueryRow(ctx, scheduleSelect+` WHERE s.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) GetProgram(ctx context.Context, id string) (Program, error) {
	return r.programByID(ctx, id)
}

func (r *Repository) ListDocumentProfiles(ctx context.Context, programID string) ([]DocumentProfile, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,program_id::text,version,title,subtitle,procurement_description,document_series,status,published_at,created_at,updated_at FROM program_document_profile_versions WHERE program_id=$1 ORDER BY version DESC`, programID)
	if err != nil {
		return nil, fmt.Errorf("list document profiles: %w", err)
	}
	defer rows.Close()
	items := []DocumentProfile{}
	for rows.Next() {
		item, err := scanDocumentProfile(rows)
		if err != nil {
			return nil, err
		}
		item.Logos, err = r.documentLogos(ctx, r.pool, item.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanDocumentProfile(row rowScanner) (DocumentProfile, error) {
	var item DocumentProfile
	err := row.Scan(&item.ID, &item.ProgramID, &item.Version, &item.Title, &item.Subtitle, &item.ProcurementDescription, &item.DocumentSeries, &item.Status, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return DocumentProfile{}, fmt.Errorf("scan document profile: %w", err)
	}
	return item, nil
}

func (r *Repository) GetDocumentProfile(ctx context.Context, id string) (DocumentProfile, error) {
	item, err := scanDocumentProfile(r.pool.QueryRow(ctx, `SELECT id::text,program_id::text,version,title,subtitle,procurement_description,document_series,status,published_at,created_at,updated_at FROM program_document_profile_versions WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentProfile{}, ErrNotFound
	}
	if err != nil {
		return DocumentProfile{}, err
	}
	item.Logos, err = r.documentLogos(ctx, r.pool, item.ID)
	return item, err
}

func (r *Repository) SaveDocumentProfile(ctx context.Context, actor auth.Principal, input DocumentProfileInput, meta auth.ClientMeta) (DocumentProfile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DocumentProfile{}, fmt.Errorf("begin save document profile: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id := input.ID
	version := 1
	if id == "" {
		err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM program_document_profile_versions WHERE program_id=$1`, input.ProgramID).Scan(&version)
		if err == nil {
			err = tx.QueryRow(ctx, `INSERT INTO program_document_profile_versions(program_id,version,title,subtitle,procurement_description,document_series,status) VALUES($1,$2,$3,$4,$5,$6,'draft') RETURNING id::text`, input.ProgramID, version, input.Title, input.Subtitle, input.ProcurementDescription, input.DocumentSeries).Scan(&id)
		}
	} else {
		var status, ownerProgramID string
		err = tx.QueryRow(ctx, `SELECT program_id::text,status FROM program_document_profile_versions WHERE id=$1 FOR UPDATE`, id).Scan(&ownerProgramID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentProfile{}, ErrNotFound
		}
		if err == nil && ownerProgramID != input.ProgramID {
			return DocumentProfile{}, ErrNotFound
		}
		if err == nil && status == "published" {
			err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM program_document_profile_versions WHERE program_id=$1`, input.ProgramID).Scan(&version)
			if err == nil {
				err = tx.QueryRow(ctx, `INSERT INTO program_document_profile_versions(program_id,version,title,subtitle,procurement_description,document_series,status) VALUES($1,$2,$3,$4,$5,$6,'draft') RETURNING id::text`, input.ProgramID, version, input.Title, input.Subtitle, input.ProcurementDescription, input.DocumentSeries).Scan(&id)
			}
		} else if err == nil {
			_, err = tx.Exec(ctx, `UPDATE program_document_profile_versions SET title=$3,subtitle=$4,procurement_description=$5,document_series=$6,updated_at=now() WHERE id=$1 AND program_id=$2`, id, input.ProgramID, input.Title, input.Subtitle, input.ProcurementDescription, input.DocumentSeries)
		}
	}
	if isForeignKeyViolation(err) {
		return DocumentProfile{}, ErrNotFound
	}
	if err != nil {
		return DocumentProfile{}, fmt.Errorf("save document profile: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "document_profile.saved", "program_document_profile", id, map[string]any{"program_id": input.ProgramID, "version": version}); err != nil {
		return DocumentProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentProfile{}, fmt.Errorf("commit document profile: %w", err)
	}
	return r.GetDocumentProfile(ctx, id)
}

func (r *Repository) PublishDocumentProfile(ctx context.Context, actor auth.Principal, programID, profileID string, meta auth.ClientMeta) (DocumentProfile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DocumentProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var visible int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM program_document_logo_assets l JOIN program_document_profile_versions p ON p.id=l.profile_version_id WHERE p.id=$1 AND p.program_id=$2 AND p.status='draft' AND l.is_visible=true`, profileID, programID).Scan(&visible)
	if err != nil {
		return DocumentProfile{}, fmt.Errorf("validate document profile: %w", err)
	}
	if visible == 0 {
		return DocumentProfile{}, ErrDocumentProfileIncomplete
	}
	var id string
	err = tx.QueryRow(ctx, `UPDATE program_document_profile_versions SET status='published',published_at=now(),updated_at=now() WHERE id=$1 AND program_id=$2 AND status='draft' AND btrim(title)<>'' AND btrim(procurement_description)<>'' AND btrim(document_series)<>'' RETURNING id::text`, profileID, programID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentProfile{}, ErrDocumentProfileIncomplete
	}
	if err != nil {
		return DocumentProfile{}, fmt.Errorf("publish document profile: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "document_profile.published", "program_document_profile", id, map[string]any{"program_id": programID}); err != nil {
		return DocumentProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentProfile{}, err
	}
	return r.GetDocumentProfile(ctx, id)
}

func (r *Repository) documentLogos(ctx context.Context, db queryer, profileID string) ([]DocumentLogo, error) {
	rows, err := db.Query(ctx, `SELECT id::text,profile_version_id::text,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm::float8,max_height_mm::float8,is_visible,created_at,updated_at FROM program_document_logo_assets WHERE profile_version_id=$1 ORDER BY sort_order,id`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list document logos: %w", err)
	}
	defer rows.Close()
	items := []DocumentLogo{}
	for rows.Next() {
		var item DocumentLogo
		if err := scanDocumentLogo(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanDocumentLogo(row rowScanner, item *DocumentLogo) error {
	if err := row.Scan(&item.ID, &item.ProfileVersionID, &item.SlotCode, &item.StorageKey, &item.OriginalFilename, &item.MimeType, &item.ByteSize, &item.Checksum, &item.SortOrder, &item.MaxWidthMM, &item.MaxHeightMM, &item.IsVisible, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return fmt.Errorf("scan document logo: %w", err)
	}
	return nil
}

func (r *Repository) SaveDocumentLogo(ctx context.Context, actor auth.Principal, logo DocumentLogo, meta auth.ClientMeta) (DocumentLogo, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DocumentLogo{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM program_document_profile_versions WHERE id=$1 FOR UPDATE`, logo.ProfileVersionID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return DocumentLogo{}, "", ErrNotFound
	} else if err != nil {
		return DocumentLogo{}, "", err
	}
	if status != "draft" {
		return DocumentLogo{}, "", ErrDocumentProfilePublished
	}
	var oldKey string
	_ = tx.QueryRow(ctx, `SELECT storage_key FROM program_document_logo_assets WHERE profile_version_id=$1 AND slot_code=$2`, logo.ProfileVersionID, logo.SlotCode).Scan(&oldKey)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO program_document_logo_assets(profile_version_id,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm,max_height_mm,is_visible) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true) ON CONFLICT(profile_version_id,slot_code) DO UPDATE SET storage_key=excluded.storage_key,original_filename=excluded.original_filename,mime_type=excluded.mime_type,byte_size=excluded.byte_size,checksum=excluded.checksum,sort_order=excluded.sort_order,max_width_mm=excluded.max_width_mm,max_height_mm=excluded.max_height_mm,is_visible=true,updated_at=now() RETURNING id::text`, logo.ProfileVersionID, logo.SlotCode, logo.StorageKey, logo.OriginalFilename, logo.MimeType, logo.ByteSize, logo.Checksum, logo.SortOrder, logo.MaxWidthMM, logo.MaxHeightMM).Scan(&id)
	if err != nil {
		return DocumentLogo{}, "", fmt.Errorf("save document logo: %w", err)
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "document_logo.saved", "program_document_logo", id, map[string]any{"profile_id": logo.ProfileVersionID, "slot_code": logo.SlotCode}); err != nil {
		return DocumentLogo{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentLogo{}, "", err
	}
	item, err := r.GetDocumentLogo(ctx, logo.ProfileVersionID, id)
	return item, oldKey, err
}

func (r *Repository) UpdateDocumentLogo(ctx context.Context, actor auth.Principal, input DocumentLogoUpdateInput, meta auth.ClientMeta) (DocumentLogo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DocumentLogo{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `UPDATE program_document_logo_assets l SET sort_order=$3,max_width_mm=$4,max_height_mm=$5,is_visible=$6,updated_at=now() FROM program_document_profile_versions p WHERE l.id=$1 AND l.profile_version_id=$2 AND p.id=l.profile_version_id AND p.status='draft' RETURNING l.id::text`, input.ID, input.ProfileVersionID, input.SortOrder, input.MaxWidthMM, input.MaxHeightMM, input.IsVisible).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentLogo{}, ErrDocumentProfilePublished
	}
	if err != nil {
		return DocumentLogo{}, err
	}
	if err := recordSetupAudit(ctx, tx, actor, meta, "document_logo.updated", "program_document_logo", id, map[string]any{"profile_id": input.ProfileVersionID}); err != nil {
		return DocumentLogo{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DocumentLogo{}, err
	}
	return r.GetDocumentLogo(ctx, input.ProfileVersionID, id)
}

func (r *Repository) GetDocumentLogo(ctx context.Context, profileID, logoID string) (DocumentLogo, error) {
	var item DocumentLogo
	err := scanDocumentLogo(r.pool.QueryRow(ctx, `SELECT id::text,profile_version_id::text,slot_code,storage_key,original_filename,mime_type,byte_size,checksum,sort_order,max_width_mm::float8,max_height_mm::float8,is_visible,created_at,updated_at FROM program_document_logo_assets WHERE id=$1 AND profile_version_id=$2`, logoID, profileID), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return DocumentLogo{}, ErrNotFound
	}
	return item, err
}

func recordSetupAudit(ctx context.Context, tx pgx.Tx, actor auth.Principal, meta auth.ClientMeta, action, resourceType, resourceID string, metadata map[string]any) error {
	return audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "program_setup." + action, ResourceType: resourceType, ResourceID: resourceID, Metadata: metadata, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
