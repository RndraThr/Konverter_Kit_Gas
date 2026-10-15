package finalreport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
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
