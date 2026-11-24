package finalreport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image/jpeg"
	"io"
	"regexp"
	"strings"
	"testing"

	"konkit/internal/finalreport/template"
)

func TestRenderPPTXSubstitutesEveryToken(t *testing.T) {
	preview := Preview{
		RegencyLabel: "KABUPATEN TEST", FiscalYear: "2026", AgricultureOfficeName: "DINAS TEST",
		ConsultantCompanyName: "PT TEST", MachineManufacturerName: "PT MESIN TEST",
		DeliveryHoseDate: "1 Januari 2026", DeliveryMachineGroupDate: "2 Januari 2026", DeliveryGasCylinderDate: "3 Januari 2026",
		DeliveryRangeStart: "1 Januari 2026", DeliveryRangeEnd: "3 Januari 2026",
		RakordaDate: "4 Januari 2026", SosialisasiDate: "5 Januari 2026",
		LeaderCount: "1", AdminCount: "2", TechnicianCount: "3", HelperCount: "4",
		HandoverLocation: "GUDANG TEST", DistributionStartDate: "1 Februari 2026", DistributionEndDate: "28 Februari 2026", DistributionTotal: "100",
	}

	output, err := RenderPPTX(template.PPTX, preview, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	var allText strings.Builder
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		allText.WriteString(buf.String())
	}
	combined := allText.String()
	for _, token := range template.AllTokens {
		if strings.Contains(combined, token) {
			t.Errorf("token %s was not substituted", token)
		}
	}
	if !strings.Contains(combined, "KABUPATEN TEST") || !strings.Contains(combined, "GUDANG TEST") {
		t.Fatalf("expected substituted values present in output")
	}
}

func TestRenderPPTXEscapesXMLSpecialChars(t *testing.T) {
	preview := Preview{ConsultantCompanyName: "PT A & B <Tbk>", HandoverLocation: "Gudang \"X\" <1>"}
	output, err := RenderPPTX(template.PPTX, preview, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	var combined strings.Builder
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()
		dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s is not well-formed XML: %v", file.Name, err)
			}
		}
		combined.Write(buf.Bytes())
	}
	text := combined.String()
	if !strings.Contains(text, "PT A &amp; B &lt;Tbk&gt;") {
		t.Errorf("expected escaped company name in output")
	}
	if strings.Contains(text, "PT A & B <Tbk>") {
		t.Errorf("raw unescaped value found in output")
	}
}

func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	entries := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		entries[file.Name] = content
	}
	return entries
}

// allContentSlots lists every content-photo media entry on slides 5-8,
// excluding the shared logos image1.png (KSM) and image2.png (certificates).
var allContentSlots = []string{
	"ppt/media/image3.jpg", "ppt/media/image4.jpg", "ppt/media/image5.jpg", // slide5
	"ppt/media/image6.jpg", "ppt/media/image7.jpg", // slide6
	"ppt/media/image8.jpg", "ppt/media/image9.jpg", "ppt/media/image10.jpg", "ppt/media/image11.jpg", // slide7
	"ppt/media/image12.jpg", "ppt/media/image13.jpg", "ppt/media/image14.jpg", // slide8
}

func TestRenderPPTXNeverLeaksReferencePhotosWithZeroPicks(t *testing.T) {
	output, err := RenderPPTX(template.PPTX, Preview{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	original := zipEntries(t, template.PPTX)
	rendered := zipEntries(t, output)
	placeholder, err := placeholderJPEG()
	if err != nil {
		t.Fatalf("placeholder: %v", err)
	}
	for _, name := range allContentSlots {
		got, ok := rendered[name]
		if !ok {
			t.Fatalf("%s missing from output", name)
		}
		if bytes.Equal(got, original[name]) {
			t.Errorf("%s still carries the reference deck's original photo", name)
		}
		if !bytes.Equal(got, placeholder) {
			t.Errorf("%s is not the neutral placeholder", name)
		}
	}
	for _, shared := range []string{"ppt/media/image1.png", "ppt/media/image2.png"} {
		if !bytes.Equal(rendered[shared], original[shared]) {
			t.Errorf("shared logo %s must be left untouched", shared)
		}
	}
}

func TestRenderPPTXFillsPickedSlotsAndPlaceholdersTheRest(t *testing.T) {
	pickA, pickB := []byte("pick-a-bytes"), []byte("pick-b-bytes")
	output, err := RenderPPTX(template.PPTX, Preview{}, map[string][][]byte{
		SlideKeyPengirimanPenerimaan: {pickA},
		SlideKeySosialisasi:          {pickA, pickB},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rendered := zipEntries(t, output)
	placeholder, err := placeholderJPEG()
	if err != nil {
		t.Fatalf("placeholder: %v", err)
	}
	want := map[string][]byte{
		"ppt/media/image3.jpg": pickA, "ppt/media/image4.jpg": placeholder, "ppt/media/image5.jpg": placeholder,
		"ppt/media/image6.jpg": placeholder, "ppt/media/image7.jpg": placeholder,
		"ppt/media/image8.jpg": pickA, "ppt/media/image9.jpg": pickB, "ppt/media/image10.jpg": placeholder, "ppt/media/image11.jpg": placeholder,
		"ppt/media/image12.jpg": placeholder, "ppt/media/image13.jpg": placeholder, "ppt/media/image14.jpg": placeholder,
	}
	for name, expected := range want {
		if !bytes.Equal(rendered[name], expected) {
			t.Errorf("%s: got %d bytes, want %d bytes (%q...)", name, len(rendered[name]), len(expected), string(expected[:min(len(expected), 12)]))
		}
	}
}

func TestPlaceholderJPEGDecodesAsJPEG(t *testing.T) {
	data, err := placeholderJPEG()
	if err != nil {
		t.Fatalf("placeholder: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("placeholder is not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 1600 || b.Dy() != 1200 {
		t.Fatalf("placeholder size %dx%d, want 1600x1200", b.Dx(), b.Dy())
	}
}

func TestMaxPicksMatchesSlotCounts(t *testing.T) {
	cases := map[string]int{SlideKeyPengirimanPenerimaan: 3, SlideKeyRapatKoordinasi: 2, SlideKeySosialisasi: 4, "unknown": 0}
	for key, want := range cases {
		if got := MaxPicks(key); got != want {
			t.Errorf("MaxPicks(%q) = %d, want %d", key, got, want)
		}
	}
}

// TestSlotMapMatchesTemplate guards a rebuilt template (design/build.py):
// every mapped media file must be a picture of the expected slide, in the
// order the slide references it, and every photo of slides 5-8 is mapped.
func TestSlotMapMatchesTemplate(t *testing.T) {
	entries := zipEntries(t, template.PPTX)
	slides := map[string][]string{
		"slide5": slideImageOrder[SlideKeyPengirimanPenerimaan],
		"slide6": slideImageOrder[SlideKeyRapatKoordinasi],
		"slide7": slideImageOrder[SlideKeySosialisasi],
		"slide8": placeholderOnlySlots["dokumentasi_pendistribusian"],
	}
	for slide, want := range slides {
		rels := string(entries["ppt/slides/_rels/"+slide+".xml.rels"])
		xmlText := string(entries["ppt/slides/"+slide+".xml"])
		var got []string
		for _, ref := range regexp.MustCompile(`r:embed="(rId\d+)"`).FindAllStringSubmatch(xmlText, -1) {
			target := regexp.MustCompile(`Id="` + ref[1] + `"[^>]*Target="\.\./media/([^"]+\.jpg)"`).FindStringSubmatch(rels)
			if target != nil {
				got = append(got, "ppt/media/"+target[1])
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s photos = %v, mapped %v", slide, got, want)
		}
	}
}
