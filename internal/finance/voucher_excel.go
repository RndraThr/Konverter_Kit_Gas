package finance

import (
	"bytes"
	"context"
	"fmt"
	"image"

	"konkit/internal/auth"

	"github.com/xuri/excelize/v2"
)

const (
	voucherSheet     = "Voucher"
	voucherLogoPixel = 130.0
	voucherLogoTall  = 76.0
)

var voucherExcelWidths = map[string]float64{"A": 6, "B": 14, "C": 54, "D": 18, "E": 16, "F": 26, "G": 19}

// Groups of the sheet's columns: the journal has six, the Bank, Kas, Lainnya
// box three, and the sign-off five.
var (
	voucherBoxColumns  = [][2]string{{"A", "C"}, {"D", "E"}, {"F", "G"}}
	voucherSignColumns = [][2]string{{"A", "B"}, {"C", "C"}, {"D", "E"}, {"F", "F"}, {"G", "G"}}
)

// VoucherExcel is the voucher page as a workbook, without the proofs.
func (s *Service) VoucherExcel(ctx context.Context, actor auth.Principal, access Access, id string) ([]byte, string, error) {
	voucher, err := s.Voucher(ctx, actor, access, id)
	if err != nil {
		return nil, "", err
	}
	settings, err := s.VoucherSettings(ctx)
	if err != nil {
		return nil, "", err
	}
	logo, err := s.Logo(ctx)
	if err != nil {
		return nil, "", err
	}
	data, err := buildVoucherExcel(voucher, settings, logo)
	if err != nil {
		return nil, "", err
	}
	return data, "Voucher_" + voucher.Number + ".xlsx", nil
}

func voucherBorder(sides ...string) []excelize.Border {
	borders := make([]excelize.Border, 0, len(sides))
	for _, side := range sides {
		borders = append(borders, excelize.Border{Type: side, Color: "000000", Style: 1})
	}
	return borders
}

// voucherStyles are the cell formats of the sheet, by name.
func voucherStyles(file *excelize.File) (map[string]int, error) {
	all := voucherBorder("left", "right", "top", "bottom")
	font := func(bold bool, size float64) *excelize.Font {
		return &excelize.Font{Family: "Arial", Size: size, Bold: bold, Color: "000000"}
	}
	align := func(horizontal string) *excelize.Alignment {
		return &excelize.Alignment{Horizontal: horizontal, Vertical: "center"}
	}
	money := "#,##0;-#,##0;"
	definitions := map[string]*excelize.Style{
		"text":     {Font: font(false, 9), Alignment: align("left"), Border: all},
		"label":    {Font: font(true, 9), Alignment: align("left"), Border: all},
		"centre":   {Font: font(false, 9), Alignment: align("center"), Border: all},
		"header":   {Font: font(true, 9), Alignment: align("center"), Border: all},
		"money":    {Font: font(false, 9), Alignment: align("right"), Border: all, CustomNumFmt: &money},
		"total":    {Font: font(true, 9), Alignment: align("right"), Border: all, CustomNumFmt: &money},
		"title":    {Font: font(true, 14), Alignment: align("center")},
		"company":  {Font: font(true, 11), Alignment: align("left"), Border: voucherBorder("top", "left", "right")},
		"address":  {Font: font(false, 9), Alignment: align("left"), Border: voucherBorder("left", "right")},
		"last":     {Font: font(false, 9), Alignment: align("left"), Border: voucherBorder("left", "right", "bottom")},
		"boxTitle": {Font: font(true, 8), Alignment: align("left"), Border: voucherBorder("top", "left", "right")},
		"signTop":  {Font: font(false, 9), Alignment: align("center"), Border: voucherBorder("top", "left", "right")},
		"signMid":  {Font: font(false, 9), Alignment: align("center"), Border: voucherBorder("left", "right")},
		"signEnd":  {Font: font(false, 9), Alignment: align("center"), Border: voucherBorder("left", "right", "bottom")},
	}
	styles := make(map[string]int, len(definitions))
	for name, definition := range definitions {
		id, err := file.NewStyle(definition)
		if err != nil {
			return nil, fmt.Errorf("style voucher sheet: %w", err)
		}
		styles[name] = id
	}
	return styles, nil
}

// voucherWriter fills merged cells; text goes in with SetCellStr, so a value
// such as =SUM(...) stays text.
type voucherWriter struct {
	file   *excelize.File
	styles map[string]int
	err    error
}

func (w *voucherWriter) put(first, last string, row int, value any, style string) {
	w.area(first, row, last, row, value, style)
}

func (w *voucherWriter) area(first string, top int, last string, bottom int, value any, style string) {
	if w.err != nil {
		return
	}
	from, to := fmt.Sprintf("%s%d", first, top), fmt.Sprintf("%s%d", last, bottom)
	if from != to {
		if w.err = w.file.MergeCell(voucherSheet, from, to); w.err != nil {
			return
		}
	}
	if w.err = w.file.SetCellStyle(voucherSheet, from, to, w.styles[style]); w.err != nil {
		return
	}
	switch typed := value.(type) {
	case nil:
	case string:
		w.err = w.file.SetCellStr(voucherSheet, from, typed)
	default:
		w.err = w.file.SetCellValue(voucherSheet, from, typed)
	}
}

func voucherAccountRows(settings VoucherSettings, accountID string) (map[string][]string, int) {
	rows := map[string][]string{}
	most := voucherBoxRows
	for _, account := range settings.Accounts {
		if !account.IsActive {
			continue
		}
		mark := "[ ]"
		if account.ID == accountID {
			mark = "[X]"
		}
		rows[account.Kind] = append(rows[account.Kind], mark+" "+account.boxLabel())
		most = max(most, len(rows[account.Kind]))
	}
	return rows, most
}

func buildVoucherExcel(voucher Voucher, settings VoucherSettings, logo []byte) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", voucherSheet); err != nil {
		return nil, fmt.Errorf("name voucher sheet: %w", err)
	}
	for column, width := range voucherExcelWidths {
		if err := file.SetColWidth(voucherSheet, column, column, width); err != nil {
			return nil, fmt.Errorf("size voucher sheet: %w", err)
		}
	}
	styles, err := voucherStyles(file)
	if err != nil {
		return nil, err
	}
	w := &voucherWriter{file: file, styles: styles}

	w.area("A", 1, "B", 4, nil, "text")
	w.put("C", "D", 1, settings.CompanyName, "company")
	w.put("C", "D", 2, settings.AddressLine1, "address")
	w.put("C", "D", 3, settings.AddressLine2, "address")
	w.put("C", "D", 4, settings.AddressLine3, "last")
	w.area("E", 1, "E", 2, "Date", "label")
	w.area("F", 1, "G", 2, voucherDate(voucher.Date), "text")
	w.area("E", 3, "E", 4, "No.", "label")
	w.area("F", 3, "G", 4, voucher.Number, "text")
	w.put("A", "G", 6, "PAYMENT VOUCHER", "title")

	accounts, boxRows := voucherAccountRows(settings, voucher.AccountID)
	row := 7
	for index, kind := range []string{AccountBank, AccountKas, AccountLainnya} {
		columns := voucherBoxColumns[index]
		w.put(columns[0], columns[1], row, []string{"BANK", "KAS", "LAINNYA"}[index], "boxTitle")
		for offset := range boxRows {
			text := ""
			if offset < len(accounts[kind]) {
				text = accounts[kind][offset]
			}
			style := "address"
			if offset == boxRows-1 {
				style = "last"
			}
			w.put(columns[0], columns[1], row+1+offset, text, style)
		}
	}
	row += boxRows + 1

	debit, kredit := voucherTotals(voucher.Lines)
	for _, field := range [][2]string{{"Pay To:", voucher.PayTo}, {"Total Amount:", "Rp" + voucherMoney(debit)}, {"Description:", voucher.Description}} {
		w.put("A", "B", row, field[0], "label")
		w.put("C", "G", row, field[1], "text")
		row++
	}
	row++

	w.put("A", "A", row, "NO.", "header")
	w.put("B", "C", row, "DESCRIPTION", "header")
	w.put("D", "D", row, "DEBIT", "header")
	w.put("E", "E", row, "KREDIT", "header")
	w.put("F", "F", row, "NOTE", "header")
	w.put("G", "G", row, "PROJECT CODE", "header")
	row++
	for index, line := range voucher.Lines {
		w.put("A", "A", row, index+1, "centre")
		w.put("B", "C", row, line.Description, "text")
		w.put("D", "D", row, line.Debit, "money")
		w.put("E", "E", row, line.Kredit, "money")
		w.put("F", "F", row, line.Note, "text")
		w.put("G", "G", row, line.ProjectCode, "text")
		row++
	}
	w.put("A", "A", row, nil, "text")
	w.put("B", "C", row, "TOTAL", "header")
	w.put("D", "D", row, debit, "total")
	w.put("E", "E", row, kredit, "total")
	w.put("F", "F", row, nil, "text")
	w.put("G", "G", row, nil, "text")
	row += 2

	for index, signer := range voucherSigners {
		columns := voucherSignColumns[index]
		w.put(columns[0], columns[1], row, signer[0], "signTop")
		w.put(columns[0], columns[1], row+1, nil, "signMid")
		w.put(columns[0], columns[1], row+2, nil, "signMid")
		w.put(columns[0], columns[1], row+3, "________________", "signMid")
		w.put(columns[0], columns[1], row+4, signer[1], "signEnd")
	}
	if w.err != nil {
		return nil, fmt.Errorf("write voucher sheet: %w", w.err)
	}
	if err := placeVoucherLogo(file, logo); err != nil {
		return nil, err
	}
	if err := fitVoucherPage(file); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := file.Write(&out); err != nil {
		return nil, fmt.Errorf("write voucher workbook: %w", err)
	}
	return out.Bytes(), nil
}

func placeVoucherLogo(file *excelize.File, logo []byte) error {
	if len(logo) == 0 {
		return nil
	}
	size, _, err := image.DecodeConfig(bytes.NewReader(logo))
	if err != nil {
		return nil // A logo that cannot be read must not stop the export.
	}
	scale := min(voucherLogoPixel/float64(size.Width), voucherLogoTall/float64(size.Height))
	extension := ".png"
	if imageType(logo) == "JPG" {
		extension = ".jpg"
	}
	picture := &excelize.Picture{Extension: extension, File: logo, Format: &excelize.GraphicOptions{ScaleX: scale, ScaleY: scale, OffsetX: 6, OffsetY: 6, Positioning: "oneCell"}}
	if err := file.AddPictureFromBytes(voucherSheet, "A1", picture); err != nil {
		return fmt.Errorf("place voucher logo: %w", err)
	}
	return nil
}

// fitVoucherPage prints the sheet on landscape A4, one page wide.
func fitVoucherPage(file *excelize.File) error {
	orientation, size, wide, tall := "landscape", 9, 1, 0
	if err := file.SetPageLayout(voucherSheet, &excelize.PageLayoutOptions{Orientation: &orientation, Size: &size, FitToWidth: &wide, FitToHeight: &tall}); err != nil {
		return fmt.Errorf("lay out voucher sheet: %w", err)
	}
	fit, grid := true, false
	if err := file.SetSheetProps(voucherSheet, &excelize.SheetPropsOptions{FitToPage: &fit}); err != nil {
		return fmt.Errorf("lay out voucher sheet: %w", err)
	}
	if err := file.SetSheetView(voucherSheet, 0, &excelize.ViewOptions{ShowGridLines: &grid}); err != nil {
		return fmt.Errorf("lay out voucher sheet: %w", err)
	}
	return nil
}
