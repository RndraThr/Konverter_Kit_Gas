package bast

import (
	_ "embed"

	"github.com/go-pdf/fpdf"
)

// Arial disematkan ke binary agar PDF BA kegiatan memakai Arial asli di semua
// lingkungan (lokal, Docker) tanpa memasang font di server.
var (
	//go:embed fonts/arial.ttf
	arialRegular []byte
	//go:embed fonts/arialbd.ttf
	arialBold []byte
	//go:embed fonts/ariali.ttf
	arialItalic []byte
	//go:embed fonts/arialbi.ttf
	arialBoldItalic []byte
)

// arialFamily sengaja bukan "Arial": fpdf memetakan nama itu ke font inti
// Helvetica sebelum mencari font yang didaftarkan. "ArialMT" adalah nama
// PostScript Arial sehingga tetap terbaca sebagai Arial di properti PDF.
const arialFamily = "ArialMT"

func registerArial(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(arialFamily, "", arialRegular)
	pdf.AddUTF8FontFromBytes(arialFamily, "B", arialBold)
	pdf.AddUTF8FontFromBytes(arialFamily, "I", arialItalic)
	pdf.AddUTF8FontFromBytes(arialFamily, "BI", arialBoldItalic)
}
