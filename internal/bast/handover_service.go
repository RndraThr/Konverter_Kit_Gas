package bast

import (
	"context"
	"reflect"
	"strings"

	"konkit/internal/auth"
)

type handoverRepository interface {
	ListHandoverLocations(context.Context, string, auth.RegencyScope) ([]HandoverLocation, error)
	ListHandoverRecipients(context.Context, string, auth.RegencyScope) ([]handoverRecipient, error)
	HandoverDocumentStatuses(context.Context, string) (map[string]handoverDocumentStatus, error)
	SaveHandoverLocations(context.Context, auth.Principal, string, []HandoverLocationInput, auth.RegencyScope, auth.ClientMeta) error
	SetHandoverOverride(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
}

// HandoverService mengelola titik serah BA Serah Terima satu jadwal.
type HandoverService struct {
	repository handoverRepository
}

func NewHandoverService(repository handoverRepository) *HandoverService {
	return &HandoverService{repository: repository}
}

// View mengembalikan daftar titik serah, penempatan penerima, dan rekap per
// lokasi.
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

// Save mengganti daftar titik serah; tanpa perubahan, dokumen final tidak
// ditandai perlu dibuat ulang.
func (s *HandoverService) Save(ctx context.Context, actor auth.Principal, scheduleID string, inputs []HandoverLocationInput, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" || len(inputs) == 0 || len(inputs) > 100 {
		return HandoverView{}, ErrHandoverLocationInvalid
	}
	normalized, err := normalizeHandoverInputs(inputs)
	if err != nil {
		return HandoverView{}, err
	}
	current, err := s.repository.ListHandoverLocations(ctx, scheduleID, scope)
	if err != nil {
		return HandoverView{}, err
	}
	if !sameHandoverLocations(current, normalized) {
		if err := s.repository.SaveHandoverLocations(ctx, actor, scheduleID, normalized, scope, meta); err != nil {
			return HandoverView{}, err
		}
	}
	return s.View(ctx, scheduleID, scope)
}

// SetOverride memindahkan satu penerima ke titik serah; locationID kosong
// mengembalikannya ke pembagian otomatis.
func (s *HandoverService) SetOverride(ctx context.Context, actor auth.Principal, scheduleID, slotID, locationID string, scope auth.RegencyScope, meta auth.ClientMeta) (HandoverView, error) {
	scheduleID, slotID, locationID = strings.TrimSpace(scheduleID), strings.TrimSpace(slotID), strings.TrimSpace(locationID)
	if scheduleID == "" || slotID == "" {
		return HandoverView{}, ErrInvalidInput
	}
	if err := s.repository.SetHandoverOverride(ctx, actor, scheduleID, slotID, locationID, scope, meta); err != nil {
		return HandoverView{}, err
	}
	return s.View(ctx, scheduleID, scope)
}

func sameHandoverLocations(current []HandoverLocation, inputs []HandoverLocationInput) bool {
	if len(current) != len(inputs) {
		return false
	}
	for i, input := range inputs {
		location := current[i]
		if input.ID != location.ID || input.Name != location.Name {
			return false
		}
		if len(input.Areas) == 0 && len(location.Areas) == 0 {
			continue
		}
		if !reflect.DeepEqual(input.Areas, location.Areas) {
			return false
		}
	}
	return true
}
