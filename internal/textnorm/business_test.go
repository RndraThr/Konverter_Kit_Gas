package textnorm

import "testing"

func TestBusinessUpperTrimsAndUppercasesUnicode(t *testing.T) {
	if got := BusinessUpper("  Siti Núraeni / Blok 2  "); got != "SITI NÚRAENI / BLOK 2" {
		t.Fatalf("BusinessUpper() = %q", got)
	}
}

func TestBusinessUpperKeepsPunctuationAndNumbers(t *testing.T) {
	if got := BusinessUpper("spwp 80-30 / 3 inch"); got != "SPWP 80-30 / 3 INCH" {
		t.Fatalf("BusinessUpper() = %q", got)
	}
}

func TestBusinessUpperNormalizesWhitespaceOnlyToEmpty(t *testing.T) {
	if got := BusinessUpper(" \t\r\n "); got != "" {
		t.Fatalf("BusinessUpper() = %q", got)
	}
}
