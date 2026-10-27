package bast

import (
	"context"
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
