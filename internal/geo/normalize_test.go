package geo

import "testing"

func TestNormalizeRegencyName(t *testing.T) {
	cases := map[string]string{
		"KAB. BANGKA BARAT": "BANGKABARAT",
		"Kabupaten Bandung": "BANDUNG",
		"KOTA BANDUNG":      "KOTABANDUNG",
		"Bangka Barat":      "BANGKABARAT",
	}
	for in, want := range cases {
		if got := NormalizeRegencyName(in); got != want {
			t.Errorf("NormalizeRegencyName(%q) = %q, want %q", in, got, want)
		}
	}
}
