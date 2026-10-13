package activities

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type slotRepositoryStub struct {
	slots       []SupportingDocumentSlot
	insertCalls int
	patchCalls  int
	lastScope   auth.RegencyScope
}

func (r *slotRepositoryStub) ListSlots(_ context.Context, scheduleID string, scope auth.RegencyScope) ([]SupportingDocumentSlot, error) {
	r.lastScope = scope
	items := make([]SupportingDocumentSlot, 0)
	for _, s := range r.slots {
		if s.ScheduleID == scheduleID && s.IsActive {
			items = append(items, s)
		}
	}
	return items, nil
}

func (r *slotRepositoryStub) InsertSlot(_ context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	r.insertCalls++
	r.lastScope = scope
	slot := SupportingDocumentSlot{ID: "new-slot", ScheduleID: scheduleID, Name: name, IsActive: true}
	r.slots = append(r.slots, slot)
	return slot, nil
}

func (r *slotRepositoryStub) PatchSlotRow(_ context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta, scope auth.RegencyScope) (SupportingDocumentSlot, error) {
	r.patchCalls++
	r.lastScope = scope
	for i, s := range r.slots {
		if s.ID != id {
			continue
		}
		if name != nil {
			r.slots[i].Name = *name
		}
		if isActive != nil {
			r.slots[i].IsActive = *isActive
		}
		return r.slots[i], nil
	}
	return SupportingDocumentSlot{}, ErrSlotNotFound
}

func TestCreateSlotRejectsBlankName(t *testing.T) {
	service := &Service{slotRepository: &slotRepositoryStub{}}
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "   ", auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if !errors.Is(err, ErrSlotNameRequired) {
		t.Fatalf("err = %v, want ErrSlotNameRequired", err)
	}
}

func TestCreateAndListSlots(t *testing.T) {
	repo := &slotRepositoryStub{}
	service := &Service{slotRepository: repo}
	scope := auth.RegencyScope{Unrestricted: true}
	created, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "Surat Tugas", auth.ClientMeta{}, scope)
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if created.Name != "Surat Tugas" || created.ScheduleID != "schedule-1" {
		t.Fatalf("created = %+v, want name/schedule matching input", created)
	}
	list, err := service.ListSlots(context.Background(), "schedule-1", scope)
	if err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "Surat Tugas" {
		t.Fatalf("list = %+v, want one slot named Surat Tugas", list)
	}
}

func TestPatchSlotDeactivates(t *testing.T) {
	repo := &slotRepositoryStub{slots: []SupportingDocumentSlot{{ID: "slot-1", ScheduleID: "schedule-1", Name: "SK Penetapan", IsActive: true}}}
	service := &Service{slotRepository: repo}
	inactive := false
	updated, err := service.PatchSlot(context.Background(), auth.Principal{}, "slot-1", nil, &inactive, auth.ClientMeta{}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatalf("PatchSlot() error = %v", err)
	}
	if updated.IsActive {
		t.Fatal("slot should be inactive after PatchSlot(isActive=false)")
	}
}

// TestServiceThreadsScopeToRepository is a unit-level proof that Service.ListSlots/CreateSlot/PatchSlot
// pass the caller's scope through to the repository unmodified, rather than dropping or substituting
// it. The stub has no concept of regencies, so it cannot prove enforcement itself — that proof is the
// job of TestInsertAndListSlotsScopedBySchedule in repository_integration_test.go, which runs the real
// SQL against a real Postgres database. This test only proves the plumbing between Service and
// slotRepository is correct.
func TestServiceThreadsScopeToRepository(t *testing.T) {
	repo := &slotRepositoryStub{slots: []SupportingDocumentSlot{{ID: "slot-1", ScheduleID: "schedule-1", Name: "SK Penetapan", IsActive: true}}}
	service := &Service{slotRepository: repo}
	restricted := auth.RegencyScope{RegencyIDs: []string{"regency-in-scope"}}

	if _, err := service.ListSlots(context.Background(), "schedule-1", restricted); err != nil {
		t.Fatalf("ListSlots() error = %v", err)
	}
	if repo.lastScope.Unrestricted != restricted.Unrestricted || len(repo.lastScope.RegencyIDs) != 1 || repo.lastScope.RegencyIDs[0] != "regency-in-scope" {
		t.Fatalf("ListSlots did not thread scope through: got %+v, want %+v", repo.lastScope, restricted)
	}

	if _, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "Surat Tugas", auth.ClientMeta{}, restricted); err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if repo.lastScope.Unrestricted != restricted.Unrestricted || len(repo.lastScope.RegencyIDs) != 1 || repo.lastScope.RegencyIDs[0] != "regency-in-scope" {
		t.Fatalf("CreateSlot did not thread scope through: got %+v, want %+v", repo.lastScope, restricted)
	}

	inactive := false
	if _, err := service.PatchSlot(context.Background(), auth.Principal{}, "slot-1", nil, &inactive, auth.ClientMeta{}, restricted); err != nil {
		t.Fatalf("PatchSlot() error = %v", err)
	}
	if repo.lastScope.Unrestricted != restricted.Unrestricted || len(repo.lastScope.RegencyIDs) != 1 || repo.lastScope.RegencyIDs[0] != "regency-in-scope" {
		t.Fatalf("PatchSlot did not thread scope through: got %+v, want %+v", repo.lastScope, restricted)
	}
}
