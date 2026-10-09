package audit

import "testing"

func TestNormalizeFilterAppliesSafePagination(t *testing.T) {
	tests := []struct {
		name     string
		input    Filter
		wantPage int
		wantSize int
	}{
		{name: "defaults", input: Filter{}, wantPage: 1, wantSize: 20},
		{name: "maximum", input: Filter{Page: 2, PageSize: 500}, wantPage: 2, wantSize: 100},
		{name: "provided", input: Filter{Page: 3, PageSize: 25}, wantPage: 3, wantSize: 25},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeFilter(test.input)
			if got.Page != test.wantPage || got.PageSize != test.wantSize {
				t.Fatalf("normalizeFilter(%+v) = %+v", test.input, got)
			}
		})
	}
}

func TestNormalizeFilterTrimsSearchFields(t *testing.T) {
	got := normalizeFilter(Filter{
		Query: "  login  ", Action: " user.updated ", ResourceType: " users ",
		ActorUserID: " actor-id ", Actor: " rendra ", DateFrom: " 2026-10-01 ", DateTo: " 2026-10-09 ",
	})
	if got.Query != "login" || got.Action != "user.updated" || got.ResourceType != "users" ||
		got.ActorUserID != "actor-id" || got.Actor != "rendra" || got.DateFrom != "2026-10-01" || got.DateTo != "2026-10-09" {
		t.Fatalf("search fields were not normalized: %+v", got)
	}
}
