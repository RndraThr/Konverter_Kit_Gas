package distribution

import (
	"context"
	"errors"
	"testing"
)

type serialRepositoryStub struct {
	gotSerial, gotSchedule string
	gotSlot                int
}

func (r *serialRepositoryStub) FindSerialMatches(_ context.Context, serial, scheduleID string, slot int) ([]SerialMatch, error) {
	r.gotSerial, r.gotSchedule, r.gotSlot = serial, scheduleID, slot
	return []SerialMatch{{SlotNumber: 7}}, nil
}

func TestCheckSerialRejectsBlankAndOverlong(t *testing.T) {
	service := &Service{serialRepository: &serialRepositoryStub{}}
	for _, serial := range []string{"", "   ", string(make([]rune, 101))} {
		if _, err := service.CheckSerial(context.Background(), serial, "", 0); !errors.Is(err, ErrSerialInvalid) {
			t.Fatalf("CheckSerial(%q) err = %v, want ErrSerialInvalid", serial, err)
		}
	}
}

func TestCheckSerialNormalizesToUppercase(t *testing.T) {
	repo := &serialRepositoryStub{}
	service := &Service{serialRepository: repo}
	matches, err := service.CheckSerial(context.Background(), "  ms-a1 ", " s1 ", 3)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotSerial != "MS-A1" || repo.gotSchedule != "s1" || repo.gotSlot != 3 || len(matches) != 1 {
		t.Fatalf("repo got %+v, matches %+v", repo, matches)
	}
}
