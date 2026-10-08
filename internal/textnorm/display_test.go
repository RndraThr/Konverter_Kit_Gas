package textnorm

import "testing"

func TestDisplayTitleFollowsIndonesianCapitalisation(t *testing.T) {
	cases := map[string]string{
		"WAJO":                           "Wajo",
		"KABUPATEN WONOGIRI":             "Kabupaten Wonogiri",
		"KAB. WONOGIRI":                  "Kab. Wonogiri",
		"SULAWESI SELATAN":               "Sulawesi Selatan",
		"DKI JAKARTA":                    "DKI Jakarta",
		"KEPULAUAN BANGKA-BELITUNG":      "Kepulauan Bangka-Belitung",
		"BALAI DESA DAN KANTOR CAMAT":    "Balai Desa dan Kantor Camat",
		"PT KIAN SANTANG MULIATAMA TBK.": "PT Kian Santang Muliatama Tbk.",
		"PT KIAN SANTANG MULIATAMA TBK":  "PT Kian Santang Muliatama Tbk.",
		"CV MAJU JAYA":                   "CV Maju Jaya",
		"PT PERTAMINA (PERSERO)":         "PT Pertamina (Persero)",
		"IR. HARYANTO, M.SI":             "Ir. Haryanto, M.Si.",
		"DR. IR. H. AHMAD SUBAGYO, M.P.": "Dr. Ir. H. Ahmad Subagyo, M.P.",
		"DRS. SUTARNO, S.H., M.M.":       "Drs. Sutarno, S.H., M.M.",
		"HJ. SITI AMINAH, S.PD.":         "Hj. Siti Aminah, S.Pd.",
		"MULYONO":                        "Mulyono",
		"DUSUN TALUNOMBO RT 02/RW 01, Desa TALUNOMBO, Kec. BATURETNO": "Dusun Talunombo RT 02/RW 01, Desa Talunombo, Kec. Baturetno",
		"JL. MERDEKA NO. 12": "Jl. Merdeka No. 12",
		"KECAMATAN TEMPE II": "Kecamatan Tempe II",
		"DI BALAI DESA":      "Di Balai Desa",
		"  ":                 "",
	}
	for input, want := range cases {
		if got := DisplayTitle(input); got != want {
			t.Errorf("DisplayTitle(%q) = %q, want %q", input, got, want)
		}
	}
}
