package distribution

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"konkit/internal/auth"

	"github.com/xuri/excelize/v2"
)

type machineSerialStub struct {
	penyerahanRepositoryStub
	numbers []int
	rows    []MachineSerialRow
}

func (r *machineSerialStub) ListMachineSerials(_ context.Context, _ string, numbers []int, _ auth.RegencyScope) (string, []MachineSerialRow, error) {
	r.numbers = numbers
	return "WNG", r.rows, nil
}

func TestExportMachineSerialsWritesSlotAndSerial(t *testing.T) {
	repo := &machineSerialStub{rows: []MachineSerialRow{{SlotNumber: 1, MachineSerialNumber: "0012345"}, {SlotNumber: 3}}}
	service := &Service{posPenyerahanRepository: repo}
	file, err := service.ExportMachineSerials(context.Background(), " schedule-1 ", []int{3, 1, 3}, auth.RegencyScope{Unrestricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(repo.numbers, []int{1, 3}) {
		t.Fatalf("numbers = %v", repo.numbers)
	}
	if !strings.HasPrefix(file.Filename, "nomor-mesin-WNG-") || !strings.HasSuffix(file.Filename, ".xlsx") {
		t.Fatalf("filename = %q", file.Filename)
	}
	book, err := excelize.OpenReader(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := book.GetRows("Nomor Mesin")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"No. Bagi", "No. Mesin"}, {"1", "0012345"}, {"3"}}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v", rows)
	}
	for i := range want {
		if !slices.Equal(rows[i], want[i]) {
			t.Fatalf("row %d = %v, want %v", i, rows[i], want[i])
		}
	}
}

func TestExportMachineSerialsNeedsScheduleAndSlots(t *testing.T) {
	service := &Service{posPenyerahanRepository: &machineSerialStub{}}
	if _, err := service.ExportMachineSerials(context.Background(), "", []int{1}, auth.RegencyScope{}); !errors.Is(err, ErrScheduleRequired) {
		t.Fatalf("err = %v", err)
	}
	for _, numbers := range [][]int{nil, {0, 2}} {
		if _, err := service.ExportMachineSerials(context.Background(), "schedule-1", numbers, auth.RegencyScope{}); !errors.Is(err, ErrSlotNumberRequired) {
			t.Fatalf("numbers %v: err = %v", numbers, err)
		}
	}
}
