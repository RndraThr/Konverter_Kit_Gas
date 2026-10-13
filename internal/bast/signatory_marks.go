package bast

import (
	"bytes"
	"context"
	"image/png"
	"math"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Tanda tangan digital dan stempel pihak penandatangan BA (Dinas, Pelaksana,
// Konsultan, Pertamina), disimpan per jadwal (kabupaten) bersama nama/NIP di
// Konfigurasi BA. Aturan cetak (global jadwal atau khusus per BA) menentukan
// apakah tanda tangan dan stempel ikut dicetak.

const (
	SignatoryAgricultureOffice = "agriculture_office"
	SignatoryInstaller         = "installer"
	SignatorySupervisor        = "supervisor"
	SignatoryPertamina         = "pertamina"

	SignatoryKindSignature = "signature"
	SignatoryKindStamp     = "stamp"

	// SignatureRulesGlobal adalah kunci aturan cetak bawaan jadwal.
	SignatureRulesGlobal = "global"
)

func validSignatoryRole(role string) bool {
	switch role {
	case SignatoryAgricultureOffice, SignatoryInstaller, SignatorySupervisor, SignatoryPertamina:
		return true
	}
	return false
}

func validSignatoryKind(kind string) bool {
	return kind == SignatoryKindSignature || kind == SignatoryKindStamp
}

// fourSignatories adalah urutan kolom tanda tangan empat pihak.
var fourSignatories = []string{SignatoryAgricultureOffice, SignatoryInstaller, SignatorySupervisor, SignatoryPertamina}

// signatureDocuments memetakan kunci BA (sama dengan tab di UI) ke pihak yang
// ada di tabel tanda tangannya. Penerima (BA Perorangan, Training) memakai
// tanda tangan slot distribusi dan tidak diatur di sini.
var signatureDocuments = map[string][]string{
	"dp3":                 {SignatoryAgricultureOffice, SignatoryInstaller, SignatorySupervisor},
	"ba-perorangan":       {SignatoryInstaller, SignatorySupervisor},
	"rekap-harian":        fourSignatories,
	"closing-titik-serah": fourSignatories,
	"closing-kabupaten":   fourSignatories,
	"sosialisasi":         fourSignatories,
	"training-10":         fourSignatories,
	"training-100":        fourSignatories,
	"ba-pemeriksaan":      fourSignatories,
	"servis-berkala":      {SignatoryInstaller, SignatoryAgricultureOffice},
	"tkdn":                fourSignatories,
}

// SignatureRule menentukan apakah tanda tangan dan stempel satu pihak dicetak,
// serta posisi stempel (kosong = posisi bawaan).
type SignatureRule struct {
	Signature          bool           `json:"signature"`
	Stamp              bool           `json:"stamp"`
	SignaturePlacement *MarkPlacement `json:"signature_placement,omitempty"`
	StampPlacement     *MarkPlacement `json:"stamp_placement,omitempty"`
}

// MarkPlacement adalah posisi tanda tangan atau stempel di kotak tanda
// tangan, dalam satuan tinggi kotak agar sama di semua BA walau lebar
// kotaknya berbeda: X/Y adalah geseran pusat gambar dari tengah kotak, Size
// sisi terpanjang gambar, dan Rotation kemiringan dalam derajat (positif
// searah jarum jam). Pusat gambar selalu di dalam kotak; gambar boleh keluar
// sedikit dari kotak.
type MarkPlacement struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Size     float64 `json:"size"`
	Rotation float64 `json:"rotation"`
}

const (
	placementMaxX = 3.0
	placementMaxY = 0.5
	// markOverflow: batas gambar boleh keluar dari kotak, satuan tinggi kotak.
	markOverflow = 0.75
)

// placementSizeLimits: batas ukuran (sisi terpanjang, satuan tinggi kotak).
// Tanda tangan umumnya melebar sehingga boleh jauh lebih panjang dari stempel.
var placementSizeLimits = map[string][2]float64{
	SignatoryKindSignature: {0.2, 4},
	SignatoryKindStamp:     {0.3, 2.5},
}

// defaultStampPlacement: menimpa sisi kiri tanda tangan, sedikit miring
// seperti cap basah.
var defaultStampPlacement = MarkPlacement{X: -0.4, Y: 0, Size: 0.78, Rotation: -8}

func clampFloat(value, low, high float64) float64 {
	if math.IsNaN(value) {
		return low
	}
	return math.Max(low, math.Min(high, value))
}

func (p MarkPlacement) normalized(kind string) MarkPlacement {
	limits := placementSizeLimits[kind]
	return MarkPlacement{
		X:        math.Round(clampFloat(p.X, -placementMaxX, placementMaxX)*1000) / 1000,
		Y:        math.Round(clampFloat(p.Y, -placementMaxY, placementMaxY)*1000) / 1000,
		Size:     math.Round(clampFloat(p.Size, limits[0], limits[1])*1000) / 1000,
		Rotation: math.Round(clampFloat(p.Rotation, -180, 180)*10) / 10,
	}
}

func normalizedPlacement(p *MarkPlacement, kind string) *MarkPlacement {
	if p == nil {
		return nil
	}
	normalized := p.normalized(kind)
	return &normalized
}

// SignatureRules adalah aturan cetak per pihak.
type SignatureRules map[string]SignatureRule

// defaultSignatureRules dipakai bila jadwal belum menyimpan aturan global:
// tanda tangan pihak sendiri (Pelaksana) dan Konsultan dicetak, pejabat luar
// (Dinas, Pertamina) tidak tanpa persetujuan eksplisit.
func defaultSignatureRules() SignatureRules {
	return SignatureRules{
		SignatoryAgricultureOffice: {},
		SignatoryInstaller:         {Signature: true, Stamp: true},
		SignatorySupervisor:        {Signature: true},
		SignatoryPertamina:         {},
	}
}

func (rules SignatureRules) normalized() SignatureRules {
	result := SignatureRules{}
	for role, rule := range rules {
		if !validSignatoryRole(role) {
			continue
		}
		rule.SignaturePlacement = normalizedPlacement(rule.SignaturePlacement, SignatoryKindSignature)
		rule.StampPlacement = normalizedPlacement(rule.StampPlacement, SignatoryKindStamp)
		result[role] = rule
	}
	return result
}

// SignatureMark adalah tanda tangan/stempel satu pihak yang dicetak pada satu
// BA; disimpan di snapshot BA final sebagai jejak.
type SignatureMark struct {
	Signature          bool           `json:"signature,omitempty"`
	Stamp              bool           `json:"stamp,omitempty"`
	SignaturePlacement *MarkPlacement `json:"signature_placement,omitempty"`
	StampPlacement     *MarkPlacement `json:"stamp_placement,omitempty"`
	// layout: mode editor, kotak hanya dicatat letaknya (tanpa gambar).
	layout *signatureLayoutRecorder
}

// SignatureMarks memetakan pihak ke tanda yang dicetak.
type SignatureMarks map[string]SignatureMark

func signatoryImageKey(role, kind string) string { return "signatory:" + role + ":" + kind }

// signatoryMarkLoader dipenuhi Repository; stub uji yang tidak
// mengimplementasikannya menghasilkan BA tanpa tanda tangan digital.
type signatoryMarkLoader interface {
	ResolveSignatoryMarks(ctx context.Context, scheduleID, documentKey string) (SignatureMarks, map[string][]byte, error)
}

// loadSignatoryMarks mengambil tanda yang dicetak untuk satu BA beserta gambarnya.
func loadSignatoryMarks(ctx context.Context, repository any, scheduleID, documentKey string) (SignatureMarks, map[string][]byte, error) {
	if recorder := signatureLayoutFrom(ctx); recorder != nil {
		return layoutMarks(recorder, documentKey), nil, nil
	}
	loader, ok := repository.(signatoryMarkLoader)
	if !ok || strings.TrimSpace(scheduleID) == "" {
		return nil, nil, nil
	}
	return loader.ResolveSignatoryMarks(ctx, scheduleID, documentKey)
}

func mergeImages(target, images map[string][]byte) {
	for key, data := range images {
		target[key] = data
	}
}

// registerSignatoryImages mendaftarkan PNG tanda tangan/stempel pihak ke PDF.
func registerSignatoryImages(pdf *fpdf.Fpdf, images map[string][]byte) {
	for key, data := range images {
		if !strings.HasPrefix(key, "signatory:") {
			continue
		}
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			continue
		}
		pdf.RegisterImageOptionsReader(key, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(data))
		if pdf.Err() {
			pdf.ClearError()
		}
	}
}

// drawSignatoryMark menggambar tanda tangan lalu stempel di kotak tanda
// tangan sesuai posisinya. Tanpa posisi tersimpan, tanda tangan dibuat
// proporsional di tengah kotak dan stempel menimpa sisi kiri tanda tangan,
// sedikit miring. Stempel memakai mode campur Multiply agar tanda tangan
// tetap terlihat. Keduanya dipotong sedikit di luar batas kotak.
func drawSignatoryMark(pdf *fpdf.Fpdf, role string, mark SignatureMark, x, y, w, h float64) {
	if mark.layout != nil {
		mark.layout.record(pdf, role, x, y, w, h)
		return
	}
	if mark.Signature {
		key := signatoryImageKey(role, SignatoryKindSignature)
		if info := pdf.GetImageInfo(key); info != nil && info.Width() > 0 && info.Height() > 0 {
			if mark.SignaturePlacement != nil {
				drawPlacedImage(pdf, key, info, mark.SignaturePlacement.normalized(SignatoryKindSignature), x, y, w, h, false)
			} else {
				boxW, boxH := minFloat(w-6, 60), h-4
				ratio := info.Width() / info.Height()
				drawW, drawH := boxW, boxW/ratio
				if drawH > boxH {
					drawH, drawW = boxH, boxH*ratio
				}
				pdf.ImageOptions(key, x+(w-drawW)/2, y+(h-drawH)/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
			}
		}
	}
	if !mark.Stamp {
		return
	}
	key := signatoryImageKey(role, SignatoryKindStamp)
	info := pdf.GetImageInfo(key)
	if info == nil || info.Width() <= 0 || info.Height() <= 0 {
		return
	}
	placement := defaultStampPlacement
	if mark.StampPlacement != nil {
		placement = mark.StampPlacement.normalized(SignatoryKindStamp)
	}
	drawPlacedImage(pdf, key, info, placement, x, y, w, h, true)
}

// drawPlacedImage menggambar gambar terdaftar pada posisi placement di kotak
// (x, y, w, h), dipotong markOverflow di luar batas kotak.
func drawPlacedImage(pdf *fpdf.Fpdf, key string, info *fpdf.ImageInfoType, placement MarkPlacement, x, y, w, h float64, multiply bool) {
	size := placement.Size * h
	drawW, drawH := size, size*info.Height()/info.Width()
	if drawH > size {
		drawH, drawW = size, size*info.Width()/info.Height()
	}
	cx := clampFloat(x+w/2+placement.X*h, x, x+w)
	cy := y + h/2 + placement.Y*h
	margin := markOverflow * h
	pdf.ClipRect(x-margin, y-margin, w+2*margin, h+2*margin, false)
	if multiply {
		pdf.SetAlpha(0.92, "Multiply")
	}
	pdf.TransformBegin()
	pdf.TransformRotate(-placement.Rotation, cx, cy)
	pdf.ImageOptions(key, cx-drawW/2, cy-drawH/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.TransformEnd()
	if multiply {
		pdf.SetAlpha(1, "Normal")
	}
	pdf.ClipEnd()
}
