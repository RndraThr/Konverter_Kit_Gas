package bast

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Tanda tangan asli penerima (diambil di POS Dokumen) untuk BA perorangan dan
// daftar hadir Training. Byte PNG ikut di peta gambar yang sama dengan logo,
// dengan kunci "signature:<ID slot distribusi>". Penerimaan tanpa tanda tangan
// asli tetap memakai paraf buatan (drawRecipientSignature).

const signatureImagePrefix = "signature:"

func signatureImageKey(slotID string) string { return signatureImagePrefix + slotID }

// recipientSignatureLoader dipenuhi Repository; layanan memanggilnya bila ada.
type recipientSignatureLoader interface {
	ListRecipientSignatures(ctx context.Context, slotIDs []string) (map[string][]byte, error)
}

// ListRecipientSignatures mengembalikan PNG tanda tangan penerima saat ini per
// ID slot. Tanda tangan penerima lama (setelah penggantian) diabaikan.
func (r *Repository) ListRecipientSignatures(ctx context.Context, slotIDs []string) (map[string][]byte, error) {
	result := map[string][]byte{}
	if len(slotIDs) == 0 {
		return result, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT sig.distribution_slot_id::text, sig.image_png
		FROM distribution_slot_signatures sig
		JOIN distribution_slots ds ON ds.id = sig.distribution_slot_id AND ds.recipient_person_id = sig.person_id
		WHERE sig.distribution_slot_id::text = ANY($1)
	`, slotIDs)
	if err != nil {
		return nil, fmt.Errorf("list recipient signatures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slotID string
		var image []byte
		if err := rows.Scan(&slotID, &image); err != nil {
			return nil, err
		}
		result[slotID] = image
	}
	return result, rows.Err()
}

// addRecipientSignatures menambahkan tanda tangan slot ke peta gambar render.
func addRecipientSignatures(ctx context.Context, repository any, images map[string][]byte, slotIDs []string) error {
	loader, ok := repository.(recipientSignatureLoader)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(slotIDs))
	for _, id := range slotIDs {
		if id != "" {
			ids = append(ids, id)
		}
	}
	signatures, err := loader.ListRecipientSignatures(ctx, ids)
	if err != nil {
		return err
	}
	for slotID, image := range signatures {
		images[signatureImageKey(slotID)] = image
	}
	return nil
}

// registerSignatures mendaftarkan PNG tanda tangan di peta gambar ke PDF.
// Gambar yang tidak valid dilewati (kembali ke paraf buatan).
func registerSignatures(pdf *fpdf.Fpdf, images map[string][]byte) {
	for key, data := range images {
		if !strings.HasPrefix(key, signatureImagePrefix) {
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

// drawSignature menggambar tanda tangan asli penerima slot di tengah sel,
// proporsional, atau paraf buatan bila belum ada.
func drawSignature(pdf *fpdf.Fpdf, x, y, width, height float64, slotID string) {
	if slotID == "" {
		return
	}
	name := signatureImageKey(slotID)
	info := pdf.GetImageInfo(name)
	if info == nil || info.Width() <= 0 || info.Height() <= 0 {
		drawRecipientSignature(pdf, x, y, width, height, slotID)
		return
	}
	boxW, boxH := width-4, height-1.5
	ratio := info.Width() / info.Height()
	drawW, drawH := boxW, boxW/ratio
	if drawH > boxH {
		drawH, drawW = boxH, boxH*ratio
	}
	pdf.ImageOptions(name, x+(width-drawW)/2, y+(height-drawH)/2, drawW, drawH, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
}
