package finalreport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"strings"
	"sync"

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

// placeholderOnlySlots are content-photo slots that have no photo picker in
// v1 and are therefore always overwritten with the neutral placeholder.
// slide8 (Dokumentasi Pendistribusian) references image11.jpg, image13.jpg
// and image7.jpg left-to-right (verified against slide8.xml.rels and the
// <a:off x=.../> positions in slide8.xml), plus the shared logos.
var placeholderOnlySlots = map[string][]string{
	"dokumentasi_pendistribusian": {"ppt/media/image11.jpg", "ppt/media/image13.jpg", "ppt/media/image7.jpg"},
}

// MaxPicks returns how many photos may be picked for slideKey: exactly the
// number of photo slots the template has on that slide (0 for unknown keys).
func MaxPicks(slideKey string) int {
	return len(slideImageOrder[slideKey])
}

var (
	placeholderOnce  sync.Once
	placeholderBytes []byte
	placeholderErr   error
)

// placeholderJPEG returns a 1600x1200 solid light-gray JPEG, generated once
// in-process. It replaces every content-photo slot that has no picked photo,
// so no reference-deck photo ever leaks into a generated PPTX.
func placeholderJPEG() ([]byte, error) {
	placeholderOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
		draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 0xE5, G: 0xE7, B: 0xEB, A: 0xFF}}, image.Point{}, draw.Src)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			placeholderErr = fmt.Errorf("encode placeholder jpeg: %w", err)
			return
		}
		placeholderBytes = buf.Bytes()
	})
	return placeholderBytes, placeholderErr
}

// xmlEscape escapes a substituted value so it is safe inside an <a:t> element.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// RenderPPTX opens the master PPTX template as a ZIP archive, replaces every
// placeholder token in each slide's XML with the matching value from
// preview, overwrites every content-photo slot on slides 5-8 — with the
// picked photo's JPEG bytes (keyed by slide_key, in slot order) when one is
// available, otherwise with a neutral placeholder JPEG — and returns the
// re-zipped archive as bytes. No reference-deck photo survives rendering.
func RenderPPTX(templateData []byte, preview Preview, photoBytes map[string][][]byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(templateData), int64(len(templateData)))
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

	placeholder, err := placeholderJPEG()
	if err != nil {
		return nil, err
	}
	imageOverrides := map[string][]byte{}
	for slideKey, files := range slideImageOrder {
		pics := photoBytes[slideKey]
		for i, name := range files {
			if i < len(pics) && len(pics[i]) > 0 {
				imageOverrides[name] = pics[i]
			} else {
				imageOverrides[name] = placeholder
			}
		}
	}
	for _, files := range placeholderOnlySlots {
		for _, name := range files {
			imageOverrides[name] = placeholder
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
