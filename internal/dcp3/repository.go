package dcp3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/programs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) CreatePreview(ctx context.Context, actor auth.Principal, scheduleID, filename, checksum string, workbook WorkbookPreview, meta auth.ClientMeta, scope auth.RegencyScope) (ImportPreview, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ImportPreview{}, fmt.Errorf("begin DCP3 preview: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var batchID string
	headerEnvelope, _ := json.Marshal(map[string]any{"headers": workbook.Headers})
	err = tx.QueryRow(ctx, `
		INSERT INTO dcp3_import_batches(schedule_id,original_filename,file_checksum,sheet_name,mapping_json,total_rows)
		SELECT s.id,$2,$3,$4,$5,$6 FROM program_schedules s WHERE s.id=$1 AND ($7 OR s.regency_id::text = ANY($8))
		RETURNING id::text
	`, scheduleID, filename, checksum, workbook.SheetName, headerEnvelope, len(workbook.Rows), scope.Unrestricted, scope.RegencyIDs).Scan(&batchID)
	if isUniqueViolation(err) {
		return ImportPreview{}, ErrDuplicateImport
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, ErrPreviewNotFound
	}
	if err != nil {
		return ImportPreview{}, fmt.Errorf("insert DCP3 preview: %w", err)
	}
	for _, row := range workbook.Rows {
		values := make(map[string]string, len(workbook.Headers))
		for index, header := range workbook.Headers {
			if index < len(row.Values) {
				values[header] = row.Values[index]
			}
		}
		raw, _ := json.Marshal(values)
		if _, err := tx.Exec(ctx, `INSERT INTO dcp3_import_rows(batch_id,source_row_number,raw_data_json) VALUES($1,$2,$3)`, batchID, row.SourceRowNumber, raw); err != nil {
			return ImportPreview{}, fmt.Errorf("insert DCP3 preview row: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "dcp3.preview_created", ResourceType: "dcp3_import_batch", ResourceID: batchID, Metadata: map[string]any{"schedule_id": scheduleID, "filename": filename, "checksum": checksum, "row_count": len(workbook.Rows)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return ImportPreview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportPreview{}, fmt.Errorf("commit DCP3 preview: %w", err)
	}
	return r.GetPreview(ctx, batchID, auth.RegencyScope{Unrestricted: true})
}

func (r *Repository) GetPreview(ctx context.Context, id string, scope auth.RegencyScope) (ImportPreview, error) {
	var result ImportPreview
	var programType programs.ProgramType
	var envelopeData []byte
	err := r.pool.QueryRow(ctx, `
		SELECT b.id::text,b.schedule_id::text,p.program_type,b.original_filename,b.file_checksum,b.sheet_name,b.mapping_json,b.status,b.created_at
		FROM dcp3_import_batches b JOIN program_schedules s ON s.id=b.schedule_id JOIN programs p ON p.id=s.program_id
		WHERE b.id=$1 AND ($2 OR s.regency_id::text = ANY($3))
	`, id, scope.Unrestricted, scope.RegencyIDs).Scan(&result.ID, &result.ScheduleID, &programType, &result.OriginalFilename, &result.FileChecksum, &result.SheetName, &envelopeData, &result.Status, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, ErrPreviewNotFound
	}
	if err != nil {
		return ImportPreview{}, fmt.Errorf("get DCP3 preview: %w", err)
	}
	result.ProgramType = programType
	var envelope struct {
		Headers []string `json:"headers"`
	}
	if err := json.Unmarshal(envelopeData, &envelope); err != nil {
		return ImportPreview{}, fmt.Errorf("decode DCP3 headers: %w", err)
	}
	result.Headers = envelope.Headers
	rows, err := r.pool.Query(ctx, `SELECT id::text,source_row_number,raw_data_json FROM dcp3_import_rows WHERE batch_id=$1 ORDER BY source_row_number`, id)
	if err != nil {
		return ImportPreview{}, fmt.Errorf("list DCP3 preview rows: %w", err)
	}
	defer rows.Close()
	result.Rows = []RawImportRow{}
	for rows.Next() {
		var item RawImportRow
		var raw []byte
		if err := rows.Scan(&item.ID, &item.SourceRowNumber, &raw); err != nil {
			return ImportPreview{}, fmt.Errorf("scan DCP3 preview row: %w", err)
		}
		if err := json.Unmarshal(raw, &item.Values); err != nil {
			return ImportPreview{}, fmt.Errorf("decode DCP3 preview row: %w", err)
		}
		result.Rows = append(result.Rows, item)
	}
	return result, rows.Err()
}

func (r *Repository) Commit(ctx context.Context, actor auth.Principal, batchID string, mapping Mapping, meta auth.ClientMeta) (ImportResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin DCP3 import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var programType programs.ProgramType
	var scheduleID, documentationTemplateID string
	var packageSnapshot, previewMetadata []byte
	err = tx.QueryRow(ctx, `
		SELECT b.status,p.program_type,b.schedule_id::text,s.documentation_template_version_id::text,pt.values_json,b.mapping_json
		FROM dcp3_import_batches b
		JOIN program_schedules s ON s.id=b.schedule_id
		JOIN programs p ON p.id=s.program_id
		JOIN package_template_versions pt ON pt.id=s.package_template_version_id
		WHERE b.id=$1 FOR UPDATE OF b
	`, batchID).Scan(&status, &programType, &scheduleID, &documentationTemplateID, &packageSnapshot, &previewMetadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportResult{}, ErrPreviewNotFound
	}
	if err != nil {
		return ImportResult{}, fmt.Errorf("lock DCP3 batch: %w", err)
	}
	if status != "draft" {
		return ImportResult{}, ErrImportState
	}

	rows, err := tx.Query(ctx, `SELECT id::text,source_row_number,raw_data_json FROM dcp3_import_rows WHERE batch_id=$1 ORDER BY source_row_number FOR UPDATE`, batchID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("lock DCP3 rows: %w", err)
	}
	rawRows := []RawImportRow{}
	for rows.Next() {
		var item RawImportRow
		var raw []byte
		if err := rows.Scan(&item.ID, &item.SourceRowNumber, &raw); err != nil {
			rows.Close()
			return ImportResult{}, fmt.Errorf("scan DCP3 row: %w", err)
		}
		if err := json.Unmarshal(raw, &item.Values); err != nil {
			rows.Close()
			return ImportResult{}, fmt.Errorf("decode DCP3 row: %w", err)
		}
		rawRows = append(rawRows, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ImportResult{}, err
	}
	rows.Close()

	result := ImportResult{BatchID: batchID, TotalRows: len(rawRows)}
	seenNIK := map[string]struct{}{}
	for _, raw := range rawRows {
		normalized := NormalizeRow(programType, raw, mapping)
		duplicateIdentity := false
		if normalized.NIK != "" {
			if _, exists := seenNIK[normalized.NIK]; exists {
				normalized.raise(RowNeedsReview, "NIK muncul lebih dari sekali dalam batch")
				duplicateIdentity = true
			} else {
				seenNIK[normalized.NIK] = struct{}{}
			}
		}
		personID, identityConflict, err := r.matchOrCreatePerson(ctx, tx, normalized, duplicateIdentity)
		if err != nil {
			return ImportResult{}, err
		}
		if identityConflict {
			normalized.raise(RowNeedsReview, "Identitas NIK dan kartu sektor dimiliki orang yang berbeda")
		}
		if personID != "" && !identityConflict {
			var previouslyReceived bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM distribution_records WHERE recipient_person_id=$1 AND status='completed')`, personID).Scan(&previouslyReceived); err != nil {
				return ImportResult{}, fmt.Errorf("check prior distribution: %w", err)
			}
			if previouslyReceived {
				normalized.raise(RowNeedsReview, "Penerima pernah menerima paket sebelumnya")
			}
		}
		normalizedJSON, _ := json.Marshal(normalized)
		messagesJSON, _ := json.Marshal(normalized.ValidationMessages)
		_, err = tx.Exec(ctx, `UPDATE dcp3_import_rows SET source_sequence_number=$2,normalized_data_json=$3,validation_status=$4,validation_messages_json=$5,updated_at=now() WHERE id=$1`, raw.ID, normalized.SourceSequenceNumber, normalizedJSON, normalized.ValidationStatus, messagesJSON)
		if err != nil {
			return ImportResult{}, fmt.Errorf("update DCP3 row: %w", err)
		}

		nominationStatus := "ready"
		allocationStatus := "ready"
		if normalized.ValidationStatus == RowNeedsReview || normalized.ValidationStatus == RowInvalid || personID == "" {
			nominationStatus, allocationStatus = "needs_review", "needs_review"
		}
		sourceSnapshot, _ := json.Marshal(normalized.SourceValues)
		var nominationID string
		if err := tx.QueryRow(ctx, `INSERT INTO candidate_nominations(batch_id,import_row_id,person_id,program_type,source_snapshot_json,status) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6) RETURNING id::text`, batchID, raw.ID, personID, programType, sourceSnapshot, nominationStatus).Scan(&nominationID); err != nil {
			return ImportResult{}, fmt.Errorf("insert candidate nomination: %w", err)
		}
		distributionNumber, err := nextDistributionNumber(ctx, tx, scheduleID, normalized.SourceSequenceNumber)
		if err != nil {
			return ImportResult{}, err
		}
		var allocationID string
		if err := tx.QueryRow(ctx, `INSERT INTO package_allocations(schedule_id,nomination_id,intended_person_id,distribution_number,status,package_snapshot_json) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6) RETURNING id::text`, scheduleID, nominationID, personID, distributionNumber, allocationStatus, packageSnapshot).Scan(&allocationID); err != nil {
			return ImportResult{}, fmt.Errorf("insert package allocation: %w", err)
		}
		var distributionID string
		if err := tx.QueryRow(ctx, `INSERT INTO distribution_records(allocation_id,recipient_person_id) VALUES($1,NULLIF($2,'')::uuid) RETURNING id::text`, allocationID, personID).Scan(&distributionID); err != nil {
			return ImportResult{}, fmt.Errorf("insert distribution draft: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO documentation_slots(distribution_id,slot_code,label_snapshot,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
			SELECT $1,slot_code,label,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order
			FROM documentation_template_slots WHERE template_version_id=$2
		`, distributionID, documentationTemplateID)
		if err != nil {
			return ImportResult{}, fmt.Errorf("snapshot documentation slots: %w", err)
		}
		switch normalized.ValidationStatus {
		case RowValid:
			result.ValidRows++
		case RowInvalid:
			result.InvalidRows++
		default:
			result.WarningRows++
		}
	}
	var previewEnvelope struct {
		Headers []string `json:"headers"`
	}
	if err := json.Unmarshal(previewMetadata, &previewEnvelope); err != nil {
		return ImportResult{}, fmt.Errorf("decode DCP3 preview metadata: %w", err)
	}
	mappingJSON, _ := json.Marshal(map[string]any{"headers": previewEnvelope.Headers, "mapping": mapping})
	_, err = tx.Exec(ctx, `UPDATE dcp3_import_batches SET mapping_json=$2,status='imported',valid_rows=$3,warning_rows=$4,invalid_rows=$5,imported_by=NULLIF($6,'')::uuid,imported_at=now(),updated_at=now() WHERE id=$1`, batchID, mappingJSON, result.ValidRows, result.WarningRows, result.InvalidRows, actor.UserID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("complete DCP3 batch: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "dcp3.imported", ResourceType: "dcp3_import_batch", ResourceID: batchID, Metadata: map[string]any{"schedule_id": scheduleID, "total_rows": result.TotalRows, "valid_rows": result.ValidRows, "warning_rows": result.WarningRows, "invalid_rows": result.InvalidRows}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return ImportResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, fmt.Errorf("commit DCP3 import: %w", err)
	}
	return result, nil
}

func (r *Repository) matchOrCreatePerson(ctx context.Context, tx pgx.Tx, row NormalizedRow, duplicateIdentity bool) (string, bool, error) {
	if row.ValidationStatus == RowInvalid || (row.NIK != "" && len(row.NIK) != 16) {
		return "", false, nil
	}
	nikPersonID, _ := personByNIK(ctx, tx, row.NIK)
	cardPersonID, cardPersonNIK, _ := personByIdentifier(ctx, tx, row.IdentifierType, row.SectorIdentifier)
	if nikPersonID != "" && cardPersonID != "" && nikPersonID != cardPersonID {
		return "", true, nil
	}
	if cardPersonID != "" && row.NIK != "" && cardPersonNIK != "" && cardPersonNIK != row.NIK {
		return "", true, nil
	}
	personID := nikPersonID
	if personID == "" {
		personID = cardPersonID
	}
	if personID == "" {
		if err := tx.QueryRow(ctx, `INSERT INTO people(full_name,nik,address,village,district,phone_number,verification_status) VALUES($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7) RETURNING id::text`, row.FullName, row.NIK, row.Address, row.Village, row.District, row.PhoneNumber, verificationStatus(row.ValidationStatus)).Scan(&personID); err != nil {
			return "", false, fmt.Errorf("insert person: %w", err)
		}
	} else if !duplicateIdentity {
		_, err := tx.Exec(ctx, `UPDATE people SET nik=COALESCE(nik,NULLIF($2,'')),address=COALESCE(address,NULLIF($3,'')),village=COALESCE(village,NULLIF($4,'')),district=COALESCE(district,NULLIF($5,'')),phone_number=COALESCE(phone_number,NULLIF($6,'')),updated_at=now() WHERE id=$1`, personID, row.NIK, row.Address, row.Village, row.District, row.PhoneNumber)
		if err != nil {
			return "", false, fmt.Errorf("complete person from DCP3: %w", err)
		}
	}
	if row.SectorIdentifier != "" && cardPersonID == "" && !duplicateIdentity {
		_, err := tx.Exec(ctx, `INSERT INTO person_sector_identifiers(person_id,identifier_type,normalized_value,display_value) VALUES($1,$2,$3,$4) ON CONFLICT (person_id,identifier_type) DO NOTHING`, personID, row.IdentifierType, row.SectorIdentifier, row.SectorIdentifierDisplay)
		if err != nil {
			return "", false, fmt.Errorf("insert sector identifier: %w", err)
		}
	}
	return personID, false, nil
}

func personByNIK(ctx context.Context, tx pgx.Tx, nik string) (string, error) {
	if nik == "" {
		return "", nil
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM people WHERE nik=$1`, nik).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func personByIdentifier(ctx context.Context, tx pgx.Tx, identifierType, value string) (string, string, error) {
	if value == "" {
		return "", "", nil
	}
	var id, nik string
	err := tx.QueryRow(ctx, `SELECT p.id::text,COALESCE(p.nik,'') FROM person_sector_identifiers i JOIN people p ON p.id=i.person_id WHERE i.identifier_type=$1 AND i.normalized_value=$2`, identifierType, value).Scan(&id, &nik)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return id, nik, err
}

func nextDistributionNumber(ctx context.Context, tx pgx.Tx, scheduleID string, preferred *int) (int, error) {
	if preferred != nil {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM package_allocations WHERE schedule_id=$1 AND distribution_number=$2)`, scheduleID, *preferred).Scan(&available); err != nil {
			return 0, fmt.Errorf("check distribution number: %w", err)
		}
		if available {
			return *preferred, nil
		}
	}
	var next int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(distribution_number),0)+1 FROM package_allocations WHERE schedule_id=$1`, scheduleID).Scan(&next); err != nil {
		return 0, fmt.Errorf("allocate distribution number: %w", err)
	}
	return next, nil
}

func verificationStatus(status RowStatus) string {
	if status == RowNeedsReview {
		return "needs_review"
	}
	return "unverified"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
