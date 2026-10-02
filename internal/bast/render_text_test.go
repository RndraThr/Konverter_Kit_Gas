package bast

import "testing"

func TestRenderBusinessTextTrimsAndUppercasesUnicode(t *testing.T) {
	if got := renderBusinessText("  Siti Núraeni  "); got != "SITI NÚRAENI" {
		t.Fatalf("renderBusinessText() = %q", got)
	}
}
