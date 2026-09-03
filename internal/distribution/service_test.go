package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type repositoryStub struct {
	searchRecords []SearchRecord
	searchQuery   string
	searchLimit   int
	workspace     RecipientWorkspace
	saved         DraftInput
}

func (r *repositoryStub) Search(_ context.Context, _ string, query string, limit int) ([]SearchRecord, error) {
	r.searchQuery, r.searchLimit = query, limit
	return r.searchRecords, nil
}
func (r *repositoryStub) GetWorkspace(context.Context, string) (RecipientWorkspace, error) {
	return r.workspace, nil
}
func (r *repositoryStub) SaveDraft(_ context.Context, _ auth.Principal, _ string, input DraftInput, _ auth.ClientMeta) (RecipientWorkspace, error) {
	r.saved = input
	return r.workspace, nil
}

func TestSearchValidatesContextAndNameLength(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Search(context.Background(), "", "Siti", 20); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("missing schedule err=%v", err)
	}
	if _, err := service.Search(context.Background(), "schedule-1", "S", 20); !errors.Is(err, ErrQueryTooShort) {
		t.Fatalf("short name err=%v", err)
	}
	if _, err := service.Search(context.Background(), "schedule-1", "", 20); !errors.Is(err, ErrQueryRequired) {
		t.Fatalf("empty query err=%v", err)
	}
}

func TestSearchAllowsSingleDistributionNumberMasksNIKAndCapsResults(t *testing.T) {
	repository := &repositoryStub{searchRecords: []SearchRecord{{
		AllocationID: "allocation-1", DistributionNumber: 7, FullName: "Siti Aminah",
		NIK: "7306014101900001", Location: "Tempe, Wajo", ProgramType: "farmer", Eligibility: "eligible",
	}}}
	service := NewService(repository)

	results, err := service.Search(context.Background(), " schedule-1 ", " 7 ", 200)
	if err != nil {
		t.Fatal(err)
	}
	if repository.searchQuery != "7" || repository.searchLimit != 20 {
		t.Fatalf("query=%q limit=%d", repository.searchQuery, repository.searchLimit)
	}
	if len(results) != 1 || results[0].MaskedNIK != "7306********0001" {
		t.Fatalf("results=%+v", results)
	}
}

func TestSaveDraftRequiresReasonWhenNIKChanges(t *testing.T) {
	repository := &repositoryStub{workspace: RecipientWorkspace{NIK: "7306014101900001"}}
	service := NewService(repository)
	input := DraftInput{NIK: "7306014101900002"}

	_, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{})
	if !errors.Is(err, ErrIdentityChangeReasonRequired) {
		t.Fatalf("err=%v", err)
	}
	input.IdentityChangeReason = "Perbaikan berdasarkan KTP asli"
	if _, err := service.SaveDraft(context.Background(), auth.Principal{UserID: "user-1"}, "allocation-1", input, auth.ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if repository.saved.IdentityChangeReason == "" {
		t.Fatal("identity change reason was not persisted")
	}
}
