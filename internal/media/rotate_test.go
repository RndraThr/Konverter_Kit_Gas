package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

// marked is a 4x2 image, red in its top left pixel.
func marked() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

func TestRotatePNGClockwise(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, marked()); err != nil {
		t.Fatal(err)
	}
	out, err := RotateFile(source.Bytes(), "image/png", 90, 0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Fatalf("size %v, want 2x4", b)
	}
	// Top left goes to top right after a clockwise quarter turn.
	if r, _, _, _ := img.At(1, 0).RGBA(); r>>8 != 255 {
		t.Fatalf("red pixel not at top right")
	}
}

func TestRotateJPEGAppliesExifOrientation(t *testing.T) {
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil); err != nil {
		t.Fatal(err)
	}
	// EXIF APP1 with orientation 6 (viewers show it turned 90° clockwise).
	exif := []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00\x00\x00\x00\x00")
	segment := append([]byte{0xFF, 0xE1, byte((len(exif) + 2) >> 8), byte(len(exif) + 2)}, exif...)
	withExif := append(append([]byte{0xFF, 0xD8}, segment...), plain.Bytes()[2:]...)
	if got := jpegOrientation(withExif); got != 6 {
		t.Fatalf("orientation %d, want 6", got)
	}
	// Shown as 20x40; turned once more clockwise it is 40x20.
	out, err := RotateFile(withExif, "image/jpeg", 90, 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 40 || cfg.Height != 20 {
		t.Fatalf("size %dx%d, want 40x20", cfg.Width, cfg.Height)
	}
}

func TestRotatePDFTurnsPages(t *testing.T) {
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.AddPage()
	var source bytes.Buffer
	if err := doc.Output(&source); err != nil {
		t.Fatal(err)
	}
	out, err := RotateFile(source.Bytes(), "application/pdf", 90, 0)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfapi.PageCount(bytes.NewReader(out), nil)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Fatalf("pages %d, want 2", pages)
	}
	ctx, err := pdfapi.ReadAndValidate(bytes.NewReader(out), nil)
	if err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= 2; page++ {
		_, _, inherited, err := ctx.PageDict(page, false)
		if err != nil {
			t.Fatal(err)
		}
		if inherited.Rotate != 90 {
			t.Fatalf("page %d rotation %d, want 90", page, inherited.Rotate)
		}
	}
}

func TestRotateRejectsVideoAndOddAngles(t *testing.T) {
	if _, err := RotateFile([]byte("x"), "video/mp4", 90, 0); err != ErrRotateUnsupported {
		t.Fatalf("err %v, want unsupported", err)
	}
	if _, err := NormalizeDegrees(45); err != ErrRotateDegrees {
		t.Fatalf("err %v, want degrees", err)
	}
	if d, _ := NormalizeDegrees(-90); d != 270 {
		t.Fatalf("-90 normalized to %d", d)
	}
}

func TestLocalStorageReplaceKeepsKey(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key, _, _, err := storage.Put(ctx, "file-1", nil, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	size, checksum, err := Replace(ctx, storage, key, strings.NewReader("new content"))
	if err != nil || size != 11 || len(checksum) != 64 {
		t.Fatalf("replace size=%d checksum=%q err=%v", size, checksum, err)
	}
	reader, err := storage.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, _ := io.ReadAll(reader)
	if string(got) != "new content" {
		t.Fatalf("content %q", got)
	}
	if _, _, err := Replace(ctx, storage, "missing", strings.NewReader("x")); err == nil {
		t.Fatal("replacing a missing file must fail")
	}
}
