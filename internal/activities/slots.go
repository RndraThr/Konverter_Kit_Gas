package activities

import (
	"context"
	"errors"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSlotNameRequired = errors.New("supporting document slot name is required")
	ErrSlotNotFound     = errors.New("supporting document slot not found")
)

type SupportingDocumentSlot struct {
	ID         string    `json:"id"`
	RegencyID  string    `json:"regency_id"`
	Name       string    `json:"name"`
	IsActive   bool      `json:"is_active"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type slotRepository interface {
	ListSlots(ctx context.Context, regencyID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error)
	InsertSlot(ctx context.Context, actor auth.Principal, regencyID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)
	PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error)
}

func (s *Service) ListSlots(ctx context.Context, regencyID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error) {
	regencyID = strings.TrimSpace(regencyID)
	if regencyID == "" {
		return nil, ErrRegencyRequired
	}
	return s.slotRepository.ListSlots(ctx, regencyID, scope)
}

func (s *Service) CreateSlot(ctx context.Context, actor auth.Principal, regencyID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	regencyID = strings.TrimSpace(regencyID)
	name = strings.TrimSpace(name)
	if regencyID == "" {
		return SupportingDocumentSlot{}, ErrRegencyRequired
	}
	if name == "" {
		return SupportingDocumentSlot{}, ErrSlotNameRequired
	}
	return s.slotRepository.InsertSlot(ctx, actor, regencyID, name, meta, scope)
}

func (s *Service) PatchSlot(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SupportingDocumentSlot{}, ErrSlotNotFound
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return SupportingDocumentSlot{}, ErrSlotNameRequired
		}
		name = &trimmed
	}
	return s.slotRepository.PatchSlotRow(ctx, actor, id, name, isActive, meta, scope)
}

// ListSlots filters by regency scope at the SQL level — like activities.List
// elsewhere in this package, an out-of-scope regency_id yields an empty
// list, not an error.
func (r *Repository) ListSlots(ctx context.Context, regencyID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id::text, s.regency_id::text, s.name, s.is_active, s.sort_order, s.created_at, s.updated_at
		FROM supporting_document_slots s
		WHERE s.regency_id = $1 AND s.is_active AND ($2 OR s.regency_id::text = ANY($3))
		ORDER BY s.sort_order, s.name
	`, regencyID, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SupportingDocumentSlot, 0)
	for rows.Next() {
		var item SupportingDocumentSlot
		if err := rows.Scan(&item.ID, &item.RegencyID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// InsertSlot only inserts if regencyID is in scope — the SELECT...FROM
// regencies in place of a bare VALUES list makes an out-of-scope or
// nonexistent regency_id insert zero rows instead of succeeding, which
// ErrRegencyNotFound below turns into a clear not-found error.
func (r *Repository) InsertSlot(ctx context.Context, actor auth.Principal, regencyID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	err = tx.QueryRow(ctx, `
		INSERT INTO supporting_document_slots (regency_id, name)
		SELECT r.id, $2 FROM regencies r
		WHERE r.id = $1 AND ($3 OR r.id::text = ANY($4))
		RETURNING id::text, regency_id::text, name, is_active, sort_order, created_at, updated_at
	`, regencyID, name, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.RegencyID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupportingDocumentSlot{}, ErrRegencyNotFound
	}
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "supporting_document_slot.created", ResourceType: "supporting_document_slot", ResourceID: item.ID, Metadata: map[string]any{"regency_id": regencyID, "name": name}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportingDocumentSlot{}, err
	}
	return item, nil
}

// PatchSlotRow filters by regency scope in the UPDATE so an id belonging to
// an out-of-scope regency matches zero rows (-> ErrSlotNotFound) instead of
// being patched.
func (r *Repository) PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	err = tx.QueryRow(ctx, `
		UPDATE supporting_document_slots s SET
			name = COALESCE($2, s.name),
			is_active = COALESCE($3, s.is_active),
			updated_at = now()
		WHERE s.id = $1 AND ($4 OR s.regency_id::text = ANY($5))
		RETURNING s.id::text, s.regency_id::text, s.name, s.is_active, s.sort_order, s.created_at, s.updated_at
	`, id, name, isActive, scope.Unrestricted, scope.RegencyIDs).Scan(&item.ID, &item.RegencyID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SupportingDocumentSlot{}, ErrSlotNotFound
	}
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "supporting_document_slot.patched", ResourceType: "supporting_document_slot", ResourceID: id, Metadata: map[string]any{"name": name, "is_active": isActive}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportingDocumentSlot{}, err
	}
	return item, nil
}
