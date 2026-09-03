package reports

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

var reportColumnHeaders = []string{
	"No. Pembagian", "Nama", "NIK", "No. Kartu Petani/KUSUKA", "Desa", "Kecamatan",
	"Status Alokasi", "Status Distribusi", "Dokumentasi", "Tanggal Selesai",
}

func reportRowValues(row Row) []string {
	return []string{
		fmt.Sprint(row.DistributionNumber), row.FullName, row.NIK, row.SectorIdentifier,
		row.Village, row.District, row.AllocationStatus, row.DistributionStatus,
		documentationLabel(row.DocumentationComplete), formatCompletedAt(row.CompletedAt),
	}
}

func documentationLabel(complete bool) string {
	if complete {
		return "Lengkap"
	}
	return "Belum lengkap"
}

func formatCompletedAt(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04")
}

func buildExcel(rows []Row) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	const sheet = "Laporan"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("name report sheet: %w", err)
	}
	for column, header := range reportColumnHeaders {
		cell, _ := excelize.CoordinatesToCellName(column+1, 1)
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, fmt.Errorf("write report header: %w", err)
		}
	}
	for index, row := range rows {
		for column, value := range reportRowValues(row) {
			cell, _ := excelize.CoordinatesToCellName(column+1, index+2)
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, fmt.Errorf("write report row: %w", err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode report excel: %w", err)
	}
	return buffer.Bytes(), nil
}

var pdfColumnWidths = []float64{18, 45, 32, 32, 25, 25, 25, 28, 22, 30}

func buildPDF(summary Summary, rows []Row) ([]byte, error) {
	doc := fpdf.New("L", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Helvetica", "B", 14)
	doc.CellFormat(0, 10, "Laporan Distribusi", "", 1, "L", false, 0, "")

	doc.SetFont("Helvetica", "", 11)
	doc.CellFormat(0, 7, fmt.Sprintf("Total alokasi: %d", summary.TotalAllocations), "", 1, "L", false, 0, "")
	for _, count := range summary.AllocationStatusCounts {
		doc.CellFormat(0, 6, fmt.Sprintf("Status alokasi %s: %d", count.Status, count.Count), "", 1, "L", false, 0, "")
	}
	for _, count := range summary.DistributionStatusCounts {
		doc.CellFormat(0, 6, fmt.Sprintf("Status distribusi %s: %d", count.Status, count.Count), "", 1, "L", false, 0, "")
	}
	doc.CellFormat(0, 6, fmt.Sprintf("Dokumentasi belum lengkap: %d", summary.DocumentationIncomplete), "", 1, "L", false, 0, "")
	doc.Ln(4)

	doc.SetFont("Helvetica", "B", 9)
	for index, header := range reportColumnHeaders {
		doc.CellFormat(pdfColumnWidths[index], 7, header, "1", 0, "L", false, 0, "")
	}
	doc.Ln(-1)
	doc.SetFont("Helvetica", "", 9)
	for _, row := range rows {
		for index, value := range reportRowValues(row) {
			doc.CellFormat(pdfColumnWidths[index], 6, value, "1", 0, "L", false, 0, "")
		}
		doc.Ln(-1)
	}

	var buffer bytes.Buffer
	if err := doc.Output(&buffer); err != nil {
		return nil, fmt.Errorf("encode report pdf: %w", err)
	}
	return buffer.Bytes(), nil
}
