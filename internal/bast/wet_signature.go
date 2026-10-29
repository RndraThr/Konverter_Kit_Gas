package bast

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Tanda tangan basah: halaman tanda tangan (halaman terakhir) BA dicetak,
// ditandatangani dan distempel basah, dipindai, lalu diunggah. Preview dan
// PDF final memakai halaman pindaian itu menggantikan halaman tanda tangan
// buatan sistem. Kode dokumen di kaki halaman cetak mengikat pindaian ke isi
// dokumen: bila isi berubah, pindaian lama tidak dipakai lagi.

// ErrWetSignatureRequired: mode tanda tangan basah aktif tetapi halaman
// bertanda tangan untuk isi dokumen saat ini belum diunggah.
var ErrWetSignatureRequired = errors.New("signed signature page scan is required")

// wetSignatureDocuments adalah BA yang tanda tangannya berada di halaman
// terakhir tersendiri (Servis Berkala satu halaman: seluruh halaman diganti).
// BA Pemeriksaan memakai satu pindaian untuk halaman tanda tangan setiap
// form (isinya sama). RAKORDA tidak punya tabel penandatangan.
var wetSignatureDocuments = map[string]bool{
	"dp3": true, "rekap-harian": true, "closing-titik-serah": true,
	"closing-kabupaten": true, "servis-berkala": true, "tkdn": true,
	"gas-elpiji": true, "sosialisasi": true, "training-10": true,
	"training-100": true, "ba-pemeriksaan": true,
}

const wetScanImageKey = "wetscan"

// WetSignature menyertai snapshot BA. PageOnly hanya untuk mencetak halaman
// tanda tangan (tidak disimpan).
type WetSignature struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Code     string `json:"code,omitempty"`
	Scanned  bool   `json:"scanned,omitempty"`
	PageOnly bool   `json:"-"`
}

// wetPageRequest menandai permintaan "cetak halaman tanda tangan": Preview BA
// yang menerima konteks ini hanya merender halaman tanda tangan (tanpa
// pindaian) dan menuliskan kode dokumennya ke sini.
type wetPageRequest struct{ code string }

type wetPageKey struct{}

func withWetPage(ctx context.Context) (context.Context, *wetPageRequest) {
	request := &wetPageRequest{}
	return context.WithValue(ctx, wetPageKey{}, request), request
}

func wetPageFrom(ctx context.Context) *wetPageRequest {
	request, _ := ctx.Value(wetPageKey{}).(*wetPageRequest)
	return request
}

// contentCode adalah sidik isi dokumen (8 karakter) tanpa bagian yang tidak
// mengubah isi halaman yang ditandatangani: tanda tangan digital, status
// tanda tangan basah, serta nomor versi dan nomor dokumen (ikut versi).
func contentCode(documentKey, date string, snapshot any) string {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	for _, key := range []string{"signature_marks", "wet_signature", "version", "document_number"} {
		delete(fields, key)
	}
	normalized, _ := json.Marshal(fields)
	sum := sha256.Sum256(append([]byte(documentKey+"|"+date+"|"), normalized...))
	return strings.ToUpper(hex.EncodeToString(sum[:])[:8])
}

// wetSignatureLoader dipenuhi Repository; stub uji tanpa metode ini berarti
// mode tanda tangan basah tidak aktif.
type wetSignatureLoader interface {
	ResolveWetSignature(ctx context.Context, scheduleID, documentKey, date string) (enabled bool, scan []byte, scanCode string, err error)
}

// loadWetSignature menentukan status tanda tangan basah satu BA dari isi
// snapshot-nya, dan mengembalikan gambar pindaian yang masih cocok.
func loadWetSignature(ctx context.Context, repository any, scheduleID, documentKey, date string, snapshot any) (WetSignature, map[string][]byte, error) {
	if !wetSignatureDocuments[wetBaseKey(documentKey)] {
		return WetSignature{}, nil, nil
	}
	if signatureLayoutFrom(ctx) != nil {
		return WetSignature{PageOnly: true}, nil, nil
	}
	code := contentCode(documentKey, date, snapshot)
	if request := wetPageFrom(ctx); request != nil {
		request.code = code
		return WetSignature{Enabled: true, Code: code, PageOnly: true}, nil, nil
	}
	loader, ok := repository.(wetSignatureLoader)
	if !ok {
		return WetSignature{}, nil, nil
	}
	enabled, scan, scanCode, err := loader.ResolveWetSignature(ctx, scheduleID, documentKey, date)
	if err != nil || !enabled {
		return WetSignature{}, nil, err
	}
	wet := WetSignature{Enabled: true, Code: code}
	if len(scan) > 0 && scanCode == code {
		wet.Scanned = true
		return wet, map[string][]byte{wetScanImageKey: scan}, nil
	}
	return wet, nil, nil
}

// registerWetScan mendaftarkan PNG halaman pindaian ke PDF bila ada.
func registerWetScan(pdf *fpdf.Fpdf, images map[string][]byte) {
	registerWetScanAs(pdf, images, wetScanImageKey)
}

// registerWetScanAs mendaftarkan pindaian dengan kunci gambar tertentu (BA
// gabungan memuat beberapa pindaian, satu per halaman).
func registerWetScanAs(pdf *fpdf.Fpdf, images map[string][]byte, key string) {
	data, ok := images[key]
	if !ok {
		return
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return
	}
	pdf.RegisterImageOptionsReader(key, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(data))
	if pdf.Err() {
		pdf.ClearError()
	}
}

// drawWetScanPage menambahkan halaman berisi pindaian halaman tanda tangan,
// diperkecil proporsional agar muat di halaman A4 dengan margin tipis.
func drawWetScanPage(pdf *fpdf.Fpdf, orientation string) {
	drawWetScanPageAs(pdf, orientation, wetScanImageKey)
}

func drawWetScanPageAs(pdf *fpdf.Fpdf, orientation, key string) {
	pdf.AddPageFormat(orientation, pdf.GetPageSizeStr("A4"))
	info := pdf.GetImageInfo(key)
	if info == nil || info.Width() <= 0 || info.Height() <= 0 {
		return
	}
	pageW, pageH := pdf.GetPageSize()
	const margin = 4.0
	boxW, boxH := pageW-2*margin, pageH-2*margin
	drawW, drawH := boxW, boxW*info.Height()/info.Width()
	if drawH > boxH {
		drawH, drawW = boxH, boxH*info.Width()/info.Height()
	}
	pdf.ImageOptions(key, (pageW-drawW)/2, (pageH-drawH)/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
}

// printWetFooter: tulisan kode dokumen di kaki halaman tanda tangan basah.
// Disembunyikan atas permintaan pengguna; kode tetap dipakai untuk mencocokkan
// pindaian dengan isi dokumen dan tampil di aplikasi.
const printWetFooter = false

// drawWetFooter menulis kode dokumen di kaki halaman tanda tangan yang
// dicetak untuk ditandatangani basah (bila printWetFooter aktif).
func drawWetFooter(pdf *fpdf.Fpdf, wet WetSignature, title, date string) {
	if !printWetFooter || !wet.Enabled || wet.Code == "" {
		return
	}
	pageW, pageH := pdf.GetPageSize()
	text := fmt.Sprintf("%s · %s · Kode dokumen %s — setelah ditandatangani, pindai dan unggah halaman ini di menu Berita Acara.", title, formatIndonesianDate(date), wet.Code)
	pdf.SetFont(activityFont, "", 7.5)
	pdf.SetTextColor(90, 90, 90)
	pdf.Text((pageW-pdf.GetStringWidth(text))/2, pageH-6, text)
	pdf.SetTextColor(0, 0, 0)
}

// nextVersionOrFirst mengembalikan nomor versi yang akan dipakai finalisasi
// berikutnya, agar nomor dokumen di preview (dan halaman yang dicetak untuk
// tanda tangan basah) sama dengan PDF final. Bila gagal, versi 1.
func nextVersionOrFirst(ctx context.Context, repository interface {
	NextAggregateVersion(context.Context, string, string, string) (int, error)
}, scheduleID, documentType, documentDate string) int {
	version, err := repository.NextAggregateVersion(ctx, scheduleID, documentType, documentDate)
	if err != nil || version < 1 {
		return 1
	}
	return version
}
