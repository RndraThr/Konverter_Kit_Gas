package bast

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/textnorm"

	"github.com/go-pdf/fpdf"
)

// ActivityParticipant adalah satu baris daftar hadir Training yang terisi dari
// penerima distribusi (urutan nomor bagi DP3) pada satu tanggal BA perorangan.
type ActivityParticipant struct {
	Name       string `json:"name"`
	Occupation string `json:"occupation"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	// SignatureKey = ID slot distribusi (satu penerimaan), sama dengan BA
	// perorangan, sehingga dokumen penerimaan tersebut memakai tanda tangan sama.
	SignatureKey string `json:"signature_key"`
}

// participantMode menentukan peserta daftar hadir kegiatan.
type participantMode int

const (
	participantsNone       participantMode = iota // lembar kosong (RAKORDA, Sosialisasi)
	participantsAll                               // Training 100%: semua penerima tanggal tsb.
	participantsTenPercent                        // Training 10%: 10% pertama dari daftar 100%.
)

// tenPercentCount membulatkan ke atas agar tanggal dengan sedikit penerima
// tetap punya peserta (15 penerima -> 2 peserta).
func tenPercentCount(total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / 10))
}

func selectParticipants(mode participantMode, all []ActivityParticipant) []ActivityParticipant {
	if mode == participantsTenPercent {
		return append([]ActivityParticipant(nil), all[:tenPercentCount(len(all))]...)
	}
	return all
}

// TrainingDate adalah satu tanggal distribusi beserta jumlah peserta Training.
type TrainingDate struct {
	LocalDate        string `json:"local_date"`
	RecipientCount   int    `json:"recipient_count"`
	ParticipantCount int    `json:"participant_count"`
}

// ListTrainingParticipants mengembalikan penerima yang distribusinya selesai
// pada tanggal lokal tersebut, berurutan seperti DP3 (nomor bagi).
func (r *Repository) ListTrainingParticipants(ctx context.Context, scheduleID, localDate string, scope auth.RegencyScope) ([]ActivityParticipant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ds.id::text, p.full_name, COALESCE(p.phone_number,''), COALESCE(p.address,''), COALESCE(p.village,''), COALESCE(p.district,''), pr.program_type
		FROM distribution_slots ds
		JOIN program_schedules ps ON ps.id = ds.schedule_id
		JOIN programs pr ON pr.id = ps.program_id
		JOIN people p ON p.id = ds.recipient_person_id
		WHERE ds.schedule_id=$1 AND ds.status='completed' AND ds.distributed_at IS NOT NULL
		  AND (ds.distributed_at AT TIME ZONE 'Asia/Jakarta')::date = $2::date
		  AND ($3 OR ps.regency_id::text = ANY($4))
		ORDER BY ds.slot_number, ds.id
	`, scheduleID, localDate, scope.Unrestricted, scope.RegencyIDs)
	if err != nil {
		return nil, fmt.Errorf("list training participants: %w", err)
	}
	defer rows.Close()
	participants := []ActivityParticipant{}
	for rows.Next() {
		var slotID, name, phone, address, village, district, programType string
		if err := rows.Scan(&slotID, &name, &phone, &address, &village, &district, &programType); err != nil {
			return nil, err
		}
		occupation := "Petani"
		if programType == "fisherman" {
			occupation = "Nelayan"
		}
		parts := []string{strings.TrimSpace(address)}
		if v := strings.TrimSpace(village); v != "" {
			parts = append(parts, "Desa "+v)
		}
		if d := strings.TrimSpace(district); d != "" {
			parts = append(parts, "Kec. "+d)
		}
		participants = append(participants, ActivityParticipant{Name: strings.TrimSpace(name), Occupation: occupation, Phone: strings.TrimSpace(phone), Address: strings.Trim(strings.Join(parts, ", "), ", "), SignatureKey: slotID})
	}
	return participants, rows.Err()
}

// renderActivityParticipantRow menulis satu peserta: nomor, nama, pekerjaan,
// telepon, alamat, dan tanda tangan. Teks dibungkus utuh (tanpa dipotong) dan
// tinggi baris mengikuti sel dengan baris terbanyak.
func renderActivityParticipantRow(pdf *fpdf.Fpdf, number int, participant ActivityParticipant, lines [][]string, height float64) {
	x, y := marginMM, pdf.GetY()
	for _, width := range activityColumnWidths {
		pdf.Rect(x, y, width, height, "")
		x += width
	}
	pdf.SetFont(activityFont, "B", activityTableHeadPt)
	numberLeft := marginMM + (activityColumnWidths[0]-pdf.GetStringWidth("No."))/2
	pdf.SetFont(activityFont, "", activityTableBodyPt)
	pdf.Text(numberLeft, textBaseline(y+0.4, 4.6, activityTableBodyPt), fmt.Sprintf("%d", number))

	pdf.SetFont(activityFont, "", participantTextPt)
	x = marginMM + activityColumnWidths[0]
	// Teks mulai dari kiri atas sel, baris pertama sejajar nomor urut; kolom
	// Pekerjaan dan No. Telepon rata tengah atas.
	firstBaseline := textBaseline(y+0.4, 4.6, activityTableBodyPt)
	for i, cell := range lines {
		width := activityColumnWidths[i+1]
		for j, line := range cell {
			left := x + participantPadMM
			if i == participantOccupationColumn || i == participantPhoneColumn {
				left = x + (width-pdf.GetStringWidth(line))/2
			}
			pdf.Text(left, firstBaseline+float64(j)*participantLineMM, line)
		}
		x += width
	}
	signatureWidth := activityColumnWidths[len(activityColumnWidths)-1]
	drawRecipientSignature(pdf, x, y+(height-activityRowMM)/2, signatureWidth, activityRowMM, participant.SignatureKey)
	pdf.SetXY(marginMM, y+height)
}

const (
	// Indeks sel teks peserta yang rata tengah: Pekerjaan dan No. Telepon.
	participantOccupationColumn = 1
	participantPhoneColumn      = 2

	// Isi tabel sama dengan ukuran nomor urut (10 pt).
	participantTextPt = activityTableBodyPt
	participantLineMM = 4.2
	participantPadMM  = 1.0
)

// layoutParticipantRow membungkus Nama, Pekerjaan, Telepon, dan Alamat sesuai
// lebar kolom dan menghitung tinggi baris.
func layoutParticipantRow(pdf *fpdf.Fpdf, participant ActivityParticipant) ([][]string, float64) {
	pdf.SetFont(activityFont, "", participantTextPt)
	cells := []string{textnorm.DisplayTitle(participant.Name), participant.Occupation, participant.Phone, textnorm.DisplayTitle(participant.Address)}
	lines := make([][]string, len(cells))
	maxLines := 1
	for i, text := range cells {
		text = strings.Join(strings.Fields(text), " ")
		if text != "" {
			lines[i] = wrapWords(pdf, text, activityColumnWidths[i+1]-2*participantPadMM)
		}
		if len(lines[i]) > maxLines {
			maxLines = len(lines[i])
		}
	}
	return lines, math.Max(activityRowMM, float64(maxLines)*participantLineMM+2*participantPadMM)
}

// drawRecipientSignature menggambar tanda tangan penerima pada kotak tanda
// tangan. Sementara berupa coretan dummy yang stabil per slot distribusi (sama
// di BA perorangan dan Training); nanti diganti tanda tangan digital penerimaan.
func drawRecipientSignature(pdf *fpdf.Fpdf, x, y, width, height float64, seed string) {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(seed))
	random := rand.New(rand.NewSource(int64(hash.Sum64())))
	// Kotak tanda tangan berproporsi tetap (2,6 : 1) di tengah sel agar bentuk
	// tanda tangan orang yang sama identik di kotak besar maupun kecil.
	boxH := math.Min(height-1.5, (width-4.4)/2.6)
	boxW := boxH * 2.6
	left, right := x+(width-boxW)/2, x+(width+boxW)/2
	mid := y + height/2
	amp := boxH * 0.28
	pdf.SetDrawColor(30, 45, 110)
	pdf.SetLineWidth(0.25)
	px, py := left, mid+(random.Float64()-0.5)*amp
	strokes := 3 + random.Intn(2)
	step := (right - left) / float64(strokes)
	for i := 0; i < strokes; i++ {
		nx := px + step
		ny := mid + (random.Float64()-0.5)*amp*1.6
		cx := px + step*0.5
		cy := mid + (random.Float64()-0.5)*amp*3.2
		pdf.Curve(px, py, cx, cy, nx, ny, "D")
		px, py = nx, ny
	}
	// Garis bawah pendek seperti paraf.
	underline := mid + amp*0.9
	pdf.Line(left+step*0.3, underline, left+step*(0.3+1.6*random.Float64()+0.6), underline-random.Float64()*0.8)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
}
