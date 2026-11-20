package banner

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

func testLogo(t *testing.T) []byte {
	t.Helper()
	if path := os.Getenv("BANNER_LOGO"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 50))
	for x := range 40 {
		for y := range 50 {
			img.Set(x, y, color.RGBA{0x1f, 0x5f, 0xa8, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Every banner renders on its template, keeps its single page and size, and
// grows by little more than the stamped layer (the template is not recompressed).
// BANNER_OUT=<dir> also writes the files for a visual check.
func TestRenderEveryKind(t *testing.T) {
	logo := testLogo(t)
	for _, kind := range Kinds {
		t.Run(string(kind), func(t *testing.T) {
			out, err := Render(Input{
				Kind: kind, RegencyName: "KAB. BONE", Logo: logo,
				ServiceCenter: ServiceCenter{Name: "Agus Saidi Tekhnik", Address: "Dsn. Muara, Kel. Sungai Lueng, Kec. Langsa Timur, Kota Langsa", Phone: "0852-7709-7647"},
				PersonName:    "Dr. Ir. Hj. Andi Yuliani Paris, M.Sc", Place: "Bone", Date: time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC),
				Styles: map[string]TextStyle{StylePlakatName: {SizePercent: 130, Color: "#8B0000"}, StyleBackdropService: {SizePercent: 80}},
			})
			if err != nil {
				t.Fatal(err)
			}
			pages, err := pdfapi.PageCount(bytes.NewReader(out), nil)
			if err != nil || pages != 1 {
				t.Fatalf("pages=%d err=%v", pages, err)
			}
			// Rewriting the file tidies its cross-reference data only; the
			// images keep their bytes, so the size stays within 1%.
			template, _ := assets.ReadFile(layouts[kind].template)
			if float64(len(out)) < float64(len(template))*0.99 {
				t.Fatalf("output %d bytes is well below the template %d: it was recompressed", len(out), len(template))
			}
			if dir := os.Getenv("BANNER_OUT"); dir != "" {
				if err := os.WriteFile(filepath.Join(dir, Filename(kind, "KAB. BONE")), out, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRegencyNames(t *testing.T) {
	for in, want := range map[string]string{"KAB. TANAH DATAR": "Kabupaten Tanah Datar", "KOTA LANGSA": "Kota Langsa", "Kab. Bireuen": "Kabupaten Bireuen", "KABUPATEN BONE": "Kabupaten Bone"} {
		if got := RegencyDisplayName(in); got != want {
			t.Errorf("RegencyDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RegencyShortName("KAB. BONE"); got != "Bone" {
		t.Errorf("short name = %q", got)
	}
	if got := FormatDate(time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC)); got != "6 Desember 2026" {
		t.Errorf("date = %q", got)
	}
	if scale, color := resolveStyle(map[string]TextStyle{StylePlakatName: {SizePercent: 120, Color: "#8B0000"}}, StylePlakatName); scale != 1.2 || color != [3]int{0x8B, 0, 0} {
		t.Errorf("style = %v %v", scale, color)
	}
	if scale, color := resolveStyle(nil, StyleSeremoniName); scale != 1 || color != [3]int{0x23, 0x2C, 0x7A} {
		t.Errorf("default style = %v %v", scale, color)
	}
	if got := Filename(KindPlakat, "KAB. BONE"); got != "PLAKAT PETANI KABUPATEN BONE.pdf" {
		t.Errorf("filename = %q", got)
	}
}
