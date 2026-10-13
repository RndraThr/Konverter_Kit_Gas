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
}

func (r *slotRepositoryStub) ListSlots(_ context.Context, scheduleID string) ([]SupportingDocumentSlot, error) {
	items := make([]SupportingDocumentSlot, 0)
	for _, s := range r.slots {
		if s.ScheduleID == scheduleID && s.IsActive {
			items = append(items, s)
		}
	}
	return items, nil
}

func (r *slotRepositoryStub) InsertSlot(_ context.Context, actor auth.Principal, scheduleID, name string, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	r.insertCalls++
	slot := SupportingDocumentSlot{ID: "new-slot", ScheduleID: scheduleID, Name: name, IsActive: true}
	r.slots = append(r.slots, slot)
	return slot, nil
}

func (r *slotRepositoryStub) PatchSlotRow(_ context.Context, actor auth.Principal, id string, name *string, isActive *bool, meta auth.ClientMeta) (SupportingDocumentSlot, error) {
	r.patchCalls++
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
	_, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "   ", auth.ClientMeta{})
	if !errors.Is(err, ErrSlotNameRequired) {
		t.Fatalf("err = %v, want ErrSlotNameRequired", err)
	}
}

func TestCreateAndListSlots(t *testing.T) {
	repo := &slotRepositoryStub{}
	service := &Service{slotRepository: repo}
	created, err := service.CreateSlot(context.Background(), auth.Principal{}, "schedule-1", "Surat Tugas", auth.ClientMeta{})
	if err != nil {
		t.Fatalf("CreateSlot() error = %v", err)
	}
	if created.Name != "Surat Tugas" || created.ScheduleID != "schedule-1" {
		t.Fatalf("created = %+v, want name/schedule matching input", created)
	}
	list, err := service.ListSlots(context.Background(), "schedule-1")
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
	updated, err := service.PatchSlot(context.Background(), auth.Principal{}, "slot-1", nil, &inactive, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("PatchSlot() error = %v", err)
	}
	if updated.IsActive {
		t.Fatal("slot should be inactive after PatchSlot(isActive=false)")
	}
}
