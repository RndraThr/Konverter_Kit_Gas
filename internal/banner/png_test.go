package banner

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"testing"
)

func TestRenderPNGRasterizesTheBanner(t *testing.T) {
	pdf, err := Render(Input{Kind: KindPlakat, RegencyName: "KAB. BONE", PersonName: "Ir. Lamhot"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := RenderPNG(context.Background(), KindPlakat, pdf)
	if errors.Is(err, ErrImageUnavailable) {
		t.Skip("no pdftoppm or Ghostscript installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	// 1700.79 x 1133.86 pt at 300 dpi.
	if cfg.Width < 7000 || cfg.Width > 7100 || cfg.Height < 4700 || cfg.Height > 4730 {
		t.Fatalf("size=%dx%d", cfg.Width, cfg.Height)
	}
	if ImageFilename(KindPlakat, "KAB. BONE") != "PLAKAT PETANI KABUPATEN BONE.png" {
		t.Fatalf("filename=%q", ImageFilename(KindPlakat, "KAB. BONE"))
	}
}
