package textnorm

import "testing"

func TestPlaceKeyIgnoresCaseSpacesAndPunctuation(t *testing.T) {
	for _, value := range []string{"Balai Ds. Tempe", " balai  ds tempe ", "BALAI-DS/TEMPE"} {
		if got := PlaceKey(value); got != "BALAI DS TEMPE" {
			t.Fatalf("PlaceKey(%q)=%q", value, got)
		}
	}
	if PlaceName("  Balai   Desa  ") != "Balai Desa" {
		t.Fatal("PlaceName should collapse spaces")
	}
}
