package finalreport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"konkit/internal/finalreport/template"
)

// slideImageOrder maps each photo-bearing slide to the ppt/media file names
// that RenderPPTX overwrites with the picked photos' bytes, in the order the
// pictures are referenced (and spatially laid out left-to-right) on each
// slide. Verified directly against the committed
// internal/finalreport/template/final_report_template.pptx by inspecting
// ppt/slides/_rels/slide{5,6,7}.xml.rels (slide5 = Pengiriman & Penerimaan,
// slide6 = Rapat Koordinasi, slide7 = Sosialisasi) and cross-checking the
// r:embed reference order and <a:off x=.../> positions in each
// ppt/slides/slideN.xml. image9.png and image5.png appear in all three rels
// files at identical positions (top-left header logo and bottom-right
// footer logo) and are excluded here as shared graphics, not content photos.
//
// Note: slide5's content images appear in the rels/XML in the order
// image12.jpg, image2.jpg, image1.jpg (left-to-right by x offset), which is
// the reverse of a naive numeric sort of the file names - order was
// confirmed by reading the actual <a:off> positions in slide5.xml rather
// than assumed.
var slideImageOrder = map[string][]string{
	SlideKeyPengirimanPenerimaan: {"ppt/media/image12.jpg", "ppt/media/image2.jpg", "ppt/media/image1.jpg"},
	SlideKeyRapatKoordinasi:      {"ppt/media/image8.jpg", "ppt/media/image15.jpg"},
	SlideKeySosialisasi:          {"ppt/media/image3.jpg", "ppt/media/image6.jpg", "ppt/media/image16.jpg", "ppt/media/image14.jpg"},
}

// RenderPPTX opens the master PPTX template as a ZIP archive, replaces every
// placeholder token in each slide's XML with the matching value from
// preview, optionally overwrites specific ppt/media/* image files with
// picked photo bytes (keyed by slide_key, in placeholder order), and
// returns the re-zipped archive as bytes.
// xmlEscape escapes a substituted value so it is safe inside an <a:t> element.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func RenderPPTX(templatePath string, preview Preview, photoBytes map[string][][]byte) ([]byte, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("read pptx template: %w", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pptx template as zip: %w", err)
	}

	replacements := map[string]string{
		template.TokenRegencyLabel: preview.RegencyLabel, template.TokenFiscalYear: preview.FiscalYear,
		template.TokenAgricultureOfficeName: preview.AgricultureOfficeName, template.TokenConsultantCompanyName: preview.ConsultantCompanyName,
		template.TokenMachineManufacturerName: preview.MachineManufacturerName,
		template.TokenDeliveryHoseDate:        preview.DeliveryHoseDate, template.TokenDeliveryMachineGroupDate: preview.DeliveryMachineGroupDate,
		template.TokenDeliveryGasCylinderDate: preview.DeliveryGasCylinderDate,
		template.TokenDeliveryRangeStart:      preview.DeliveryRangeStart, template.TokenDeliveryRangeEnd: preview.DeliveryRangeEnd,
		template.TokenRakordaDate: preview.RakordaDate, template.TokenSosialisasiDate: preview.SosialisasiDate,
		template.TokenLeaderCount: preview.LeaderCount, template.TokenAdminCount: preview.AdminCount,
		template.TokenTechnicianCount: preview.TechnicianCount, template.TokenHelperCount: preview.HelperCount,
		template.TokenHandoverLocation:      preview.HandoverLocation,
		template.TokenDistributionStartDate: preview.DistributionStartDate, template.TokenDistributionEndDate: preview.DistributionEndDate,
		template.TokenDistributionTotal: preview.DistributionTotal,
	}

	for token, value := range replacements {
		replacements[token] = xmlEscape(value)
	}

	imageOverrides := map[string][]byte{}
	for slideKey, files := range slideImageOrder {
		pics := photoBytes[slideKey]
		for i, name := range files {
			if i < len(pics) {
				imageOverrides[name] = pics[i]
			}
		}
	}

	out := new(bytes.Buffer)
	writer := zip.NewWriter(out)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, err)
		}

		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			text := string(content)
			for token, value := range replacements {
				text = strings.ReplaceAll(text, token, value)
			}
			content = []byte(text)
		}
		if override, ok := imageOverrides[file.Name]; ok {
			content = override
		}

		header, err := zip.FileInfoHeader(file.FileInfo())
		if err != nil {
			return nil, err
		}
		header.Name = file.Name
		header.Method = zip.Deflate
		w, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close pptx zip: %w", err)
	}
	return out.Bytes(), nil
}
