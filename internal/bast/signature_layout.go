package bast

import (
	"context"
	"strings"

	"konkit/internal/auth"

	"github.com/go-pdf/fpdf"
)

// Editor tanda tangan: halaman tanda tangan BA dirender tanpa tanda tangan
// digital/stempel, sambil mencatat letak (mm) setiap kotak tanda tangan.
// Frontend menampilkan halaman itu sebagai latar lalu menaruh tanda tangan
// dan stempel di atasnya pada koordinat yang sama seperti saat dicetak.

// SignatureBox adalah kotak tanda tangan satu pihak pada halaman PDF (mm).
type SignatureBox struct {
	Role string  `json:"role"`
	Page int     `json:"page"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// SignatureLayout adalah isi editor tanda tangan satu BA: PDF halaman tanda
// tangan, nomor halaman yang memuat tabel, ukuran halaman, dan kotak per pihak.
type SignatureLayout struct {
	DocumentKey string         `json:"document_key"`
	Page        int            `json:"page"`
	PageWidth   float64        `json:"page_width"`
	PageHeight  float64        `json:"page_height"`
	Boxes       []SignatureBox `json:"boxes"`
	PDF         []byte         `json:"pdf"`
}

type signatureLayoutRecorder struct {
	boxes []SignatureBox
	sizes map[int][2]float64
}

func (r *signatureLayoutRecorder) record(pdf *fpdf.Fpdf, role string, x, y, w, h float64) {
	page := pdf.PageNo()
	if r.sizes == nil {
		r.sizes = map[int][2]float64{}
	}
	width, height := pdf.GetPageSize()
	r.sizes[page] = [2]float64{width, height}
	r.boxes = append(r.boxes, SignatureBox{Role: role, Page: page, X: x, Y: y, W: w, H: h})
}

type signatureLayoutKey struct{}

func withSignatureLayout(ctx context.Context) (context.Context, *signatureLayoutRecorder) {
	recorder := &signatureLayoutRecorder{}
	return context.WithValue(ctx, signatureLayoutKey{}, recorder), recorder
}

func signatureLayoutFrom(ctx context.Context) *signatureLayoutRecorder {
	recorder, _ := ctx.Value(signatureLayoutKey{}).(*signatureLayoutRecorder)
	return recorder
}

// layoutMarks menggantikan tanda yang dicetak saat merekam letak kotak: setiap
// pihak BA mendapat penanda perekam, dan tidak ada gambar yang digambar.
func layoutMarks(recorder *signatureLayoutRecorder, documentKey string) SignatureMarks {
	marks := SignatureMarks{}
	for _, role := range signatureDocuments[documentKey] {
		marks[role] = SignatureMark{layout: recorder}
	}
	return marks
}

// PreviewFunc menjadikan fungsi preview berbentuk (jadwal, tanggal) sebagai
// WetPreviewer, untuk BA yang preview-nya memakai parameter lain.
type PreviewFunc func(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error)

func (f PreviewFunc) Preview(ctx context.Context, scheduleID, date string, scope auth.RegencyScope) (AggregatePreview, error) {
	return f(ctx, scheduleID, date, scope)
}

// SignatureLayoutService menyiapkan isi editor tanda tangan untuk semua BA
// yang memiliki tabel tanda tangan.
type SignatureLayoutService struct {
	documents map[string]WetPreviewer
}

func NewSignatureLayoutService(documents map[string]WetPreviewer) *SignatureLayoutService {
	return &SignatureLayoutService{documents: documents}
}

func (s *SignatureLayoutService) Layout(ctx context.Context, documentKey, scheduleID, date string, scope auth.RegencyScope) (SignatureLayout, error) {
	documentKey = strings.TrimSpace(documentKey)
	previewer, ok := s.documents[documentKey]
	if !ok || previewer == nil || !validSignatureDocument(documentKey) || !validWetInput(scheduleID, date) {
		return SignatureLayout{}, ErrInvalidInput
	}
	layoutCtx, recorder := withSignatureLayout(ctx)
	preview, err := previewer.Preview(layoutCtx, strings.TrimSpace(scheduleID), strings.TrimSpace(date), scope)
	if err != nil {
		return SignatureLayout{}, err
	}
	if len(recorder.boxes) == 0 {
		return SignatureLayout{}, ErrInvalidInput
	}
	// Tabel pertama saja (BA Perorangan dan Pemeriksaan bisa berulang per halaman).
	page := recorder.boxes[0].Page
	layout := SignatureLayout{DocumentKey: documentKey, Page: page, PageWidth: recorder.sizes[page][0], PageHeight: recorder.sizes[page][1], PDF: preview.PDF}
	seen := map[string]bool{}
	for _, box := range recorder.boxes {
		if box.Page == page && !seen[box.Role] {
			seen[box.Role] = true
			layout.Boxes = append(layout.Boxes, box)
		}
	}
	return layout, nil
}
