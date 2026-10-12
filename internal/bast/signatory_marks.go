package bast

import (
	"bytes"
	"context"
	"image/png"
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

// SignatureRule menentukan apakah tanda tangan dan stempel satu pihak dicetak.
type SignatureRule struct {
	Signature bool `json:"signature"`
	Stamp     bool `json:"stamp"`
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
		if validSignatoryRole(role) {
			result[role] = rule
		}
	}
	return result
}

// SignatureMark adalah tanda tangan/stempel satu pihak yang dicetak pada satu
// BA; disimpan di snapshot BA final sebagai jejak.
type SignatureMark struct {
	Signature bool `json:"signature,omitempty"`
	Stamp     bool `json:"stamp,omitempty"`
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

const (
	// stampMaxMM: diameter/lebar stempel maksimum; di kotak rendah (BA
	// Perorangan) stempel mengecil mengikuti tinggi kotak.
	stampMaxMM = 22.0
	// stampRotationDeg: kemiringan kecil seperti cap basah (searah jarum jam negatif).
	stampRotationDeg = -8.0
)

// drawSignatoryMark menggambar tanda tangan (proporsional, di tengah kotak)
// lalu stempel di posisi tetap: menimpa sepertiga kiri tanda tangan, sedikit
// miring, dengan mode campur Multiply agar tanda tangan tetap terlihat, dan
// dipotong pada batas kotak agar tidak menimpa kolom lain.
func drawSignatoryMark(pdf *fpdf.Fpdf, role string, mark SignatureMark, x, y, w, h float64) {
	signatureLeft, signatureWidth := x+w/2, 0.0
	if mark.Signature {
		key := signatoryImageKey(role, SignatoryKindSignature)
		if info := pdf.GetImageInfo(key); info != nil && info.Width() > 0 && info.Height() > 0 {
			boxW, boxH := minFloat(w-6, 60), h-4
			ratio := info.Width() / info.Height()
			drawW, drawH := boxW, boxW/ratio
			if drawH > boxH {
				drawH, drawW = boxH, boxH*ratio
			}
			signatureLeft, signatureWidth = x+(w-drawW)/2, drawW
			pdf.ImageOptions(key, signatureLeft, y+(h-drawH)/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
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
	size := minFloat(stampMaxMM, h-2)
	drawW, drawH := size, size*info.Height()/info.Width()
	if drawH > size {
		drawH, drawW = size, size*info.Width()/info.Height()
	}
	cx := signatureLeft + signatureWidth*0.25
	cx = maxFloat(x+drawW/2+1, minFloat(cx, x+w-drawW/2-1))
	cy := y + h/2
	pdf.ClipRect(x, y, w, h, false)
	pdf.SetAlpha(0.92, "Multiply")
	pdf.TransformBegin()
	pdf.TransformRotate(-stampRotationDeg, cx, cy)
	pdf.ImageOptions(key, cx-drawW/2, cy-drawH/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")
	pdf.ClipEnd()
}
