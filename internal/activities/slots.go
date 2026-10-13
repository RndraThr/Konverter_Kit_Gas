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
	ScheduleID string    `json:"schedule_id"`
	Name       string    `json:"name"`
	IsActive   bool      `json:"is_active"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type slotRepository interface {
	ListSlots(ctx context.Context, scheduleID string) ([]SupportingDocumentSlot, error)
	InsertSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta) (SupportingDocumentSlot, error)
	PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta) (SupportingDocumentSlot, error)
}

func (s *Service) ListSlots(ctx context.Context, scheduleID string) ([]SupportingDocumentSlot, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrScheduleRequired
	}
	return s.slotRepository.ListSlots(ctx, scheduleID)
}

func (s *Service) CreateSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	name = strings.TrimSpace(name)
	if scheduleID == "" {
		return SupportingDocumentSlot{}, ErrScheduleRequired
	}
	if name == "" {
		return SupportingDocumentSlot{}, ErrSlotNameRequired
	}
	return s.slotRepository.InsertSlot(ctx, actor, scheduleID, name, meta)
}

func (s *Service) PatchSlot(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
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
	return s.slotRepository.PatchSlotRow(ctx, actor, id, name, isActive, meta)
}

func (r *Repository) ListSlots(ctx context.Context, scheduleID string) ([]SupportingDocumentSlot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, schedule_id::text, name, is_active, sort_order, created_at, updated_at
		FROM supporting_document_slots WHERE schedule_id = $1 AND is_active
		ORDER BY sort_order, name
	`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SupportingDocumentSlot, 0)
	for rows.Next() {
		var item SupportingDocumentSlot
		if err := rows.Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) InsertSlot(ctx context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	if err := tx.QueryRow(ctx, `
		INSERT INTO supporting_document_slots (schedule_id, name) VALUES ($1, $2)
		RETURNING id::text, schedule_id::text, name, is_active, sort_order, created_at, updated_at
	`, scheduleID, name).Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "supporting_document_slot.created", ResourceType: "supporting_document_slot", ResourceID: item.ID, Metadata: map[string]any{"schedule_id": scheduleID, "name": name}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return SupportingDocumentSlot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SupportingDocumentSlot{}, err
	}
	return item, nil
}

func (r *Repository) PatchSlotRow(ctx context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SupportingDocumentSlot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item SupportingDocumentSlot
	err = tx.QueryRow(ctx, `
		UPDATE supporting_document_slots SET
			name = COALESCE($2, name),
			is_active = COALESCE($3, is_active),
			updated_at = now()
		WHERE id = $1
		RETURNING id::text, schedule_id::text, name, is_active, sort_order, created_at, updated_at
	`, id, name, isActive).Scan(&item.ID, &item.ScheduleID, &item.Name, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
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
