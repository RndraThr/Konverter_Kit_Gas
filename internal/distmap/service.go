package distmap

import (
	"context"
	"fmt"

	"konkit/internal/auth"
)

// ValidationError reports an invalid map-only query parameter (bbox or stage).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

// Validate checks the map-only parts of the query; the recipient filter is validated by the
// recipients contract that produced it.
func (q PointQuery) Validate() error {
	if q.Stage < 0 || q.Stage > StageSigned {
		return &ValidationError{Field: "stage", Message: "Tahap harus M1 sampai M5"}
	}
	if b := q.BBox; b != nil {
		if b[0] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 || b[0] > b[2] || b[1] > b[3] {
			return &ValidationError{Field: "bbox", Message: "Batas peta harus minLon,minLat,maxLon,maxLat"}
		}
	}
	return nil
}

type repository interface {
	Points(context.Context, PointQuery, auth.RegencyScope) ([]Point, PointsMeta, error)
	UnlocatedSlots(context.Context, PointQuery, auth.RegencyScope) ([]UnlocatedSlot, error)
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) Points(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]Point, PointsMeta, error) {
	if err := q.Validate(); err != nil {
		return nil, PointsMeta{}, err
	}
	return s.repository.Points(ctx, q, scope)
}

func (s *Service) UnlocatedSlots(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]UnlocatedSlot, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return s.repository.UnlocatedSlots(ctx, q, scope)
}
