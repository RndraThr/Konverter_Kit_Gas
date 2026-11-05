package distribution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
)

// Export nomor mesin: petugas memilih beberapa nomor bagi di katalog lalu
// mengunduh Excel berisi nomor bagi dan nomor mesinnya.

// MaxExportSlots caps the slot numbers of one export.
const MaxExportSlots = 5000

// MachineSerialRow is one slot of the export.
type MachineSerialRow struct {
	SlotNumber          int
	MachineSerialNumber string
}

// ExportFile is a generated workbook.
type ExportFile struct {
	Filename string
	Data     []byte
}

type machineSerialRepository interface {
	ListMachineSerials(ctx context.Context, scheduleID string, slotNumbers []int, scope auth.RegencyScope) (string, []MachineSerialRow, error)
}

// ExportMachineSerials builds the workbook for the chosen slot numbers of a
// schedule; numbers without a slot are left out.
func (s *Service) ExportMachineSerials(ctx context.Context, scheduleID string, slotNumbers []int, scope auth.RegencyScope) (ExportFile, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return ExportFile{}, ErrScheduleRequired
	}
	numbers := slices.Clone(slotNumbers)
	slices.Sort(numbers)
	numbers = slices.Compact(numbers)
	if len(numbers) == 0 || numbers[0] < 1 || len(numbers) > MaxExportSlots {
		return ExportFile{}, ErrSlotNumberRequired
	}
	repository, ok := s.posPenyerahanRepository.(machineSerialRepository)
	if !ok {
		return ExportFile{}, ErrSlotNotFound
	}
	regencyCode, rows, err := repository.ListMachineSerials(ctx, scheduleID, numbers, scope)
	if err != nil {
		return ExportFile{}, err
	}
	data, err := buildMachineSerialExcel(rows)
	if err != nil {
		return ExportFile{}, err
	}
	name := "nomor-mesin"
	if code := strings.TrimSpace(regencyCode); code != "" {
		name += "-" + code
	}
	return ExportFile{Filename: fmt.Sprintf("%s-%s.xlsx", name, time.Now().In(wib).Format("20060102")), Data: data}, nil
}

var wib = time.FixedZone("WIB", 7*60*60)

func buildMachineSerialExcel(rows []MachineSerialRow) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	const sheet = "Nomor Mesin"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("name export sheet: %w", err)
	}
	if err := file.SetSheetRow(sheet, "A1", &[]any{"No. Bagi", "No. Mesin"}); err != nil {
		return nil, fmt.Errorf("write export header: %w", err)
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+2)
		// The serial stays text so Excel keeps leading zeros and long digits.
		if err := file.SetSheetRow(sheet, cell, &[]any{row.SlotNumber, row.MachineSerialNumber}); err != nil {
			return nil, fmt.Errorf("write export row: %w", err)
		}
	}
	if style, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		_ = file.SetCellStyle(sheet, "A1", "B1", style)
	}
	if style, err := file.NewStyle(&excelize.Style{NumFmt: 49}); err == nil && len(rows) > 0 {
		_ = file.SetCellStyle(sheet, "B2", fmt.Sprintf("B%d", len(rows)+1), style)
	}
	_ = file.SetColWidth(sheet, "A", "A", 12)
	_ = file.SetColWidth(sheet, "B", "B", 28)
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode export excel: %w", err)
	}
	return buffer.Bytes(), nil
}

func (r *Repository) ListMachineSerials(ctx context.Context, scheduleID string, slotNumbers []int, scope auth.RegencyScope) (string, []MachineSerialRow, error) {
	var regencyCode string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(g.document_code, '') FROM program_schedules ps JOIN regencies g ON g.id = ps.regency_id
		WHERE ps.id = $1 AND ($2 OR ps.regency_id::text = ANY($3))
	`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&regencyCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrSlotNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("export schedule: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT slot_number, COALESCE(machine_serial_number, '') FROM distribution_slots
		WHERE schedule_id = $1 AND slot_number = ANY($2) ORDER BY slot_number
	`, scheduleID, slotNumbers)
	if err != nil {
		return "", nil, fmt.Errorf("list machine serials: %w", err)
	}
	defer rows.Close()
	result := []MachineSerialRow{}
	for rows.Next() {
		var row MachineSerialRow
		if err := rows.Scan(&row.SlotNumber, &row.MachineSerialNumber); err != nil {
			return "", nil, fmt.Errorf("scan machine serial: %w", err)
		}
		result = append(result, row)
	}
	return regencyCode, result, rows.Err()
}
