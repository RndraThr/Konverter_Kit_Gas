package activities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/go-pdf/fpdf"
)

const (
	// MaxScanPageBytes caps the size of a single scanned page image before
	// it is assembled into the combined PDF.
	MaxScanPageBytes int64 = 6 << 20
	// MaxScanRequestBytes caps the size of the resulting assembled PDF.
	MaxScanRequestBytes int64 = 60 << 20
	maxScanPages              = 30
)

var (
	ErrScanPagesRequired = errors.New("at least one scanned page is required")
	ErrScanTooManyPages  = errors.New("too many scanned pages in one document")
)

// ScanUploadInput describes a set of camera-captured (or gallery-picked)
// page images to assemble into a single PDF and store as a dokumen_pendukung
// upload against a regency's slot list (or an ad-hoc custom name).
type ScanUploadInput struct {
	ProgramID  string
	RegencyID  string
	SlotID     string
	CustomName string
	Source     string
	Pages      []io.Reader
}

// ScanUpload assembles the given page images into a single PDF document (one
// page per image) and stores it exactly like a normal dokumen_pendukung
// upload.
func (s *Service) ScanUpload(ctx context.Context, actor auth.Principal, input ScanUploadInput, meta auth.ClientMeta, scope auth.RegencyScope) (ActivityMedia, error) {
	if len(input.Pages) == 0 {
		return ActivityMedia{}, ErrScanPagesRequired
	}
	if len(input.Pages) > maxScanPages {
		return ActivityMedia{}, ErrScanTooManyPages
	}
	if input.Source != "camera" && input.Source != "gallery" {
		input.Source = "camera"
	}
	if input.ProgramID == "" || s.programContexts == nil {
		return ActivityMedia{}, ErrProgramRequired
	}

	nameSegment, err := s.resolveDocumentNameSegment(ctx, input.RegencyID, input.SlotID, input.CustomName, scope)
	if err != nil {
		return ActivityMedia{}, err
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, input.ProgramID, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}
	regency, err := s.repository.GetRegency(ctx, input.RegencyID, scope)
	if err != nil {
		return ActivityMedia{}, err
	}

	pdfBytes, err := assembleScanPDF(input.Pages)
	if err != nil {
		return ActivityMedia{}, err
	}

	detected, err := media.DetectUpload(bytes.NewReader(pdfBytes), "scan.pdf")
	if err != nil {
		return ActivityMedia{}, fmt.Errorf("detect assembled pdf: %w", err)
	}

	category, child := folderDestination("dokumen_pendukung")
	folderPath, err := media.BuildFolderPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName,
		RegencyName: storageContext.RegencyName, Category: category, Child: child,
	})
	if err != nil {
		return ActivityMedia{}, err
	}
	displayName := fmt.Sprintf("%s-%s-%s", regency.DocumentCode, nameSegment, s.now().Format("20060102-150405"))

	return s.store(ctx, actor, preparedUpload{
		ActivityType: "dokumen_pendukung", ProgramID: input.ProgramID, RegencyID: input.RegencyID,
		DisplayName: displayName, OriginalFilename: displayName + ".pdf", Source: input.Source, Detected: detected,
	}, folderPath, meta)
}

// scanPaperSizes are the standard papers (portrait, millimetres) a scanned
// page is matched to by its proportions: A4, F4, Letter, Legal.
var scanPaperSizes = [][2]float64{{210, 297}, {215, 330}, {216, 279}, {216, 356}}

// scanPaperFor returns the portrait size of the standard paper closest to a
// width x height page (within 6 % of its proportions), or A4.
func scanPaperFor(width, height float64) (float64, float64) {
	ratio := max(width, height) / min(width, height)
	best, bestError := scanPaperSizes[0], 0.06
	for _, paper := range scanPaperSizes {
		target := paper[1] / paper[0]
		if e := math.Abs(ratio-target) / target; e < bestError {
			best, bestError = paper, e
		}
	}
	return best[0], best[1]
}

// assembleScanPDF validates each page as an allowed image and renders it on
// the standard paper it matches (else A4) in a single combined PDF,
// returning the assembled bytes.
func assembleScanPDF(pages []io.Reader) ([]byte, error) {
	doc := fpdf.New("P", "mm", "A4", "")
	doc.SetAutoPageBreak(false, 0)

	for i, page := range pages {
		limited := io.LimitReader(page, MaxScanPageBytes+1)
		detected, err := media.DetectUpload(limited, fmt.Sprintf("page-%d.jpg", i))
		if err != nil {
			return nil, ErrMediaTypeInvalid
		}
		if detected.Kind != media.KindImage {
			return nil, ErrMediaTypeInvalid
		}
		imageType := scanImageType(detected.MimeType)
		if imageType == "" {
			// fpdf (and this scan pipeline) only supports JPEG and PNG
			// pages; reject anything else (e.g. WebP) rather than
			// mislabeling it and producing a corrupt PDF.
			return nil, ErrMediaTypeInvalid
		}
		imageBytes, err := io.ReadAll(io.LimitReader(detected.Reader, MaxScanPageBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(imageBytes)) > MaxScanPageBytes {
			return nil, ErrFileTooLarge
		}

		imageName := fmt.Sprintf("page-%d", i)
		info := doc.RegisterImageOptionsReader(imageName, fpdf.ImageOptions{ImageType: imageType, ReadDpi: true}, bytes.NewReader(imageBytes))
		if doc.Err() || info == nil || info.Width() <= 0 || info.Height() <= 0 {
			return nil, fmt.Errorf("register scanned page %d: %w", i, doc.Error())
		}
		// Keep the page's proportions: the matching paper in the image's
		// orientation, the image fitted and centred (stretching it to the full
		// page squashed landscape photos and odd-sized documents).
		pageWidthMM, pageHeightMM := scanPaperFor(info.Width(), info.Height())
		pageW, pageH := pageWidthMM, pageHeightMM
		orientation := "P"
		if info.Width() > info.Height() {
			pageW, pageH = pageHeightMM, pageWidthMM
			orientation = "L"
		}
		doc.AddPageFormat(orientation, fpdf.SizeType{Wd: pageWidthMM, Ht: pageHeightMM})
		scale := min(pageW/info.Width(), pageH/info.Height())
		drawW, drawH := info.Width()*scale, info.Height()*scale
		doc.ImageOptions(imageName, (pageW-drawW)/2, (pageH-drawH)/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: imageType}, 0, "")
	}
	if doc.Err() {
		return nil, fmt.Errorf("assemble scanned pdf: %w", doc.Error())
	}

	var buffer bytes.Buffer
	if err := doc.Output(&buffer); err != nil {
		return nil, fmt.Errorf("assemble scanned pdf: %w", err)
	}
	if int64(buffer.Len()) > MaxScanRequestBytes {
		return nil, ErrFileTooLarge
	}
	return buffer.Bytes(), nil
}

// scanImageType maps a detected image MIME type to the fpdf ImageType it
// needs to render that page. fpdf only supports JPEG and PNG, so anything
// else (e.g. WebP) returns "" and must be rejected by the caller.
func scanImageType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "JPG"
	case "image/png":
		return "PNG"
	default:
		return ""
	}
}
