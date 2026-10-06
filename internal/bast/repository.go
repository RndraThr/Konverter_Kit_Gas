package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) GetSourceContext(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (SourceContext, error) {
	var result SourceContext
	var slotQuota *int
	var zoneName *string
	var placeholder *bool
	err := r.pool.QueryRow(ctx, `
		SELECT p.id::text,r.id::text,p.program_type,r.document_code,r.name,z.name,z.is_placeholder,
			s.slot_quota,
			EXISTS(SELECT 1 FROM program_ba_logo_assets logo WHERE logo.program_id=p.id AND logo.is_visible=true)
		FROM programs p
		JOIN regencies r ON r.id=$2
		JOIN LATERAL (
			SELECT slot_quota FROM program_schedules
			WHERE program_id=p.id AND regency_id=r.id
			ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END,created_at DESC LIMIT 1
		) s ON true
		LEFT JOIN program_regency_assignments a ON a.program_id=p.id AND a.regency_id=r.id
		LEFT JOIN program_zones z ON z.id=a.zone_id
		WHERE p.id=$1 AND ($3 OR r.id::text=ANY($4))
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ProgramID, &result.RegencyID, &result.ProgramType, &result.RegencyCode, &result.RegencyName, &zoneName, &placeholder, &slotQuota, &result.HasActiveLogo)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceContext{}, ErrNotFound
	}
	if err != nil {
		return SourceContext{}, fmt.Errorf("get BA source context: %w", err)
	}
	if slotQuota != nil {
		result.SlotQuota = *slotQuota
	}
	if zoneName != nil {
		result.ZoneName = *zoneName
	}
	if placeholder != nil {
		result.ZonePlaceholder = *placeholder
	}
	result.DocumentSeries = documentSeries
	return result, nil
}

type packageValues struct {
	MachineOptions []struct{ Code, Brand, Type string } `json:"machine_options"`
	HoseOptions    []struct {
		Code           string `json:"code"`
		Brand          string `json:"brand"`
		Spec           string `json:"spec"`
		SuctionBrand   string `json:"suction_brand"`
		SuctionSpec    string `json:"suction_spec"`
		DischargeBrand string `json:"discharge_brand"`
		DischargeSpec  string `json:"discharge_spec"`
	} `json:"hose_options"`
	ConverterOptions []struct{ Code, Brand string } `json:"converter_options"`
	Components       []struct {
		Code, Label string
		Quantity    int
		Unit        string
	} `json:"components"`
}

func (r *Repository) LoadSourceData(ctx context.Context, recipient RecipientDocument, sourceContext SourceContext, scope auth.RegencyScope) (SourceData, error) {
	var source SourceData
	var packageJSON, verificationJSON []byte
	var machineCode, hoseCode, converterCode string
	err := r.pool.QueryRow(ctx, `
		SELECT p.program_type,p.fiscal_year,
			person.full_name,COALESCE(person.nik,''),COALESCE(identifier.display_value,''),COALESCE(person.address,''),COALESCE(person.village,''),COALESCE(person.district,''),r.name,COALESCE(person.phone_number,''),
			COALESCE(ds.machine_option_code,''),COALESCE(ds.machine_serial_number,''),COALESCE(ds.hose_option_code,''),COALESCE(ds.hose_serial_number,''),COALESCE(ds.converter_option_code,''),COALESCE(ds.converter_serial_number,''),
			pt.id::text,pt.values_json,ds.verification_snapshot_json,COALESCE(u.full_name,u.username,''),COALESCE(ps.supervisor_name,'')
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id=ds.schedule_id
		JOIN programs p ON p.id=ps.program_id JOIN regencies r ON r.id=ps.regency_id
		JOIN people person ON person.id=ds.recipient_person_id
		LEFT JOIN person_sector_identifiers identifier ON identifier.person_id=person.id AND identifier.identifier_type='farmer_card'
		JOIN package_template_versions pt ON pt.id=ps.package_template_version_id
		LEFT JOIN users u ON u.id=ds.distributed_by
		WHERE ds.id=$1 AND ps.program_id=$2 AND ps.regency_id=$3 AND ds.status='completed' AND ($4 OR ps.regency_id::text=ANY($5))
	`, recipient.DistributionSlotID, sourceContext.ProgramID, sourceContext.RegencyID, scope.Unrestricted, scope.RegencyIDs).Scan(&source.ProgramType, &source.Render.FiscalYear, &source.Recipient.FullName, &source.Recipient.NIK, &source.Recipient.SectorIdentifier, &source.Recipient.Address, &source.Recipient.Village, &source.Recipient.District, &source.Recipient.Regency, &source.Recipient.PhoneNumber, &machineCode, &source.Equipment.MachineSerial, &hoseCode, &source.Equipment.HoseSerial, &converterCode, &source.Equipment.ConverterSerial, &source.PackageTemplateVersionID, &packageJSON, &verificationJSON, &source.ExecutorName, &source.SupervisorName)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceData{}, ErrNotFound
	}
	if err != nil {
		return SourceData{}, fmt.Errorf("load BA source: %w", err)
	}
	source.DocumentNumber, source.LocalDate = recipient.DocumentNumber, recipient.LocalDate
	source.ProgramID, source.RegencyID = sourceContext.ProgramID, sourceContext.RegencyID
	var values packageValues
	if err := json.Unmarshal(packageJSON, &values); err != nil {
		return SourceData{}, fmt.Errorf("decode BA package: %w", err)
	}
	verifiedEquipment, hasVerification, err := equipmentFromVerificationSnapshot(verificationJSON)
	if err != nil {
		return SourceData{}, fmt.Errorf("decode equipment verification snapshot: %w", err)
	}
	if hasVerification {
		source.Equipment = verifiedEquipment
	} else {
		for _, option := range values.MachineOptions {
			if option.Code == machineCode {
				source.Equipment.MachineBrand, source.Equipment.MachineType = option.Brand, option.Type
				break
			}
		}
		for _, option := range values.HoseOptions {
			if option.Code == hoseCode {
				source.Equipment.HoseBrand = joinHosePair(option.SuctionBrand, option.DischargeBrand, option.Brand)
				source.Equipment.HoseSpec = joinHosePair(option.SuctionSpec, option.DischargeSpec, option.Spec)
				break
			}
		}
		for _, option := range values.ConverterOptions {
			if option.Code == converterCode {
				source.Equipment.ConverterBrand = option.Brand
				break
			}
		}
	}
	for _, component := range values.Components {
		source.Components = append(source.Components, ComponentSnapshot{Code: component.Code, Label: component.Label, Quantity: component.Quantity, Unit: component.Unit, Checked: true})
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text,storage_key,mime_type,sort_order,max_width_mm::float8,max_height_mm::float8 FROM program_ba_logo_assets WHERE program_id=$1 AND is_visible=true ORDER BY sort_order,id`, sourceContext.ProgramID)
	if err != nil {
		return SourceData{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var logo LogoSnapshot
		if err := rows.Scan(&logo.AssetID, &logo.StorageKey, &logo.MimeType, &logo.SortOrder, &logo.MaxWidthMM, &logo.MaxHeightMM); err != nil {
			return SourceData{}, err
		}
		source.Render.Logos = append(source.Render.Logos, logo)
	}
	return source, rows.Err()
}

func joinHosePair(first, second, legacy string) string {
	first, second, legacy = strings.TrimSpace(first), strings.TrimSpace(second), strings.TrimSpace(legacy)
	if first == "" && second == "" {
		return legacy
	}
	if first == "" {
		first = legacy
	}
	if second == "" {
		second = legacy
	}
	return first + "\n" + second
}

func (r *Repository) SaveFinalDocument(ctx context.Context, actor auth.Principal, recipient RecipientDocument, source SourceData, snapshot Snapshot, meta auth.ClientMeta) (IndividualDocument, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return IndividualDocument{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existingID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM bast_individual_documents WHERE distribution_slot_id=$1 AND document_type='individual' AND status='final'`, recipient.DistributionSlotID).Scan(&existingID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return r.getIndividualDocument(ctx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IndividualDocument{}, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return IndividualDocument{}, ErrInvalidInput
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO bast_individual_documents(distribution_slot_id,program_id,regency_id,local_date,slot_number,final_total,document_number,package_template_version_id,snapshot_json,revision,status,finalized_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,(SELECT COALESCE(max(revision),0)+1 FROM bast_individual_documents WHERE distribution_slot_id=$1 AND document_type='individual'),'final',NULLIF($10,'')::uuid) RETURNING id::text`, recipient.DistributionSlotID, source.ProgramID, source.RegencyID, recipient.LocalDate, recipient.SlotNumber, recipient.FinalTotal, recipient.DocumentNumber, source.PackageTemplateVersionID, payload, actor.UserID).Scan(&id)
	if err != nil {
		return IndividualDocument{}, fmt.Errorf("save final BA document: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.individual_finalized", ResourceType: "bast_individual_document", ResourceID: id, Metadata: map[string]any{"distribution_slot_id": recipient.DistributionSlotID, "document_number": recipient.DocumentNumber}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return IndividualDocument{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IndividualDocument{}, err
	}
	return r.getIndividualDocument(ctx, id)
}

func (r *Repository) getIndividualDocument(ctx context.Context, id string) (IndividualDocument, error) {
	var item IndividualDocument
	var payload []byte
	err := r.pool.QueryRow(ctx, `SELECT id::text,distribution_slot_id::text,program_id::text,regency_id::text,document_number,local_date::text,slot_number,final_total,package_template_version_id::text,revision,status,snapshot_json,finalized_at FROM bast_individual_documents WHERE id=$1`, id).Scan(&item.ID, &item.DistributionSlotID, &item.ProgramID, &item.RegencyID, &item.DocumentNumber, &item.LocalDate, &item.SlotNumber, &item.FinalTotal, &item.PackageTemplateVersionID, &item.Revision, &item.Status, &payload, &item.FinalizedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return IndividualDocument{}, ErrNotFound
	}
	if err != nil {
		return IndividualDocument{}, err
	}
	item.Snapshot, err = DecodeSnapshot(payload)
	if err != nil {
		return IndividualDocument{}, err
	}
	return item, nil
}

func (r *Repository) ListCompletedSlots(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]CompletedSlot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text,ds.slot_number,ds.distributed_at
		FROM distribution_slots ds JOIN program_schedules ps ON ps.id=ds.schedule_id
		WHERE ps.program_id=$1 AND ps.regency_id=$2 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		AND ($3 OR ps.regency_id::text=ANY($4)) ORDER BY ds.slot_number,ds.id
	`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list completed BA slots: %w", err)
	}
	defer rows.Close()
	items := []CompletedSlot{}
	for rows.Next() {
		var item CompletedSlot
		if err := rows.Scan(&item.ID, &item.SlotNumber, &item.DistributedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetLockedTotal(ctx context.Context, programID, regencyID string) (LockResult, error) {
	var result LockResult
	err := r.pool.QueryRow(ctx, `SELECT program_id::text,regency_id::text,final_total,locked_at FROM program_regency_bast_settings WHERE program_id=$1 AND regency_id=$2`, programID, regencyID).Scan(&result.ProgramID, &result.RegencyID, &result.FinalTotal, &result.LockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LockResult{}, ErrNotFound
	}
	if err != nil {
		return LockResult{}, fmt.Errorf("get locked BA total: %w", err)
	}
	return result, nil
}

func (r *Repository) LockRegencyTotal(ctx context.Context, actor auth.Principal, source SourceContext, meta auth.ClientMeta) (LockResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LockResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var result LockResult
	err = tx.QueryRow(ctx, `
		INSERT INTO program_regency_bast_settings(program_id,regency_id,final_total,locked_by)
		VALUES($1,$2,$3,NULLIF($4,'')::uuid)
		ON CONFLICT(program_id,regency_id) DO UPDATE SET final_total=EXCLUDED.final_total,locked_at=now(),locked_by=EXCLUDED.locked_by,updated_at=now()
		WHERE program_regency_bast_settings.final_total=EXCLUDED.final_total OR NOT EXISTS(
			SELECT 1 FROM bast_individual_documents d WHERE d.program_id=EXCLUDED.program_id AND d.regency_id=EXCLUDED.regency_id AND d.status='final'
		)
		RETURNING program_id::text,regency_id::text,final_total,locked_at
	`, source.ProgramID, source.RegencyID, source.SlotQuota, actor.UserID).Scan(&result.ProgramID, &result.RegencyID, &result.FinalTotal, &result.LockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LockResult{}, ErrFinalTotalLocked
	}
	if err != nil {
		return LockResult{}, fmt.Errorf("lock BA total: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.total_locked", ResourceType: "program_regency_bast_setting", ResourceID: source.ProgramID + ":" + source.RegencyID, Metadata: map[string]any{"final_total": source.SlotQuota}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return LockResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LockResult{}, err
	}
	return result, nil
}

func (r *Repository) ListActiveBundles(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]DailyBundle, error) {
	rows, err := r.pool.Query(ctx, `SELECT b.id::text,b.program_id::text,b.regency_id::text,b.local_date::text,b.filename,b.recipient_count,b.page_count,b.version,b.status,b.checksum,b.storage_key,COALESCE(b.last_error,''),b.synced_at FROM bast_daily_bundles b WHERE b.program_id=$1 AND b.regency_id=$2 AND b.status='active' AND ($3 OR b.regency_id::text=ANY($4)) ORDER BY b.local_date DESC`, programID, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list BA bundles: %w", err)
	}
	defer rows.Close()
	items := []DailyBundle{}
	for rows.Next() {
		var item DailyBundle
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.LocalDate, &item.Filename, &item.RecipientCount, &item.PageCount, &item.Version, &item.Status, &item.Checksum, &item.StorageKey, &item.LastError, &item.SyncedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetActiveBundle(ctx context.Context, programID, regencyID, localDate string, scope auth.RegencyScope) (DailyBundle, error) {
	var item DailyBundle
	err := r.pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text,local_date::text,filename,recipient_count,page_count,version,status,checksum,storage_key,COALESCE(last_error,''),synced_at FROM bast_daily_bundles WHERE program_id=$1 AND regency_id=$2 AND local_date=$3 AND document_type='individual' AND status='active' AND ($4 OR regency_id::text=ANY($5))`, programID, regencyID, localDate, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.LocalDate, &item.Filename, &item.RecipientCount, &item.PageCount, &item.Version, &item.Status, &item.Checksum, &item.StorageKey, &item.LastError, &item.SyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DailyBundle{}, ErrNotFound
	}
	if err != nil {
		return DailyBundle{}, fmt.Errorf("get active BA bundle: %w", err)
	}
	return item, nil
}

func (r *Repository) GetActiveBundleByID(ctx context.Context, id string, scope auth.RegencyScope) (DailyBundle, error) {
	var item DailyBundle
	err := r.pool.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text,local_date::text,filename,recipient_count,page_count,version,status,checksum,storage_key,COALESCE(last_error,''),synced_at FROM bast_daily_bundles WHERE id=$1 AND document_type='individual' AND status='active' AND ($2 OR regency_id::text=ANY($3))`, id, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.ProgramID, &item.RegencyID, &item.LocalDate, &item.Filename, &item.RecipientCount, &item.PageCount, &item.Version, &item.Status, &item.Checksum, &item.StorageKey, &item.LastError, &item.SyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DailyBundle{}, ErrNotFound
	}
	if err != nil {
		return DailyBundle{}, fmt.Errorf("get BA bundle: %w", err)
	}
	return item, nil
}

func (r *Repository) ActivateBundle(ctx context.Context, actor auth.Principal, input BundleActivation, meta auth.ClientMeta) (BundleActivationResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BundleActivationResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := input.ProgramID + ":" + input.RegencyID + ":" + input.LocalDate + ":individual"
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return BundleActivationResult{}, err
	}

	var current DailyBundle
	err = tx.QueryRow(ctx, `SELECT id::text,program_id::text,regency_id::text,local_date::text,filename,recipient_count,page_count,version,status,checksum,storage_key,COALESCE(last_error,''),synced_at FROM bast_daily_bundles WHERE program_id=$1 AND regency_id=$2 AND local_date=$3 AND document_type='individual' AND status='active' FOR UPDATE`, input.ProgramID, input.RegencyID, input.LocalDate).Scan(&current.ID, &current.ProgramID, &current.RegencyID, &current.LocalDate, &current.Filename, &current.RecipientCount, &current.PageCount, &current.Version, &current.Status, &current.Checksum, &current.StorageKey, &current.LastError, &current.SyncedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return BundleActivationResult{}, err
	}
	currentExists := err == nil
	if currentExists && current.Checksum == input.Checksum {
		if err := tx.Commit(ctx); err != nil {
			return BundleActivationResult{}, err
		}
		return BundleActivationResult{Bundle: current, Unchanged: true}, nil
	}
	if (currentExists && current.ID != input.ExpectedActiveID) || (!currentExists && input.ExpectedActiveID != "") {
		return BundleActivationResult{}, ErrBundleConflict
	}
	if !currentExists && input.ExpectedActiveID == "" {
		// No prior active version is the expected initial state.
	} else if currentExists {
		if _, err := tx.Exec(ctx, `UPDATE bast_daily_bundles SET status='superseded',updated_at=now() WHERE id=$1`, current.ID); err != nil {
			return BundleActivationResult{}, err
		}
	}

	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM bast_daily_bundles WHERE program_id=$1 AND regency_id=$2 AND local_date=$3 AND document_type='individual'`, input.ProgramID, input.RegencyID, input.LocalDate).Scan(&version); err != nil {
		return BundleActivationResult{}, err
	}
	var bundle DailyBundle
	err = tx.QueryRow(ctx, `INSERT INTO bast_daily_bundles(program_id,regency_id,local_date,document_type,filename,recipient_count,page_count,checksum,storage_key,version,status,synced_at,synced_by) VALUES($1,$2,$3,'individual',$4,$5,$6,$7,$8,$9,'active',now(),NULLIF($10,'')::uuid) RETURNING id::text,program_id::text,regency_id::text,local_date::text,filename,recipient_count,page_count,version,status,checksum,storage_key,COALESCE(last_error,''),synced_at`, input.ProgramID, input.RegencyID, input.LocalDate, input.Filename, len(input.Items), input.PageCount, input.Checksum, input.StorageKey, version, actor.UserID).Scan(&bundle.ID, &bundle.ProgramID, &bundle.RegencyID, &bundle.LocalDate, &bundle.Filename, &bundle.RecipientCount, &bundle.PageCount, &bundle.Version, &bundle.Status, &bundle.Checksum, &bundle.StorageKey, &bundle.LastError, &bundle.SyncedAt)
	if err != nil {
		return BundleActivationResult{}, fmt.Errorf("insert BA bundle: %w", err)
	}
	for i, item := range input.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO bast_daily_bundle_items(bundle_id,individual_document_id,item_order,page_start,page_end) VALUES($1,$2,$3,$4,$5)`, bundle.ID, item.IndividualDocumentID, i+1, item.PageStart, item.PageEnd); err != nil {
			return BundleActivationResult{}, fmt.Errorf("insert BA bundle item: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.bundle_activated", ResourceType: "bast_daily_bundle", ResourceID: bundle.ID, Metadata: map[string]any{"local_date": input.LocalDate, "recipient_count": len(input.Items), "version": version}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return BundleActivationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BundleActivationResult{}, err
	}
	return BundleActivationResult{Bundle: bundle, OldStorageKey: current.StorageKey}, nil
}

func (r *Repository) RecordCleanupFailure(ctx context.Context, bundleID, message string) error {
	_, err := r.pool.Exec(ctx, `UPDATE bast_daily_bundles SET last_error=$2,updated_at=now() WHERE id=$1 AND status='active'`, bundleID, message)
	return err
}
