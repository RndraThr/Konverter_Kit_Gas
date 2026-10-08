package textnorm

import (
	"strings"
	"unicode"
)

// DisplayTitle mengubah teks bisnis yang tersimpan huruf kapital (lihat
// BusinessUpper) menjadi huruf judul untuk dicetak di dokumen, mengikuti
// kaidah KBBI/PUEBI: setiap kata berhuruf awal kapital, kata tugas di tengah
// huruf kecil, singkatan (PT, RT, LPG, …) tetap kapital, dan gelar/singkatan
// bertitik ditulis baku (Ir., M.Si., Kab., Tbk.). Data tersimpan tidak diubah.
func DisplayTitle(value string) string {
	words := strings.Fields(value)
	for i, word := range words {
		words[i] = displayToken(word, i == 0)
	}
	return strings.Join(words, " ")
}

// displayToken memproses satu kata beserta tanda baca yang menempel, misalnya
// "(PERSERO)," atau "02/RW".
func displayToken(token string, first bool) string {
	start, end := 0, len(token)
	for start < end && strings.ContainsRune("([\"'", rune(token[start])) {
		start++
	}
	for end > start && strings.ContainsRune(",;:)]\"'", rune(token[end-1])) {
		end--
	}
	core := token[start:end]
	if core == "" {
		return token
	}
	return token[:start] + displayCore(core, first) + token[end:]
}

// displayCore menangani kata yang dipisah "/" atau "-" (RT 02/RW 01,
// BANGKA-BELITUNG) per bagian.
func displayCore(core string, first bool) string {
	if formatted, ok := displayDotted(core); ok {
		return formatted
	}
	var out strings.Builder
	part := strings.Builder{}
	flush := func() {
		if part.Len() > 0 {
			out.WriteString(displayWord(part.String(), first && out.Len() == 0))
			part.Reset()
		}
	}
	for _, r := range core {
		if r == '/' || r == '-' {
			flush()
			out.WriteRune(r)
			continue
		}
		part.WriteRune(r)
	}
	flush()
	return out.String()
}

// displayDotted menulis baku gelar dan singkatan bertitik, termasuk gelar
// tanpa titik akhir (M.SI -> M.Si.). Nama inisial (H., A.) ditulis kapital.
func displayDotted(core string) (string, bool) {
	if !strings.Contains(core, ".") {
		if formatted, ok := dottedWithoutDot[strings.ToLower(core)]; ok {
			return formatted, true
		}
		return "", false
	}
	key := strings.TrimSuffix(strings.ToLower(core), ".")
	if formatted, ok := dottedForms[key]; ok {
		return formatted, true
	}
	// Inisial satu huruf: "A." / "H.".
	if runes := []rune(key); len(runes) == 1 && unicode.IsLetter(runes[0]) {
		return strings.ToUpper(key) + ".", true
	}
	return "", false
}

func displayWord(word string, first bool) string {
	lower := strings.ToLower(word)
	if strings.IndexFunc(word, unicode.IsDigit) >= 0 {
		return word
	}
	if acronym, ok := keepUpper[lower]; ok {
		return acronym
	}
	if !first && functionWords[lower] {
		return lower
	}
	runes := []rune(lower)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// dottedForms memetakan gelar dan singkatan bertitik (tanpa titik akhir,
// huruf kecil) ke bentuk bakunya.
var dottedForms = map[string]string{
	// Gelar depan dan sebutan.
	"ir": "Ir.", "dr": "Dr.", "drs": "Drs.", "dra": "Dra.", "prof": "Prof.", "hj": "Hj.", "drh": "drh.",
	// Gelar sarjana.
	"s.t": "S.T.", "s.p": "S.P.", "s.h": "S.H.", "s.e": "S.E.", "s.pd": "S.Pd.", "s.kom": "S.Kom.", "s.sos": "S.Sos.",
	"s.ag": "S.Ag.", "s.ip": "S.I.P.", "s.i.p": "S.I.P.", "s.ked": "S.Ked.", "s.pt": "S.Pt.", "s.hut": "S.Hut.", "s.tp": "S.TP.",
	"s.si": "S.Si.", "s.ak": "S.Ak.", "s.tr": "S.Tr.", "s.st": "S.ST.", "s.pi": "S.Pi.", "s.kel": "S.Kel.", "s.farm": "S.Farm.",
	"a.md": "A.Md.", "amd": "A.Md.",
	// Gelar magister dan doktor.
	"m.si": "M.Si.", "m.m": "M.M.", "m.t": "M.T.", "m.p": "M.P.", "m.pd": "M.Pd.", "m.kes": "M.Kes.", "m.h": "M.H.",
	"m.e": "M.E.", "m.sc": "M.Sc.", "m.eng": "M.Eng.", "m.ak": "M.Ak.", "m.ap": "M.AP.", "m.kom": "M.Kom.", "ph.d": "Ph.D.",
	// Singkatan wilayah, alamat, dan badan usaha.
	"kab": "Kab.", "kec": "Kec.", "kel": "Kel.", "ds": "Ds.", "dsn": "Dsn.", "jl": "Jl.", "jln": "Jln.", "gg": "Gg.",
	"no": "No.", "prov": "Prov.", "tbk": "Tbk.", "kp": "Kp.",
}

// dottedWithoutDot menangani singkatan baku yang sering diketik tanpa titik.
var dottedWithoutDot = map[string]string{"tbk": "Tbk.", "msi": "M.Si.", "amd": "A.Md."}

// keepUpper berisi singkatan yang ditulis kapital semua.
var keepUpper = map[string]string{}

func init() {
	for _, acronym := range []string{
		"PT", "CV", "UD", "KUD", "PD", "BUMN", "BUMD", "BUMDES", "TBK.",
		"RT", "RW", "DKI", "DIY", "NTB", "NTT", "NAD", "RI", "NKRI",
		"LPG", "BBM", "BBG", "ESDM", "SPBU", "SPBE", "KKT", "KSM", "UPTD", "UPT", "BPP", "PPL", "DPR", "DPRD", "TNI", "POLRI", "PNS", "ASN",
		"II", "III", "IV", "VI", "VII", "VIII", "IX", "XI", "XII", "XIII", "XIV", "XV",
	} {
		keepUpper[strings.ToLower(acronym)] = acronym
	}
	delete(keepUpper, "tbk.")
}

// functionWords adalah kata tugas yang ditulis huruf kecil bila tidak di awal.
var functionWords = map[string]bool{
	"dan": true, "di": true, "ke": true, "dari": true, "yang": true, "untuk": true, "atau": true, "pada": true,
	"dengan": true, "bagi": true, "oleh": true, "dalam": true, "serta": true, "atas": true, "tentang": true, "kepada": true,
}
