package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// AggregateWithSnapshot adalah dokumen agregat aktif beserta snapshot_json-nya.
type AggregateWithSnapshot struct {
	Document AggregateDocument
	Snapshot []byte
}

// ListActiveAggregatesWithSnapshot mengembalikan dokumen aktif satu jenis di
// semua tanggal jadwal, urut tanggal, beserta snapshot-nya.
func (r *Repository) ListActiveAggregatesWithSnapshot(ctx context.Context, scheduleID, documentType string, scope auth.RegencyScope) ([]AggregateWithSnapshot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+aggregateColumnsD+`,d.snapshot_json::text FROM bast_aggregate_documents d JOIN program_schedules ps ON ps.id=d.schedule_id WHERE d.schedule_id=$1 AND d.document_type=$2 AND d.status='active' AND ($3 OR ps.regency_id::text=ANY($4)) ORDER BY d.document_date`, scheduleID, documentType, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list active aggregates: %w", err)
	}
	defer rows.Close()
	items := []AggregateWithSnapshot{}
	for rows.Next() {
		var item AggregateWithSnapshot
		var snapshot string
		doc := &item.Document
		if err := rows.Scan(&doc.ID, &doc.ScheduleID, &doc.ProgramID, &doc.RegencyID, &doc.DocumentType, &doc.DocumentDate, &doc.Filename, &doc.RecipientCount, &doc.PageCount, &doc.Version, &doc.Status, &doc.Checksum, &doc.StorageKey, &doc.LastError, &doc.FinalizedAt, &snapshot); err != nil {
			return nil, fmt.Errorf("scan active aggregate: %w", err)
		}
		item.Snapshot = []byte(snapshot)
		items = append(items, item)
	}
	return items, rows.Err()
}

// RetireAggregate membatalkan dokumen aktif (mis. penerimaan yang salah
// tanggal) dengan menandainya superseded, lalu mengembalikan dokumennya agar
// berkasnya dapat dihapus dari penyimpanan.
func (r *Repository) RetireAggregate(ctx context.Context, actor auth.Principal, id, documentType string, scope auth.RegencyScope, meta auth.ClientMeta) (AggregateDocument, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AggregateDocument{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	doc, err := scanAggregate(tx.QueryRow(ctx, `UPDATE bast_aggregate_documents d SET status='superseded',updated_at=now() FROM program_schedules ps WHERE ps.id=d.schedule_id AND d.id::text=$1 AND d.document_type=$2 AND d.status='active' AND ($3 OR ps.regency_id::text=ANY($4)) RETURNING `+aggregateColumnsD, id, documentType, scope.Unrestricted, scope.RegencyIDs))
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
			return AggregateDocument{}, ErrNotFound
		}
		return AggregateDocument{}, fmt.Errorf("retire aggregate: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast." + documentType + "_cancelled", ResourceType: "bast_aggregate_document", ResourceID: doc.ID, Metadata: map[string]any{"schedule_id": doc.ScheduleID, "document_date": doc.DocumentDate, "document_type": documentType, "version": doc.Version}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return AggregateDocument{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AggregateDocument{}, err
	}
	return doc, nil
}

// ListGasElpijiDrafts mengembalikan draf isian penerimaan satu jadwal.
func (r *Repository) ListGasElpijiDrafts(ctx context.Context, scheduleID string) ([]GasElpijiInput, error) {
	rows, err := r.pool.Query(ctx, `SELECT input_json::text FROM bast_gas_elpiji_drafts WHERE schedule_id::text=$1 ORDER BY document_date`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list gas elpiji drafts: %w", err)
	}
	defer rows.Close()
	drafts := []GasElpijiInput{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan gas elpiji draft: %w", err)
		}
		var input GasElpijiInput
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, fmt.Errorf("decode gas elpiji draft: %w", err)
		}
		drafts = append(drafts, input)
	}
	return drafts, rows.Err()
}

// GetGasElpijiDraft mengembalikan draf satu tanggal; ErrNotFound bila tidak ada.
func (r *Repository) GetGasElpijiDraft(ctx context.Context, scheduleID, documentDate string) (GasElpijiInput, error) {
	var raw string
	err := r.pool.QueryRow(ctx, `SELECT input_json::text FROM bast_gas_elpiji_drafts WHERE schedule_id::text=$1 AND document_date=$2::date`, scheduleID, documentDate).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return GasElpijiInput{}, ErrNotFound
	}
	if err != nil {
		return GasElpijiInput{}, fmt.Errorf("get gas elpiji draft: %w", err)
	}
	var input GasElpijiInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return GasElpijiInput{}, fmt.Errorf("decode gas elpiji draft: %w", err)
	}
	return input, nil
}

func (r *Repository) SaveGasElpijiDraft(ctx context.Context, actor auth.Principal, input GasElpijiInput) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO bast_gas_elpiji_drafts(schedule_id,document_date,input_json,updated_by) VALUES($1,$2::date,$3::jsonb,NULLIF($4,'')::uuid)
		ON CONFLICT (schedule_id,document_date) DO UPDATE SET input_json=EXCLUDED.input_json,updated_by=EXCLUDED.updated_by,updated_at=now()`, input.ScheduleID, input.DocumentDate, string(raw), actor.UserID)
	if err != nil {
		return fmt.Errorf("save gas elpiji draft: %w", err)
	}
	return nil
}

func (r *Repository) DeleteGasElpijiDraft(ctx context.Context, scheduleID, documentDate string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM bast_gas_elpiji_drafts WHERE schedule_id::text=$1 AND document_date=$2::date`, scheduleID, documentDate); err != nil {
		return fmt.Errorf("delete gas elpiji draft: %w", err)
	}
	return nil
}

// GetWetScanImage mengembalikan pindaian tanda tangan basah satu tanggal
// beserta kode isinya, tanpa memeriksa apakah mode basah masih aktif (BA
// gabungan memakai pindaian penerimaan yang sudah final).
func (r *Repository) GetWetScanImage(ctx context.Context, scheduleID, documentKey, date string) ([]byte, string, error) {
	var image []byte
	var code string
	err := r.pool.QueryRow(ctx, `SELECT image_png, content_code FROM bast_wet_signature_scans WHERE schedule_id::text=$1 AND document_key=$2 AND document_date=$3::date`, scheduleID, documentKey, date).Scan(&image, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("get wet scan image: %w", err)
	}
	return image, code, nil
}
