package bast

import (
	"context"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/textnorm"
)

type handoverRepository interface {
	ListHandoverLocations(context.Context, string, auth.RegencyScope) ([]HandoverLocation, error)
	ListHandoverRecipients(context.Context, string, auth.RegencyScope) ([]handoverRecipient, error)
	HandoverDocumentStatuses(context.Context, string) (map[string]handoverDocumentStatus, error)
	RenameHandoverLocation(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	MergeHandoverLocations(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	DeleteHandoverLocation(context.Context, auth.Principal, string, string, auth.RegencyScope, auth.ClientMeta) error
	AssignHandoverLocation(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
}

// HandoverService merekap dan merapikan titik serah BA Serah Terima.
type HandoverService struct {
	repository handoverRepository
}

func NewHandoverService(repository handoverRepository) *HandoverService {
	return &HandoverService{repository: repository}
}

// View mengembalikan rekap per titik serah dan penerima tanpa titik serah.
func (s *HandoverService) View(ctx context.Context, scheduleID string, scope auth.RegencyScope) (HandoverView, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return HandoverView{}, ErrInvalidInput
	}
	locations, err := s.repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil {
		return HandoverView{}, err
	}
	recipients, err := s.repository.ListHandoverRecipients(ctx, scheduleID, scope)
	if err != nil {
		return HandoverView{}, err
	}
	statuses, err := s.repository.HandoverDocumentStatuses(ctx, scheduleID)
	if err != nil {
		return HandoverView{}, err
	}
	return buildHandoverView(scheduleID, locations, recipients, statuses), nil
}

// Rename memperbaiki nama titik serah (mis. salah ketik).
func (s *HandoverService) Rename(ctx context.Context, actor auth.Principal, scheduleID, locationID, name string, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	name = textnorm.PlaceName(name)
	if textnorm.PlaceKey(name) == "" || len(name) > 200 || strings.TrimSpace(locationID) == "" {
		return HandoverView{}, ErrHandoverLocationInvalid
	}
	if err := s.repository.RenameHandoverLocation(ctx, actor, strings.TrimSpace(scheduleID), strings.TrimSpace(locationID), name, scope, meta); err != nil {
		return HandoverView{}, err
	}
	return s.View(ctx, scheduleID, scope)
}

// Merge menggabungkan varian nama ke lokasi yang benar.
func (s *HandoverService) Merge(ctx context.Context, actor auth.Principal, scheduleID, sourceID, targetID string, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	sourceID, targetID = strings.TrimSpace(sourceID), strings.TrimSpace(targetID)
	if sourceID == "" || targetID == "" {
		return HandoverView{}, ErrHandoverLocationInvalid
	}
	if err := s.repository.MergeHandoverLocations(ctx, actor, strings.TrimSpace(scheduleID), sourceID, targetID, scope, meta); err != nil {
		return HandoverView{}, err
	}
	return s.View(ctx, scheduleID, scope)
}

// Delete menghapus titik serah tanpa penerima.
func (s *HandoverService) Delete(ctx context.Context, actor auth.Principal, scheduleID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	if strings.TrimSpace(locationID) == "" {
		return HandoverView{}, ErrHandoverLocationInvalid
	}
	if err := s.repository.DeleteHandoverLocation(ctx, actor, strings.TrimSpace(scheduleID), strings.TrimSpace(locationID), scope, meta); err != nil {
		return HandoverView{}, err
	}
	return s.View(ctx, scheduleID, scope)
}

// Assign mengoreksi titik serah satu penerima.
func (s *HandoverService) Assign(ctx context.Context, actor auth.Principal, scheduleID, slotID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	slotID, locationID = strings.TrimSpace(slotID), strings.TrimSpace(locationID)
	if slotID == "" || locationID == "" {
		return HandoverView{}, ErrHandoverLocationInvalid
	}
	if err := s.repository.AssignHandoverLocation(ctx, actor, strings.TrimSpace(scheduleID), slotID, locationID, scope, meta); err != nil {
		return HandoverView{}, err
	}
	return s.View(ctx, scheduleID, scope)
}
