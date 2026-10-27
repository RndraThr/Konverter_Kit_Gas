package bast

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// BA Surat Pengantar Pengiriman Serah Terima Tabung Gas Elpiji. LPG untuk satu
// kabupaten biasanya datang beberapa kali, jadi tiap penerimaan adalah satu
// dokumen per tanggal (gas_elpiji) yang isiannya diketik petugas. Setelah semua
// penerimaan final, seluruhnya digabung urut tanggal menjadi satu BA Gas LPG
// (gas_elpiji_gabungan) yang disimpan dengan tanggal mulai jadwal sebagai
// kunci tetap, sehingga penggabungan ulang hanya menaikkan versinya.
const (
	AggregateDocumentGasElpiji       = "gas_elpiji"
	AggregateDocumentGasElpijiMerged = "gas_elpiji_gabungan"

	gasElpijiMaxCylinders = 1000000
	gasElpijiMaxText      = 300
)

var (
	// ErrGasElpijiIncomplete: isian wajib penerimaan belum lengkap saat finalisasi.
	ErrGasElpijiIncomplete = errors.New("gas elpiji receipt is incomplete")
	// ErrGasElpijiNoReceipts: belum ada penerimaan final untuk digabung.
	ErrGasElpijiNoReceipts = errors.New("no finalized gas elpiji receipts")
)

// GasElpijiInput adalah isian satu penerimaan LPG.
type GasElpijiInput struct {
	ScheduleID        string `json:"schedule_id"`
	DocumentDate      string `json:"document_date"`
	WarehouseLocation string `json:"warehouse_location"`
	WarehouseAddress  string `json:"warehouse_address"`
	PIC               string `json:"pic"`
	CylinderCount     int    `json:"cylinder_count"`
	Remarks           string `json:"remarks"`
	ReceiverName      string `json:"receiver_name"`
	PertaminaName     string `json:"pertamina_name"`
}

// normalize merapikan spasi lalu memeriksa batas isian. Isian kosong masih
// boleh (preview mencetaknya kosong); kelengkapan diperiksa oleh complete.
func (input *GasElpijiInput) normalize() error {
	input.ScheduleID = strings.TrimSpace(input.ScheduleID)
	input.DocumentDate = strings.TrimSpace(input.DocumentDate)
	for _, field := range []*string{&input.WarehouseLocation, &input.WarehouseAddress, &input.PIC, &input.Remarks, &input.ReceiverName, &input.PertaminaName} {
		*field = strings.Join(strings.Fields(*field), " ")
		if utf8.RuneCountInString(*field) > gasElpijiMaxText {
			return ErrInvalidInput
		}
	}
	if input.ScheduleID == "" || !validRequiredDate(input.DocumentDate) || input.CylinderCount < 0 || input.CylinderCount > gasElpijiMaxCylinders {
		return ErrInvalidInput
	}
	return nil
}

func (input GasElpijiInput) complete() bool {
	return input.WarehouseLocation != "" && input.WarehouseAddress != "" && input.PIC != "" &&
		input.CylinderCount > 0 && input.ReceiverName != "" && input.PertaminaName != ""
}

// GasElpijiSnapshot adalah snapshot_json satu penerimaan.
type GasElpijiSnapshot struct {
	DocumentType      string         `json:"document_type"`
	DocumentDate      string         `json:"document_date"`
	FiscalYear        int            `json:"fiscal_year"`
	RegencyName       string         `json:"regency_name"`
	WarehouseLocation string         `json:"warehouse_location"`
	WarehouseAddress  string         `json:"warehouse_address"`
	PIC               string         `json:"pic"`
	CylinderCount     int            `json:"cylinder_count"`
	Remarks           string         `json:"remarks"`
	ReceiverName      string         `json:"receiver_name"`
	ReceiverCompany   string         `json:"receiver_company"`
	PertaminaName     string         `json:"pertamina_name"`
	Logos             []LogoSnapshot `json:"logos"`
}

func (s GasElpijiSnapshot) input(scheduleID string) GasElpijiInput {
	return GasElpijiInput{
		ScheduleID: scheduleID, DocumentDate: s.DocumentDate, WarehouseLocation: s.WarehouseLocation,
		WarehouseAddress: s.WarehouseAddress, PIC: s.PIC, CylinderCount: s.CylinderCount, Remarks: s.Remarks,
		ReceiverName: s.ReceiverName, PertaminaName: s.PertaminaName,
	}
}

// GasElpijiMergedReceipt mengikat halaman gabungan ke dokumen penerimaan yang
// digabung (ID + checksum) agar perubahan penerimaan terdeteksi.
type GasElpijiMergedReceipt struct {
	DocumentID string            `json:"document_id"`
	Checksum   string            `json:"checksum"`
	Snapshot   GasElpijiSnapshot `json:"snapshot"`
}

// GasElpijiMergedSnapshot adalah snapshot_json BA Gas LPG gabungan.
type GasElpijiMergedSnapshot struct {
	DocumentType   string                   `json:"document_type"`
	Receipts       []GasElpijiMergedReceipt `json:"receipts"`
	TotalCylinders int                      `json:"total_cylinders"`
}

// GasElpijiReceipt adalah dokumen penerimaan aktif beserta isiannya.
type GasElpijiReceipt struct {
	Document AggregateDocument `json:"document"`
	Input    GasElpijiInput    `json:"input"`
	snapshot GasElpijiSnapshot
}

// GasElpijiSummary memberi panel daftar penerimaan dan status gabungan.
type GasElpijiSummary struct {
	Receipts       []GasElpijiReceipt `json:"receipts"`
	TotalCylinders int                `json:"total_cylinders"`
	Merged         *AggregateDocument `json:"merged"`
	// MergedCurrent: gabungan aktif memuat persis penerimaan aktif saat ini.
	MergedCurrent bool `json:"merged_current"`
}

func sortedLogoCopy(logos []LogoSnapshot) []LogoSnapshot {
	sorted := append([]LogoSnapshot(nil), logos...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].SortOrder < sorted[j].SortOrder })
	return sorted
}

func buildGasElpijiSnapshot(ctx DP3Context, settings ScheduleSettings, logos []LogoSnapshot, input GasElpijiInput) GasElpijiSnapshot {
	return GasElpijiSnapshot{
		DocumentType:      AggregateDocumentGasElpiji,
		DocumentDate:      input.DocumentDate,
		FiscalYear:        ctx.FiscalYear,
		RegencyName:       ctx.RegencyName,
		WarehouseLocation: input.WarehouseLocation,
		WarehouseAddress:  input.WarehouseAddress,
		PIC:               input.PIC,
		CylinderCount:     input.CylinderCount,
		Remarks:           input.Remarks,
		ReceiverName:      input.ReceiverName,
		ReceiverCompany:   settings.Consultant(),
		PertaminaName:     input.PertaminaName,
		Logos:             sortedLogoCopy(logos),
	}
}

func formatGasElpijiFilename(regencyName, documentDate string, version int) string {
	return fmt.Sprintf("BA GAS ELPIJI - %s - %s - V%d.pdf", strings.ToUpper(strings.Join(strings.Fields(regencyName), " ")), documentDate, version)
}

func formatGasElpijiMergedFilename(regencyName string, version int) string {
	return fmt.Sprintf("BA GAS ELPIJI GABUNGAN - %s - V%d.pdf", strings.ToUpper(strings.Join(strings.Fields(regencyName), " ")), version)
}

// formatThousands menulis 1250 sebagai "1.250".
func formatThousands(value int) string {
	digits := fmt.Sprintf("%d", value)
	var out strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(digit)
	}
	return out.String()
}
