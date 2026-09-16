package recipients

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type repositoryStub struct {
	createInput CreateInput
	updateInput UpdateInput
	actor       auth.Principal
	listFilter  Filter
	statsFilter Filter
}

func (r *repositoryStub) List(_ context.Context, filter Filter, _ auth.RegencyScope) (Page, error) {
	r.listFilter = filter
	return Page{Page: filter.Page, PageSize: filter.PageSize}, nil
}
func (r *repositoryStub) Stats(_ context.Context, filter Filter, _ auth.RegencyScope) (Stats, error) {
	r.statsFilter = filter
	return Stats{}, nil
}
func (r *repositoryStub) Create(_ context.Context, actor auth.Principal, input CreateInput, _ auth.ClientMeta, _ auth.RegencyScope) (Recipient, error) {
	r.actor, r.createInput = actor, input
	return Recipient{FullName: input.FullName}, nil
}
func (r *repositoryStub) Update(_ context.Context, actor auth.Principal, _ string, input UpdateInput, _ auth.ClientMeta, _ auth.RegencyScope) (Recipient, error) {
	r.actor, r.updateInput = actor, input
	return Recipient{FullName: input.FullName}, nil
}
func (r *repositoryStub) Cancel(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error {
	return nil
}
func (r *repositoryStub) Restore(context.Context, auth.Principal, string, auth.ClientMeta, auth.RegencyScope) error {
	return nil
}

func TestCreateValidatesFullNameAndNIK(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	actor := auth.Principal{UserID: "actor-1"}

	if _, err := service.Create(context.Background(), actor, CreateInput{FullName: "  "}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrFullNameRequired) {
		t.Fatalf("expected ErrFullNameRequired, got %v", err)
	}
	if _, err := service.Create(context.Background(), actor, CreateInput{FullName: "Budi", NIK: "123"}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("expected ErrNIKInvalid, got %v", err)
	}

	saved, err := service.Create(context.Background(), actor, CreateInput{FullName: "  Budi Santoso  ", NIK: "1234567890123456"}, auth.ClientMeta{}, auth.RegencyScope{})
	if err != nil {
		t.Fatal(err)
	}
	if saved.FullName != "Budi Santoso" || repository.createInput.FullName != "Budi Santoso" {
		t.Fatalf("expected trimmed full name forwarded, got %+v", repository.createInput)
	}
	if repository.actor.UserID != actor.UserID {
		t.Fatalf("expected actor forwarded, got %+v", repository.actor)
	}
}

func TestUpdateValidatesIdentitySameAsCreate(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	if _, err := service.Update(context.Background(), auth.Principal{}, "allocation-1", UpdateInput{FullName: ""}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrFullNameRequired) {
		t.Fatalf("expected ErrFullNameRequired, got %v", err)
	}
	if _, err := service.Update(context.Background(), auth.Principal{}, "allocation-1", UpdateInput{FullName: "Budi", NIK: "abc"}, auth.ClientMeta{}, auth.RegencyScope{}); !errors.Is(err, ErrNIKInvalid) {
		t.Fatalf("expected ErrNIKInvalid, got %v", err)
	}
}

func TestListNormalizesPagination(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	if _, err := service.List(context.Background(), Filter{Page: 0, PageSize: 0}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.Page != 1 || repository.listFilter.PageSize != 20 {
		t.Fatalf("expected defaults page=1 page_size=20, got %+v", repository.listFilter)
	}

	if _, err := service.List(context.Background(), Filter{Page: 3, PageSize: 500}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.Page != 3 || repository.listFilter.PageSize != 20 {
		t.Fatalf("expected page=3 clamped page_size=20, got %+v", repository.listFilter)
	}
}

func TestListNormalizesSortingToAnAllowlistedColumnAndDirection(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	if _, err := service.List(context.Background(), Filter{SortBy: "full_name", SortDirection: "asc"}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.SortBy != "full_name" || repository.listFilter.SortDirection != "asc" {
		t.Fatalf("expected valid sort preserved, got %+v", repository.listFilter)
	}

	if _, err := service.List(context.Background(), Filter{SortBy: "pa.id; DROP TABLE people", SortDirection: "sideways"}, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.listFilter.SortBy != "created_at" || repository.listFilter.SortDirection != "desc" {
		t.Fatalf("expected invalid sort normalized to created_at desc, got %+v", repository.listFilter)
	}
}

func TestStatsUsesTheSameNormalizedFiltersAsTheRecipientList(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	filter := Filter{Search: "Siti", RegencyID: "regency-1", ProgramID: "program-1", ScheduleID: "schedule-1", District: "Sabbangparu", EvidenceStatus: "partial"}

	if _, err := service.Stats(context.Background(), filter, auth.RegencyScope{}); err != nil {
		t.Fatal(err)
	}
	if repository.statsFilter.Search != "Siti" || repository.statsFilter.ScheduleID != "schedule-1" || repository.statsFilter.District != "Sabbangparu" || repository.statsFilter.EvidenceStatus != "partial" {
		t.Fatalf("expected combined filters forwarded to stats, got %+v", repository.statsFilter)
	}
}
