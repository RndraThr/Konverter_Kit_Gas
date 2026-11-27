package bast

import (
	"testing"

	"github.com/go-pdf/fpdf"
)

// A machine serial such as "SP065ABF320005" (and the 16-digit farmer card)
// must stay on one line in the Rekap Harian table.
func TestDailyRecapColumnsKeepSerialAndCardOnOneLine(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	registerArial(pdf)
	pdf.SetFont(activityFont, "", activityTableBodyPt)
	cells := map[int]string{2: "6013010629816241", 5: "SP065ABF320005"}
	for column, value := range cells {
		if lines := wrapWords(pdf, value, dailyRecapColumnWidths[column]-2*formalCellPadXMM); len(lines) != 1 {
			t.Fatalf("%q wraps into %d lines in column %q", value, len(lines), dailyRecapHeaders[column])
		}
	}
	total := 0.0
	for _, width := range dailyRecapColumnWidths {
		total += width
	}
	if total != pageWidthMM-2*marginMM {
		t.Fatalf("columns add up to %v mm, want %v", total, pageWidthMM-2*marginMM)
	}
}
