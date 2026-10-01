package bast

import (
	"context"
	"errors"
	"testing"
	"time"

	"konkit/internal/auth"
)

type repositoryStub struct {
	context               SourceContext
	slots                 []CompletedSlot
	lock                  LockResult
	contextErr, errorLock error
}

func (r *repositoryStub) GetSourceContext(context.Context, string, string, auth.RegencyScope) (SourceContext, error) {
	return r.context, r.contextErr
}
func (r *repositoryStub) ListCompletedSlots(context.Context, string, string, auth.RegencyScope) ([]CompletedSlot, error) {
	return r.slots, nil
}
func (r *repositoryStub) GetLockedTotal(context.Context, string, string) (LockResult, error) {
	return r.lock, r.errorLock
}
func (r *repositoryStub) LockRegencyTotal(context.Context, auth.Principal, SourceContext, auth.ClientMeta) (LockResult, error) {
	return LockResult{ProgramID: r.context.ProgramID, RegencyID: r.context.RegencyID, FinalTotal: r.context.SlotQuota}, nil
}
func (r *repositoryStub) ListActiveBundles(context.Context, string, string, auth.RegencyScope) ([]DailyBundle, error) {
	return nil, nil
}
func (r *repositoryStub) LoadSourceData(context.Context, RecipientDocument, SourceContext, auth.RegencyScope) (SourceData, error) {
	return SourceData{}, nil
}
func (r *repositoryStub) SaveFinalDocument(context.Context, auth.Principal, RecipientDocument, SourceData, Snapshot, auth.ClientMeta) (IndividualDocument, error) {
	return IndividualDocument{}, nil
}

func readyContext() SourceContext {
	return SourceContext{ProgramID: "program-1", RegencyID: "regency-1", ProgramType: "farmer", RegencyCode: "WJO", ZoneName: "Zona 1", Padding: 4, SlotQuota: 1578, DocumentSeries: "KSM-KKT", HasActiveLogo: true}
}

func TestBuildDocumentNumberUsesRegencyTotalAndRomanMonth(t *testing.T) {
	got, err := BuildDocumentNumber(NumberInput{SlotNumber: 102, FinalTotal: 1578, Padding: 4, DocumentSeries: "KSM-KKT", RegencyCode: "WJO", LocalDate: time.Date(2024, 12, 10, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if got != "0102/1578/KSM-KKT-WJO/XII/2024" {
		t.Fatalf("number=%q", got)
	}
}

func TestListRecipientsGroupsJakartaDateAndSortsNumerically(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Jakarta")
	repository := &repositoryStub{context: readyContext(), lock: LockResult{FinalTotal: 1578}, slots: []CompletedSlot{{ID: "100", SlotNumber: 100, DistributedAt: time.Date(2024, 12, 9, 18, 0, 0, 0, time.UTC)}, {ID: "1", SlotNumber: 1, DistributedAt: time.Date(2024, 12, 9, 17, 30, 0, 0, time.UTC)}, {ID: "10", SlotNumber: 10, DistributedAt: time.Date(2024, 12, 9, 18, 30, 0, 0, time.UTC)}}}
	items, err := NewService(repository, location).ListRecipients(context.Background(), "program-1", "regency-1", "2024-12-10", auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].SlotNumber != 1 || items[1].SlotNumber != 10 || items[2].SlotNumber != 100 {
		t.Fatalf("items=%+v", items)
	}
}

func TestLockRejectsOutOfRangeAndPlaceholder(t *testing.T) {
	contextData := readyContext()
	contextData.SlotQuota = 50
	repository := &repositoryStub{context: contextData, slots: []CompletedSlot{{SlotNumber: 51}}}
	service := NewService(repository, time.UTC)
	if _, err := service.LockRegencyTotal(context.Background(), auth.Principal{}, "program-1", "regency-1", auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); !errors.Is(err, ErrSlotOutOfRange) {
		t.Fatalf("err=%v", err)
	}
	repository.context.ZonePlaceholder = true
	if _, err := service.LockRegencyTotal(context.Background(), auth.Principal{}, "program-1", "regency-1", auth.RegencyScope{Unrestricted: true}, auth.ClientMeta{}); !errors.Is(err, ErrZoneNotConfigured) {
		t.Fatalf("placeholder err=%v", err)
	}
}

func TestListDatesReportsMissingLock(t *testing.T) {
	repository := &repositoryStub{context: readyContext(), errorLock: ErrNotFound, slots: []CompletedSlot{{SlotNumber: 1, DistributedAt: time.Date(2024, 12, 10, 1, 0, 0, 0, time.UTC)}}}
	dates, err := NewService(repository, time.UTC).ListDates(context.Background(), "program-1", "regency-1", auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 1 || dates[0].ValidationStatus != "total_not_locked" {
		t.Fatalf("dates=%+v", dates)
	}
}
