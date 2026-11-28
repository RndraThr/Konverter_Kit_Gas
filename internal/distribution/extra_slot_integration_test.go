package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

func TestIntegrationExtraSlotAddAndRemove(t *testing.T) {
	pool := distributionIntegrationPool(t)
	fixture := seedMediaFixture(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	scope := auth.RegencyScope{Unrestricted: true}
	input := ExtraSlotInput{ScheduleID: fixture.scheduleID, SlotNumber: fixture.slotNumber, Code: "power_of_attorney"}

	// Without a KTP slot there is nothing to attach Surat Kuasa to.
	if _, err := repo.AddExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope); !errors.Is(err, ErrExtraSlotUnknown) {
		t.Fatalf("add without anchor err=%v", err)
	}

	must(t, func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO documentation_slots(distribution_slot_id,slot_code,label_snapshot,stage,is_required,min_files,max_files,input_source,require_location,require_captured_at,sort_order)
			VALUES($1,'recipient_id_card','Foto KTP dan Nomor Urut','dokumen',true,1,1,'camera',true,false,30)
		`, fixture.distributionSlotID)
		return err
	}())

	added, err := repo.AddExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope)
	if err != nil {
		t.Fatal(err)
	}
	extra := findSummary(added.Documentation, "power_of_attorney")
	if extra == nil {
		t.Fatalf("extra slot missing: %+v", added.Documentation)
	}
	if extra.Label != "Foto Surat Kuasa dan Nomor Urut" || extra.Stage != "dokumen" || !extra.Required || extra.MinFiles != 1 || extra.MaxFiles != 2 || extra.InputSource != "camera" || !extra.RequireLocation {
		t.Fatalf("extra slot=%+v", *extra)
	}
	// It sits right after KTP.
	for i, slot := range added.Documentation {
		if slot.Code == "recipient_id_card" && (i+1 >= len(added.Documentation) || added.Documentation[i+1].Code != "power_of_attorney") {
			t.Fatalf("order=%+v", added.Documentation)
		}
	}

	// Adding again keeps the one slot.
	again, err := repo.AddExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if count := countSummaries(again.Documentation, "power_of_attorney"); count != 1 {
		t.Fatalf("slots after second add=%d", count)
	}

	// A slot with a photo cannot be removed.
	must(t, func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO media_files(documentation_slot_id,storage_key,original_filename,mime_type,byte_size,checksum,source,status)
			VALUES($1,gen_random_uuid(),'kuasa.jpg','image/jpeg',1024,repeat('b',64),'camera','accepted')
		`, extra.ID)
		return err
	}())
	if _, err := repo.RemoveExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope); !errors.Is(err, ErrExtraSlotHasMedia) {
		t.Fatalf("remove with media err=%v", err)
	}
	// Deleting the photo only marks it; the slot then counts as empty, as
	// the officer sees it (0 photos).
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE media_files SET status='deleted' WHERE documentation_slot_id=$1`, extra.ID)
		return err
	}())

	removed, err := repo.RemoveExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if findSummary(removed.Documentation, "power_of_attorney") != nil {
		t.Fatalf("extra slot still present: %+v", removed.Documentation)
	}

	// Completed distributions keep their documentation as it is.
	must(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE distribution_slots SET status='completed' WHERE id=$1`, fixture.distributionSlotID)
		return err
	}())
	if _, err := repo.AddExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, scope); !errors.Is(err, ErrAlreadyCompleted) {
		t.Fatalf("add on completed err=%v", err)
	}

	// Other regencies cannot reach the slot.
	if _, err := repo.AddExtraSlot(ctx, auth.Principal{}, input, auth.ClientMeta{}, auth.RegencyScope{RegencyIDs: []string{fixture.otherRegencyID}}); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("add out of scope err=%v", err)
	}
}

func findSummary(slots []SlotSummary, code string) *SlotSummary {
	for i := range slots {
		if slots[i].Code == code {
			return &slots[i]
		}
	}
	return nil
}

func countSummaries(slots []SlotSummary, code string) int {
	count := 0
	for _, slot := range slots {
		if slot.Code == code {
			count++
		}
	}
	return count
}
