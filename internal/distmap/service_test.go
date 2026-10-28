package distmap

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"konkit/internal/auth"
)

type fakeRepository struct{ called bool }

func (f *fakeRepository) Points(context.Context, PointQuery, auth.RegencyScope) ([]Point, PointsMeta, error) {
	f.called = true
	return nil, PointsMeta{}, nil
}

func (f *fakeRepository) UnlocatedSlots(context.Context, PointQuery, auth.RegencyScope) ([]UnlocatedSlot, int, error) {
	f.called = true
	return nil, 0, nil
}

func (f *fakeRepository) ZoneBoundaries(context.Context, string, auth.RegencyScope) ([]ZoneBoundary, error) {
	f.called = true
	return nil, nil
}

func (f *fakeRepository) RegencyBoundary(context.Context, string, auth.RegencyScope) (json.RawMessage, error) {
	f.called = true
	return nil, nil
}

func TestServiceRejectsInvalidStageAndBBox(t *testing.T) {
	cases := []struct {
		name  string
		query PointQuery
		field string
	}{
		{"stage too high", PointQuery{Stage: 6}, "stage"},
		{"negative stage", PointQuery{Stage: -1}, "stage"},
		{"inverted bbox", PointQuery{BBox: &[4]float64{109, -6, 108, -7}}, "bbox"},
		{"bbox out of range", PointQuery{BBox: &[4]float64{-200, -6, 108, -5}}, "bbox"},
		{"NaN bbox", PointQuery{BBox: &[4]float64{math.NaN(), -7, 109, -6}}, "bbox"},
		{"NaN max bbox", PointQuery{BBox: &[4]float64{108, -7, math.NaN(), -6}}, "bbox"},
		{"Inf bbox", PointQuery{BBox: &[4]float64{108, -7, math.Inf(1), -6}}, "bbox"},
		{"valid", PointQuery{Stage: 5, BBox: &[4]float64{108, -7, 109, -6}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			_, _, err := NewService(repo).Points(context.Background(), tc.query, auth.RegencyScope{})
			var validation *ValidationError
			if tc.field == "" {
				if err != nil || !repo.called {
					t.Fatalf("valid query must reach the repository, err=%v", err)
				}
				return
			}
			if !errors.As(err, &validation) || validation.Field != tc.field || repo.called {
				t.Fatalf("want validation error on %s without a repository call, got %v", tc.field, err)
			}
		})
	}
}
