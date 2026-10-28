package bast

import (
	"testing"

	"github.com/go-pdf/fpdf"
)

func TestRecipientLocationsJoinsDistinctHandoverLocations(t *testing.T) {
	if got := recipientLocations([]string{"Balai Desa", " balai desa ", "Lapangan", ""}, "Gudang"); got != "Balai Desa, Lapangan" {
		t.Fatalf("locations=%q", got)
	}
	if got := recipientLocations([]string{"", " "}, "Gudang"); got != "Gudang" {
		t.Fatalf("fallback=%q", got)
	}
}

func TestWrapWordsBreaksAWordWiderThanTheCell(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 10)
	lines := wrapWords(pdf, "521435KTXIPSPDPKP075", 20)
	if len(lines) < 2 {
		t.Fatalf("long card number should wrap, got %q", lines)
	}
	for _, line := range lines {
		if pdf.GetStringWidth(line) > 20 {
			t.Fatalf("line %q wider than the cell", line)
		}
	}
	if got := wrapWords(pdf, "Shark SPWP", 60); len(got) != 1 {
		t.Fatalf("short text should stay on one line, got %q", got)
	}
}
