package finance

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // decodes PNG proofs
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const (
	voucherMargin      = 12.0
	voucherWidth       = 273.0 // A4 landscape less the margins
	voucherRowHeight   = 5.6
	voucherFirstRows   = 12
	voucherLaterRows   = 29
	voucherFooterRows  = 6
	voucherSignHeight  = 23.0
	voucherBoxRows     = 8
	voucherBoxRow      = 5.3
	proofMargin        = 15.0
	proofHeaderBottom  = 33.0
	proofContentBottom = 282.0
)

var (
	voucherColumns = []float64{12.5, 125, 35.5, 30, 40.5, 29.5}
	voucherHeaders = []string{"NO.", "DESCRIPTION", "DEBIT", "KREDIT", "NOTE", "PROJECT CODE"}
	voucherSigners = [][2]string{{"Prepared By,", "Admin"}, {"Checked By,", "Tax"}, {"Checked By,", "Accounting"}, {"Approved By,", "Manager"}, {"System Entered By,", "Admin/Accounting"}}
)

type voucherDocument struct {
	Number      string
	Date        string
	PayTo       string
	Description string
	Settings    VoucherSettings
	AccountID   string
	Logo        []byte
	Lines       []VoucherLine
}

// ProofError names a proof the bundle cannot read.
type ProofError struct{ Filename string }

func (e *ProofError) Error() string {
	return "voucher proof cannot be read: " + e.Filename
}

type bundleProof struct {
	Title       string
	Requester   string
	Filename    string
	ContentType string
	Data        []byte
}

func voucherMoney(value int64) string {
	if value == 0 {
		return ""
	}
	digits := strconv.FormatInt(value, 10)
	var out strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(digit)
	}
	return out.String()
}

func voucherDate(value string) string {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return value
	}
	return parsed.Format("02-01-2006")
}

// fitText shortens text to one line of the column, as the original voucher.
func fitText(pdf *fpdf.Fpdf, text string, width float64) string {
	if pdf.GetStringWidth(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && pdf.GetStringWidth(string(runes)) > width {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ")
}

// renderVoucher draws the voucher pages: landscape A4 with the letterhead and
// bank boxes on the first, then the journal and the sign-off.
func renderVoucher(doc voucherDocument) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(voucherMargin, voucherMargin, voucherMargin)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pages := voucherPages(len(doc.Lines), voucherFirstRows-max(voucherBoxHeight(doc)-voucherBoxRows, 0))
	for page, rows := range pages {
		pdf.AddPage()
		if page == 0 {
			drawVoucherHead(pdf, doc, tr)
		} else {
			pdf.SetFont("Helvetica", "B", 12)
			pdf.SetXY(voucherMargin, 12)
			pdf.CellFormat(voucherWidth, 6, "PAYMENT VOUCHER (lanjutan)", "", 1, "C", false, 0, "")
			pdf.SetFont("Helvetica", "", 8.5)
			pdf.CellFormat(voucherWidth, 5, tr(fmt.Sprintf("No. %s  |  Tanggal %s  |  Halaman %d/%d", doc.Number, voucherDate(doc.Date), page+1, len(pages))), "", 1, "C", false, 0, "")
			pdf.SetY(pdf.GetY() + 3)
		}
		drawVoucherTable(pdf, doc, rows, tr)
		if page == len(pages)-1 {
			drawVoucherFooter(pdf, doc, tr)
		}
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render voucher: %w", err)
	}
	return out.Bytes(), nil
}

// voucherPages splits the rows. The first page has the letterhead, so it holds
// fewer, and the last must keep room for the total and the sign-off.
func voucherPages(count, firstRoom int) [][2]int {
	pages := [][2]int{}
	start, room := 0, firstRoom
	for {
		remaining := count - start
		if remaining+voucherFooterRows <= room {
			return append(pages, [2]int{start, count})
		}
		take := min(room, remaining)
		if take == remaining {
			// The footer needs the next page: at least one row goes with it.
			take--
		}
		pages = append(pages, [2]int{start, start + take})
		start += take
		room = voucherLaterRows
	}
}

// voucherBoxHeight is the row count of the Bank, Kas, Lainnya box.
func voucherBoxHeight(doc voucherDocument) int {
	rows := voucherBoxRows
	for _, kind := range []string{AccountBank, AccountKas, AccountLainnya} {
		count := 0
		for _, account := range doc.Settings.Accounts {
			if account.IsActive && account.Kind == kind {
				count++
			}
		}
		rows = max(rows, count)
	}
	return rows
}

func drawVoucherHead(pdf *fpdf.Fpdf, doc voucherDocument, tr func(string) string) {
	const top, headHeight, logoWidth, labelWidth, valueWidth = 12.0, 24.7, 33.6, 21.0, 100.0
	left := voucherMargin
	panel := left + voucherWidth - labelWidth - valueWidth
	pdf.SetLineWidth(0.25)
	pdf.Rect(left, top, voucherWidth, headHeight, "D")
	pdf.Line(left+logoWidth, top, left+logoWidth, top+headHeight)
	pdf.Line(panel, top, panel, top+headHeight)
	pdf.Line(panel+labelWidth, top, panel+labelWidth, top+headHeight)
	pdf.Line(panel, top+9.3, left+voucherWidth, top+9.3)
	if len(doc.Logo) > 0 {
		options := fpdf.ImageOptions{ImageType: imageType(doc.Logo), ReadDpi: false}
		if info := pdf.RegisterImageOptionsReader("voucher-logo", options, bytes.NewReader(doc.Logo)); info != nil && pdf.Error() == nil {
			width, height := fitBox(info.Width(), info.Height(), logoWidth-2, headHeight-6)
			pdf.ImageOptions("voucher-logo", left+(logoWidth-width)/2, top+(headHeight-height)/2, width, height, false, options, 0, "")
		}
	}
	pdf.SetXY(left+logoWidth+2.8, top+2.5)
	pdf.SetFont("Helvetica", "B", 10.5)
	pdf.CellFormat(112, 5.2, tr(doc.Settings.CompanyName), "", 2, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.8)
	for _, line := range []string{doc.Settings.AddressLine1, doc.Settings.AddressLine2, doc.Settings.AddressLine3} {
		pdf.CellFormat(112, 4.7, tr(line), "", 2, "L", false, 0, "")
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetXY(panel+2.8, top+2.3)
	pdf.CellFormat(labelWidth-3, 5, "Date", "", 0, "L", false, 0, "")
	pdf.SetXY(panel+2.8, top+11.5)
	pdf.CellFormat(labelWidth-3, 5, "No.", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(panel+labelWidth+2.8, top+2.3)
	pdf.CellFormat(valueWidth-5, 5, voucherDate(doc.Date), "", 0, "L", false, 0, "")
	pdf.SetXY(panel+labelWidth+2.8, top+11.5)
	pdf.CellFormat(valueWidth-5, 5, tr(doc.Number), "", 0, "L", false, 0, "")

	pdf.SetXY(left, top+headHeight+2.5)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(voucherWidth, 7.5, "PAYMENT VOUCHER", "", 1, "C", false, 0, "")
	boxTop := pdf.GetY() + 1
	columnWidth := voucherWidth / 3
	boxHeight := 6.0 + voucherBoxRow*float64(voucherBoxHeight(doc))
	pdf.Rect(left, boxTop, voucherWidth, boxHeight, "D")
	pdf.Line(left+columnWidth, boxTop, left+columnWidth, boxTop+boxHeight)
	pdf.Line(left+columnWidth*2, boxTop, left+columnWidth*2, boxTop+boxHeight)
	for index, kind := range []string{AccountBank, AccountKas, AccountLainnya} {
		x := left + columnWidth*float64(index) + 2.8
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetXY(x, boxTop+1.3)
		pdf.CellFormat(40, 3.5, strings.ToUpper(kind), "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8.5)
		row := 0
		for _, account := range doc.Settings.Accounts {
			if !account.IsActive || account.Kind != kind {
				continue
			}
			mark := "[ ]"
			if account.ID == doc.AccountID {
				mark = "[X]"
			}
			pdf.SetXY(x, boxTop+5.2+voucherBoxRow*float64(row))
			pdf.CellFormat(columnWidth-4, voucherBoxRow, tr(mark+" "+account.boxLabel()), "", 0, "L", false, 0, "")
			row++
		}
	}

	debit, _ := voucherTotals(doc.Lines)
	y := boxTop + boxHeight
	for _, field := range [][2]string{{"Pay To:", doc.PayTo}, {"Total Amount:", "Rp" + voucherMoney(debit)}, {"Description:", doc.Description}} {
		pdf.Rect(left, y, voucherWidth, 6.3, "D")
		pdf.Line(left+42.4, y, left+42.4, y+6.3)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetXY(left+2.8, y+0.7)
		pdf.CellFormat(38, 5, field[0], "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetXY(left+44.5, y+0.7)
		pdf.CellFormat(voucherWidth-46, 5, fitText(pdf, tr(field[1]), voucherWidth-47), "", 0, "L", false, 0, "")
		y += 6.3
	}
	pdf.SetXY(left, y+3.5)
}

func drawVoucherRow(pdf *fpdf.Fpdf, cells []string, bold bool) {
	aligns := []string{"C", "L", "R", "R", "L", "L"}
	y, x := pdf.GetY(), voucherMargin
	style := ""
	if bold {
		style = "B"
	}
	pdf.SetFont("Helvetica", style, 7.8)
	for index, width := range voucherColumns {
		align := aligns[index]
		if bold && len(cells[1]) > 0 && index == 1 {
			align = "C"
		}
		pdf.SetXY(x, y)
		pdf.CellFormat(width, voucherRowHeight, fitText(pdf, cells[index], width-2), "1", 0, align, false, 0, "")
		x += width
	}
	pdf.SetXY(voucherMargin, y+voucherRowHeight)
}

func drawVoucherTable(pdf *fpdf.Fpdf, doc voucherDocument, rows [2]int, tr func(string) string) {
	x, y := voucherMargin, pdf.GetY()
	pdf.SetFont("Helvetica", "B", 9)
	for index, width := range voucherColumns {
		pdf.SetXY(x, y)
		pdf.CellFormat(width, 6.4, voucherHeaders[index], "1", 0, "C", false, 0, "")
		x += width
	}
	pdf.SetXY(voucherMargin, y+6.4)
	for index := rows[0]; index < rows[1]; index++ {
		line := doc.Lines[index]
		drawVoucherRow(pdf, []string{strconv.Itoa(index + 1), tr(line.Description), voucherMoney(line.Debit), voucherMoney(line.Kredit), tr(line.Note), tr(line.ProjectCode)}, false)
	}
}

func drawVoucherFooter(pdf *fpdf.Fpdf, doc voucherDocument, tr func(string) string) {
	debit, kredit := voucherTotals(doc.Lines)
	drawVoucherRow(pdf, []string{"", "TOTAL", voucherMoney(debit), voucherMoney(kredit), "", ""}, true)
	y := pdf.GetY() + 4
	width := voucherWidth / float64(len(voucherSigners))
	for index, signer := range voucherSigners {
		x := voucherMargin + width*float64(index)
		pdf.Rect(x, y, width, voucherSignHeight, "D")
		pdf.SetFont("Helvetica", "", 8.5)
		pdf.SetXY(x, y+2)
		pdf.CellFormat(width, 5, signer[0], "", 0, "C", false, 0, "")
		pdf.Line(x+7, y+16.5, x+width-7, y+16.5)
		pdf.SetXY(x, y+17.3)
		pdf.CellFormat(width, 5, tr(signer[1]), "", 0, "C", false, 0, "")
	}
}

func imageType(data []byte) string {
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return "PNG"
	}
	return "JPG"
}

func fitBox(width, height, maxWidth, maxHeight float64) (float64, float64) {
	if width <= 0 || height <= 0 {
		return maxWidth, maxHeight
	}
	scale := min(maxWidth/width, maxHeight/height)
	return width * scale, height * scale
}

func proofHeader(pdf *fpdf.Fpdf, proof bundleProof, page, pages int, tr func(string) string) {
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.SetXY(proofMargin, 12)
	pdf.CellFormat(150, 5, tr("Requester: "+proof.Requester), "", 2, "L", false, 0, "")
	pdf.CellFormat(150, 5.5, tr(proof.Title), "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetXY(proofMargin, 17)
	pdf.CellFormat(180, 5.5, fmt.Sprintf("Halaman %d/%d", page, pages), "", 0, "R", false, 0, "")
	pdf.SetXY(proofMargin, 22.5)
	pdf.CellFormat(180, 4.5, fitText(pdf, tr("File: "+proof.Filename), 178), "", 0, "L", false, 0, "")
	pdf.SetDrawColor(190, 190, 190)
	pdf.Line(proofMargin, 28, 195, 28)
	pdf.SetDrawColor(0, 0, 0)
}

// renderImageProof gives an image its own A4 page under the row header. An
// image the PDF library refuses (an interlaced PNG, say) goes in as a JPEG.
func renderImageProof(proof bundleProof) ([]byte, error) {
	page, err := drawImageProof(proof)
	if err == nil {
		return page, nil
	}
	decoded, _, decodeErr := image.Decode(bytes.NewReader(proof.Data))
	if decodeErr != nil {
		return nil, &ProofError{Filename: proof.Filename}
	}
	var converted bytes.Buffer
	if err := jpeg.Encode(&converted, decoded, &jpeg.Options{Quality: 88}); err != nil {
		return nil, &ProofError{Filename: proof.Filename}
	}
	proof.Data = converted.Bytes()
	page, err = drawImageProof(proof)
	if err != nil {
		return nil, &ProofError{Filename: proof.Filename}
	}
	return page, nil
}

func drawImageProof(proof bundleProof) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.AddPage()
	proofHeader(pdf, proof, 1, 1, tr)
	options := fpdf.ImageOptions{ImageType: imageType(proof.Data), ReadDpi: false}
	info := pdf.RegisterImageOptionsReader("proof", options, bytes.NewReader(proof.Data))
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("read proof %s: %w", proof.Filename, err)
	}
	width, height := fitBox(info.Width(), info.Height(), 210-2*proofMargin, proofContentBottom-proofHeaderBottom)
	pdf.ImageOptions("proof", (210-width)/2, proofHeaderBottom, width, height, false, options, 0, "")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render proof %s: %w", proof.Filename, err)
	}
	return out.Bytes(), nil
}

// renderPDFProof fits every page of a PDF on A4 and stamps the row header
// over the room left at the top.
func renderPDFProof(proof bundleProof) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	nup, err := pdfapi.PDFGridConfig(1, 1, "formsize:A4, margin:45, border:off", conf)
	if err != nil {
		return nil, fmt.Errorf("prepare proof %s: %w", proof.Filename, err)
	}
	var fitted bytes.Buffer
	if err := pdfapi.NUp(bytes.NewReader(proof.Data), &fitted, nil, nil, nup, conf); err != nil {
		return nil, &ProofError{Filename: proof.Filename}
	}
	pages, err := pageCount(fitted.Bytes())
	if err != nil {
		return nil, err
	}
	stamps := map[int]*model.Watermark{}
	for page := 1; page <= pages; page++ {
		text := fmt.Sprintf("Requester: %s\n%s   (Halaman %d/%d)\nFile: %s", proof.Requester, proof.Title, page, pages, proof.Filename)
		stamp, err := pdfapi.TextWatermark(text, "font:Helvetica, points:8, position:tl, offset:20 -8, scale:1 abs, rotation:0, fillcolor:#000000, opacity:1", true, false, types.POINTS)
		if err != nil {
			return nil, fmt.Errorf("stamp proof %s: %w", proof.Filename, err)
		}
		stamps[page] = stamp
	}
	var stamped bytes.Buffer
	if err := pdfapi.AddWatermarksMap(bytes.NewReader(fitted.Bytes()), &stamped, stamps, conf); err != nil {
		return nil, fmt.Errorf("stamp proof %s: %w", proof.Filename, err)
	}
	return stamped.Bytes(), nil
}

func renderBundle(doc voucherDocument, proofs []bundleProof) ([]byte, error) {
	voucher, err := renderVoucher(doc)
	if err != nil {
		return nil, err
	}
	parts := [][]byte{voucher}
	for _, proof := range proofs {
		var part []byte
		if proof.ContentType == "application/pdf" {
			part, err = renderPDFProof(proof)
		} else {
			part, err = renderImageProof(proof)
		}
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return mergePDFs(parts)
}

func mergePDFs(parts [][]byte) ([]byte, error) {
	if len(parts) == 1 {
		return parts[0], nil
	}
	readers := make([]io.ReadSeeker, len(parts))
	for index, part := range parts {
		readers[index] = bytes.NewReader(part)
	}
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	var out bytes.Buffer
	if err := pdfapi.MergeRaw(readers, &out, false, conf); err != nil {
		return nil, fmt.Errorf("merge voucher bundle: %w", err)
	}
	return out.Bytes(), nil
}

func pageCount(data []byte) (int, error) {
	count, err := pdfapi.PageCount(bytes.NewReader(data), nil)
	if err != nil {
		return 0, fmt.Errorf("count voucher pages: %w", err)
	}
	return count, nil
}
