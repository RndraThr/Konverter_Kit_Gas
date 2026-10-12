package bast

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/go-pdf/fpdf"
)

func testSignaturePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 600, 240))
	for x := 10; x < 590; x++ {
		img.Set(x, 120+x%20, color.NRGBA{R: 20, G: 30, B: 90, A: 255})
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A slot with a real signature gets the image; others keep the drawn mark;
// a broken image never breaks the document.
func TestDrawSignatureUsesRecipientImage(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	images := map[string][]byte{
		signatureImageKey("signed-slot"): testSignaturePNG(t),
		signatureImageKey("broken-slot"): []byte("not a png"),
		"logo-asset":                     []byte("ignored"),
	}
	registerSignatures(pdf, images)
	if pdf.GetImageInfo(signatureImageKey("signed-slot")) == nil {
		t.Fatal("signature image not registered")
	}
	if pdf.GetImageInfo(signatureImageKey("broken-slot")) != nil {
		t.Fatal("broken image registered")
	}
	drawSignature(pdf, 10, 10, 57, 15.5, "signed-slot")
	drawSignature(pdf, 10, 30, 57, 15.5, "broken-slot")
	drawSignature(pdf, 10, 50, 57, 15.5, "")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil || out.Len() == 0 {
		t.Fatalf("render err=%v size=%d", err, out.Len())
	}
}
