package distmap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

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
		// NaN fails every comparison, so non-finite values are rejected explicitly.
		for _, v := range b {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return &ValidationError{Field: "bbox", Message: "Batas peta harus minLon,minLat,maxLon,maxLat"}
			}
		}
		if b[0] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 || b[0] > b[2] || b[1] > b[3] {
			return &ValidationError{Field: "bbox", Message: "Batas peta harus minLon,minLat,maxLon,maxLat"}
		}
	}
	return nil
}

type repository interface {
	Points(context.Context, PointQuery, auth.RegencyScope) ([]Point, PointsMeta, error)
	UnlocatedSlots(context.Context, PointQuery, auth.RegencyScope) ([]UnlocatedSlot, int, error)
	ZoneBoundaries(context.Context, string, auth.RegencyScope) ([]ZoneBoundary, error)
	RegencyBoundary(context.Context, string, auth.RegencyScope) (json.RawMessage, error)
}

type Service struct{ repository repository }

func NewService(repository repository) *Service { return &Service{repository: repository} }

func (s *Service) Points(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]Point, PointsMeta, error) {
	if err := q.Validate(); err != nil {
		return nil, PointsMeta{}, err
	}
	return s.repository.Points(ctx, q, scope)
}

// UnlocatedSlots returns at most UnlocatedLimit slots and the total number of unlocated slots.
func (s *Service) UnlocatedSlots(ctx context.Context, q PointQuery, scope auth.RegencyScope) ([]UnlocatedSlot, int, error) {
	if err := q.Validate(); err != nil {
		return nil, 0, err
	}
	return s.repository.UnlocatedSlots(ctx, q, scope)
}

func (s *Service) ZoneBoundaries(ctx context.Context, programID string, scope auth.RegencyScope) ([]ZoneBoundary, error) {
	return s.repository.ZoneBoundaries(ctx, programID, scope)
}

// RegencyBoundary returns the boundary of one kabupaten as a GeoJSON Feature, or ErrBoundaryNotFound
// when it has none or lies outside scope.
func (s *Service) RegencyBoundary(ctx context.Context, regencyID string, scope auth.RegencyScope) (json.RawMessage, error) {
	return s.repository.RegencyBoundary(ctx, regencyID, scope)
}
