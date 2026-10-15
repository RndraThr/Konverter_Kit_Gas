package finalreport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"konkit/internal/auth"
	"konkit/internal/media"
	"konkit/internal/programs"
)

type programContextResolver interface {
	ResolveStorageContext(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (programs.StorageContext, error)
}

type Service struct {
	repo            *Repository
	programContexts programContextResolver
	storage         media.Storage
	templateData    []byte
}

func NewService(repo *Repository, programContexts programContextResolver, storage media.Storage, templateData []byte) *Service {
	return &Service{repo: repo, programContexts: programContexts, storage: storage, templateData: templateData}
}

// NewServiceForTest builds a Service with a nil repository, for tests that
// only exercise input validation (which short-circuits before any
// repository call).
func NewServiceForTest(repo *Repository, programContexts programContextResolver) *Service {
	return &Service{repo: repo, programContexts: programContexts}
}

// requireContext trims and validates the program/regency pair every
// per-kabupaten operation needs, so blank input is a 400, never a SQL error.
func requireContext(programID, regencyID string) (string, string, error) {
	programID, regencyID = strings.TrimSpace(programID), strings.TrimSpace(regencyID)
	if programID == "" {
		return "", "", ErrProgramRequired
	}
	if regencyID == "" {
		return "", "", ErrRegencyRequired
	}
	return programID, regencyID, nil
}

func (s *Service) GetSettings(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Settings, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return Settings{}, err
	}
	return s.repo.GetSettings(ctx, programID, regencyID, scope)
}

func (s *Service) PutSettings(ctx context.Context, actor auth.Principal, input Settings, scope auth.RegencyScope) (Settings, error) {
	var err error
	input.ProgramID, input.RegencyID, err = requireContext(input.ProgramID, input.RegencyID)
	if err != nil {
		return Settings{}, err
	}
	if input.LeaderCount < 0 || input.AdminCount < 0 || input.TechnicianCount < 0 || input.HelperCount < 0 {
		return Settings{}, ErrCountNegative
	}
	input.MachineManufacturerName = strings.TrimSpace(input.MachineManufacturerName)
	return s.repo.PutSettings(ctx, actor, input, scope)
}

func (s *Service) ListPhotoPicks(ctx context.Context, programID, regencyID, slideKey string, scope auth.RegencyScope) ([]PhotoPick, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return nil, err
	}
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	return s.repo.ListPhotoPicks(ctx, programID, regencyID, slideKey, scope)
}

func (s *Service) ReplacePhotoPicks(ctx context.Context, programID, regencyID, slideKey string, activityMediaIDs []string, scope auth.RegencyScope) ([]PhotoPick, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return nil, err
	}
	if !validSlideKeys[slideKey] {
		return nil, ErrSlideKeyInvalid
	}
	if len(activityMediaIDs) > MaxPicks(slideKey) {
		return nil, ErrTooManyPicks
	}
	seen := make(map[string]bool, len(activityMediaIDs))
	for _, id := range activityMediaIDs {
		if seen[id] {
			return nil, ErrDuplicatePick
		}
		seen[id] = true
	}
	return s.repo.ReplacePhotoPicks(ctx, programID, regencyID, slideKey, activityMediaIDs, scope)
}

func (s *Service) ListDocuments(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) ([]Document, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDocuments(ctx, programID, regencyID, scope)
}

// OpenDocument opens any generated version (active or superseded) within
// scope for download. Unknown or out-of-scope ids are ErrNotFound.
func (s *Service) OpenDocument(ctx context.Context, id string, scope auth.RegencyScope) (DocumentContent, error) {
	doc, err := s.repo.GetDocument(ctx, strings.TrimSpace(id), scope)
	if err != nil {
		return DocumentContent{}, err
	}
	reader, err := s.storage.Open(ctx, doc.StorageKey)
	if err != nil {
		return DocumentContent{}, fmt.Errorf("open final report document: %w", err)
	}
	return DocumentContent{Filename: doc.Filename, Reader: reader}, nil
}

func (s *Service) Preview(ctx context.Context, programID, regencyID string, scope auth.RegencyScope) (Preview, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return Preview{}, err
	}
	data, err := s.repo.GetPreviewSourceData(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}
	settings, err := s.repo.GetSettings(ctx, programID, regencyID, scope)
	if err != nil {
		return Preview{}, err
	}

	picks := map[string][]PhotoPick{}
	for _, key := range []string{SlideKeyPengirimanPenerimaan, SlideKeyRapatKoordinasi, SlideKeySosialisasi} {
		list, err := s.repo.ListPhotoPicks(ctx, programID, regencyID, key, scope)
		if err != nil {
			return Preview{}, err
		}
		picks[key] = list
	}

	deliveryStart, deliveryEnd := earliestLatest(settings.DeliveryHoseDate, settings.DeliveryMachineGroupDate, settings.DeliveryGasCylinderDate)

	return Preview{
		RegencyLabel:             orBlank(regencyLabel(data.regencyName)),
		FiscalYear:               orBlank(itoaOrEmpty(data.fiscalYear)),
		AgricultureOfficeName:    orBlank(data.agricultureOfficeName),
		ConsultantCompanyName:    orBlank(data.consultantCompanyName),
		MachineManufacturerName:  orBlank(settings.MachineManufacturerName),
		DeliveryHoseDate:         orBlank(formatIndonesianDate(settings.DeliveryHoseDate)),
		DeliveryMachineGroupDate: orBlank(formatIndonesianDate(settings.DeliveryMachineGroupDate)),
		DeliveryGasCylinderDate:  orBlank(formatIndonesianDate(settings.DeliveryGasCylinderDate)),
		DeliveryRangeStart:       orBlank(formatIndonesianDate(deliveryStart)),
		DeliveryRangeEnd:         orBlank(formatIndonesianDate(deliveryEnd)),
		RakordaDate:              orBlank(formatIndonesianDate(data.rakordaDate)),
		SosialisasiDate:          orBlank(formatIndonesianDate(data.sosialisasiDate)),
		LeaderCount:              itoaOrEmpty(settings.LeaderCount),
		AdminCount:               itoaOrEmpty(settings.AdminCount),
		TechnicianCount:          itoaOrEmpty(settings.TechnicianCount),
		HelperCount:              itoaOrEmpty(settings.HelperCount),
		HandoverLocation:         orBlank(data.handoverLocation),
		DistributionStartDate:    orBlank(formatIndonesianDate(data.distributionStart)),
		DistributionEndDate:      orBlank(formatIndonesianDate(data.distributionEnd)),
		DistributionTotal:        orBlank(itoaOrEmptyNonZero(data.distributionTotal)),
		PhotoPicks:               picks,
	}, nil
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return "0"
	}
	return strconv.Itoa(n)
}

func itoaOrEmptyNonZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// earliestLatest returns the min/max of the non-empty ISO (YYYY-MM-DD)
// dates among the 3 delivery-date settings fields; ISO strings sort
// chronologically, and formatting happens afterwards.
func earliestLatest(dates ...string) (string, string) {
	var nonEmpty []string
	for _, d := range dates {
		if d != "" {
			nonEmpty = append(nonEmpty, d)
		}
	}
	if len(nonEmpty) == 0 {
		return "", ""
	}
	sort.Strings(nonEmpty)
	return nonEmpty[0], nonEmpty[len(nonEmpty)-1]
}

var indonesianMonths = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// formatIndonesianDate renders an ISO YYYY-MM-DD date as "7 Mei 2026"
// (Indonesian month name, no leading zero). Empty or invalid input yields
// "" so the caller's orBlank turns it into "Belum diisi".
func formatIndonesianDate(iso string) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(iso))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", parsed.Day(), indonesianMonths[parsed.Month()-1], parsed.Year())
}

// regencyLabel turns regencies.name ("KAB. BIREUEN", "KOTA LANGSA") into the
// title-slide label: "KAB. " expands to "KABUPATEN ", anything else is used
// as-is, uppercased.
func regencyLabel(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if rest, ok := strings.CutPrefix(upper, "KAB. "); ok {
		return "KABUPATEN " + strings.TrimSpace(rest)
	}
	return upper
}

// documentFilename is "LAPORAN AKHIR - PETANI KAB. BIREUEN.pptx" /
// "LAPORAN AKHIR - NELAYAN KOTA LANGSA.pptx".
func documentFilename(programType programs.ProgramType, regencyName string) string {
	audience := strings.ToUpper(string(programType))
	switch programType {
	case programs.ProgramFarmer:
		audience = "PETANI"
	case programs.ProgramFisherman:
		audience = "NELAYAN"
	}
	return fmt.Sprintf("LAPORAN AKHIR - %s %s.pptx", audience, strings.ToUpper(strings.TrimSpace(regencyName)))
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// slotJPEG returns bytes suitable for a template .jpg slot: JPEG input
// passes through unchanged, PNG input is transcoded to JPEG, anything else
// is an error.
func slotJPEG(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, pngSignature) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode png: %w", err)
		}
		var out bytes.Buffer
		if err := jpeg.Encode(&out, flattenOnWhite(img), &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		return out.Bytes(), nil
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("photo is neither JPEG nor PNG: %w", err)
	}
	return data, nil
}

// flattenOnWhite composites a possibly-transparent image onto white so
// transparent PNG regions do not turn black in the JPEG.
func flattenOnWhite(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			inv := 0xffff - a
			dst.Pix[dst.PixOffset(x, y)+0] = uint8((r + inv) >> 8)
			dst.Pix[dst.PixOffset(x, y)+1] = uint8((g + inv) >> 8)
			dst.Pix[dst.PixOffset(x, y)+2] = uint8((b + inv) >> 8)
			dst.Pix[dst.PixOffset(x, y)+3] = 0xff
		}
	}
	return dst
}

// newStorageKey returns a fresh, media-valid key per generation so a new
// version never overwrites (or, on failure cleanup, deletes) the file that a
// previous version points at.
func newStorageKey() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", fmt.Errorf("generate storage key: %w", err)
	}
	return "finalreport-" + hex.EncodeToString(value), nil
}

// loadPickPhoto reads one picked photo and converts it for a .jpg slot.
func (s *Service) loadPickPhoto(ctx context.Context, pick PhotoPick) ([]byte, error) {
	reader, err := s.storage.Open(ctx, pick.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return slotJPEG(data)
}

func (s *Service) Generate(ctx context.Context, actor auth.Principal, programID, regencyID string, scope auth.RegencyScope, meta auth.ClientMeta) (Document, error) {
	programID, regencyID, err := requireContext(programID, regencyID)
	if err != nil {
		return Document{}, err
	}
	preview, err := s.Preview(ctx, programID, regencyID, scope)
	if err != nil {
		return Document{}, err
	}

	photoBytes := map[string][][]byte{}
	for slideKey, picks := range preview.PhotoPicks {
		for _, pick := range picks {
			data, err := s.loadPickPhoto(ctx, pick)
			if err != nil {
				// An unreachable or unreadable pick is skipped (its slot gets the placeholder), not fatal.
				log.Printf("final report: skipping photo pick %s (activity_media %s): %v", pick.ID, pick.ActivityMediaID, err)
				continue
			}
			photoBytes[slideKey] = append(photoBytes[slideKey], data)
		}
	}

	rendered, err := RenderPPTX(s.templateData, preview, photoBytes)
	if err != nil {
		return Document{}, fmt.Errorf("render pptx: %w", err)
	}

	storageContext, err := s.programContexts.ResolveStorageContext(ctx, programID, regencyID, scope)
	if err != nil {
		return Document{}, fmt.Errorf("resolve storage context: %w", err)
	}
	folderPath, err := media.BuildFinalReportPath(media.FolderPathInput{
		ProgramType: string(storageContext.ProgramType), ZoneName: storageContext.ZoneName, RegencyName: storageContext.RegencyName,
	})
	if err != nil {
		return Document{}, fmt.Errorf("build folder path: %w", err)
	}
	filename := documentFilename(storageContext.ProgramType, storageContext.RegencyName)

	key, err := newStorageKey()
	if err != nil {
		return Document{}, err
	}
	storageKey, _, checksum, err := media.PutNamed(ctx, s.storage, key, filename, folderPath, bytes.NewReader(rendered))
	if err != nil {
		return Document{}, fmt.Errorf("upload pptx: %w", err)
	}

	// Superseded versions keep their files (spec §3.3); only the new upload is
	// cleaned up when its row cannot be recorded.
	doc, err := s.repo.InsertDocumentVersion(ctx, actor, programID, regencyID, filename, checksum, storageKey, scope, meta)
	if err != nil {
		_ = s.storage.Delete(context.Background(), storageKey) // unique key: never touches another version's file
		return Document{}, fmt.Errorf("insert document version: %w", err)
	}
	return doc, nil
}
