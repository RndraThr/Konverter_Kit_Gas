package distribution

import (
	"context"
	"errors"
	"testing"

	"konkit/internal/auth"
)

type serialRepositoryStub struct {
	gotSerial, gotSchedule string
	gotSlot                int
	gotScope               auth.RegencyScope
}

func (r *serialRepositoryStub) FindSerialMatches(_ context.Context, serial, scheduleID string, slot int, scope auth.RegencyScope) ([]SerialMatch, error) {
	r.gotSerial, r.gotSchedule, r.gotSlot, r.gotScope = serial, scheduleID, slot, scope
	return []SerialMatch{{SlotNumber: 7}}, nil
}

func TestCheckSerialRejectsBlankAndOverlong(t *testing.T) {
	service := &Service{serialRepository: &serialRepositoryStub{}}
	for _, serial := range []string{"", "   ", string(make([]rune, 101))} {
		if _, err := service.CheckSerial(context.Background(), serial, "", 0, auth.RegencyScope{Unrestricted: true}); !errors.Is(err, ErrSerialInvalid) {
			t.Fatalf("CheckSerial(%q) err = %v, want ErrSerialInvalid", serial, err)
		}
	}
}

func TestCheckSerialNormalizesAndForwardsScope(t *testing.T) {
	repo := &serialRepositoryStub{}
	service := &Service{serialRepository: repo}
	scope := auth.RegencyScope{RegencyIDs: []string{"regency-1"}}
	matches, err := service.CheckSerial(context.Background(), "  ms-a1 ", " s1 ", 3, scope)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotSerial != "MS-A1" || repo.gotSchedule != "s1" || repo.gotSlot != 3 || len(matches) != 1 {
		t.Fatalf("repo got %+v, matches %+v", repo, matches)
	}
	if repo.gotScope.Unrestricted || len(repo.gotScope.RegencyIDs) != 1 || repo.gotScope.RegencyIDs[0] != "regency-1" {
		t.Fatalf("scope not forwarded: %+v", repo.gotScope)
	}
}
