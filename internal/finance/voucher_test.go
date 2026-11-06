package finance

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
)

func sampleJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 900))
	for x := range 600 {
		for y := range 900 {
			img.Set(x, y, color.RGBA{uint8(x / 3), uint8(y / 4), 180, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func samplePDF(t *testing.T, pages int) []byte {
	t.Helper()
	pdf := fpdf.New("P", "mm", "A5", "")
	pdf.SetFont("Helvetica", "", 14)
	for range pages {
		pdf.AddPage()
		pdf.Cell(40, 10, "Invoice")
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestComposeLinesBalanceTheKasbon(t *testing.T) {
	paid := time.Date(2026, 9, 30, 3, 0, 0, 0, time.UTC)
	r := Request{Type: TypeKasbon, RequesterName: "Salisah", RegencyCode: "WJO", TotalAmount: 75000, PaidAt: &paid, SettlementItems: []Item{
		{Key: "a", Description: "Sewa Mobil", Amount: 2250, Entry: EntryDebit},
		{Key: "b", Description: "PPh Sewa Mobil", Amount: 45, Entry: EntryKredit},
		{Key: "c", Description: "Biaya Pajak", Amount: 45, Entry: EntryDebit, ProjectCode: "OPS Kantor"},
	}}
	lines := composeLines([]Request{r})
	if !lines[0].IsBank || lines[0].Debit != 72750 {
		t.Fatalf("bank row=%+v", lines[0])
	}
	if lines[1].Description != "EX UMK a.n Salisah (Tgl 30/09/2026)" || lines[1].Kredit != 75000 {
		t.Fatalf("advance row=%+v", lines[1])
	}
	if lines[2].ProjectCode != "KONKIT-WJO" || lines[4].ProjectCode != "OPS Kantor" {
		t.Fatalf("project codes %q %q", lines[2].ProjectCode, lines[4].ProjectCode)
	}
	if debit, kredit := voucherTotals(lines); debit != kredit || debit != 75045 {
		t.Fatalf("debit=%d kredit=%d", debit, kredit)
	}
}

func TestComposeLinesJoinRequestsOnOneBankRow(t *testing.T) {
	short := Request{Type: TypeKasbon, RequesterName: "A", TotalAmount: 100, SettlementItems: []Item{{Description: "Solar", Amount: 130, Entry: EntryDebit}}}
	cashless := Request{Type: TypeCashless, Items: []Item{{Description: "Sewa", Amount: 500, Entry: EntryDebit}, {Description: "PPh", Amount: 10, Entry: EntryKredit}}}
	lines := composeLines([]Request{short, cashless})
	if len(lines) != 5 || lines[0].Kredit != 520 {
		t.Fatalf("lines=%+v", lines)
	}
	if debit, kredit := voucherTotals(lines); debit != kredit {
		t.Fatalf("debit=%d kredit=%d", debit, kredit)
	}
}

func TestPaymentProofGoesOnTheRowItPaid(t *testing.T) {
	kasbon := Request{ID: "k", Type: TypeKasbon, RequesterName: "Salisah", SettlementItems: []Item{{Key: "a"}}}
	cashless := Request{ID: "c", Type: TypeCashless, Items: []Item{{Key: "b"}}}
	gone := Request{ID: "g", Type: TypeCashless, Items: []Item{{Key: "x"}}}
	lines := []VoucherLine{{IsBank: true}, {Description: "EX UMK a.n Salisah (Tgl 30/09/2026)"}, {ItemKey: "a"}, {ItemKey: "b"}}
	rows := paymentRows([]Request{kasbon, cashless, gone}, lines)
	if rows["k"] != 1 || rows["c"] != 3 {
		t.Fatalf("rows=%v", rows)
	}
	if _, found := rows["g"]; found {
		t.Fatalf("removed request has a row: %v", rows)
	}
}

func TestVoucherInputRejectsWhatCannotPrint(t *testing.T) {
	account := Account{Kind: AccountBank, Name: "BSI Operasional", AccountNumber: "7185595751"}
	valid := func() VoucherInput {
		return VoucherInput{Number: "BSK261007", Date: "2026-10-30", PayTo: "KSO Sucofindo", Lines: []VoucherLine{{IsBank: true, Kredit: 100}, {Description: "Jasa", Debit: 100}}}
	}
	input := valid()
	if err := input.normalize(account); err != nil || input.Lines[0].Description != "Bank BSI Operasional - 7185595751" {
		t.Fatalf("err=%v line=%+v", err, input.Lines[0])
	}
	for name, change := range map[string]func(*VoucherInput){
		"number":   func(v *VoucherInput) { v.Number = "BSK 26" },
		"pay to":   func(v *VoucherInput) { v.PayTo = " " },
		"date":     func(v *VoucherInput) { v.Date = "30-10-2026" },
		"two side": func(v *VoucherInput) { v.Lines[1].Kredit = 5 },
		"empty":    func(v *VoucherInput) { v.Lines[1].Description = "" },
		"negative": func(v *VoucherInput) { v.Lines[1].Debit = -100 },
	} {
		broken := valid()
		change(&broken)
		if err := broken.normalize(account); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	unbalanced := valid()
	unbalanced.Lines[1].Debit = 90
	if err := unbalanced.normalize(account); !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("unbalanced: %v", err)
	}
}

func TestVoucherPagesKeepTheFooterWithRows(t *testing.T) {
	for _, count := range []int{1, 5, 10, 11, 17, 18, 37, 39, 60} {
		pages := voucherPages(count, voucherFirstRows)
		next := 0
		for index, page := range pages {
			if page[0] != next {
				t.Fatalf("count %d: gap in %v", count, pages)
			}
			room := voucherLaterRows
			if index == 0 {
				room = voucherFirstRows
			}
			if page[1]-page[0] > room {
				t.Fatalf("count %d: page too full %v", count, pages)
			}
			next = page[1]
		}
		last := pages[len(pages)-1]
		if next != count || last[1]-last[0] < 1 {
			t.Fatalf("count %d: %v", count, pages)
		}
	}
	if pages := voucherPages(37, voucherFirstRows); len(pages) != 3 || pages[0] != [2]int{0, 12} {
		t.Fatalf("37 rows: %v", pages)
	}
}

func TestVoucherMoneyAndProjectCode(t *testing.T) {
	if voucherMoney(75045000) != "75.045.000" || voucherMoney(0) != "" {
		t.Fatal(voucherMoney(75045000))
	}
	if projectCode("BKA") != "KONKIT-BKA" || projectCode("") != "" {
		t.Fatal(projectCode("BKA"))
	}
}

func TestRenderBundleJoinsVoucherAndProofs(t *testing.T) {
	lines := []VoucherLine{{Description: "Bank BSI Operasional - 7185595751", Debit: 30440324}, {Description: "EX UMK a.n Salisah", Kredit: 75000000, ProjectCode: "EX UMK"}}
	for index := range 35 {
		lines = append(lines, VoucherLine{ItemKey: "k", Description: "Sewa Mobil MP Swakelola a.n Sigit Arianto Jargas ESDM Kab.Wajo, Salisah dan uraian yang terlalu panjang sekali untuk satu baris", Debit: int64(1000 * (index + 1)), Note: "Sewa Mobil MP Swakelola", ProjectCode: "UMK Mandor-Wajo"})
	}
	doc := voucherDocument{Number: "BSK261007", Date: "2026-10-30", PayTo: "KSO Sucofindo - Surveyor Indonesia", Description: "Pembayaran Jasa Laporan Surveyor", Logo: defaultVoucherLogo,
		AccountID: "b2", Lines: lines, Settings: VoucherSettings{CompanyName: "PT KIAN SANTANG MULIATAMA TBK", AddressLine1: "Jalan Wibawa Mukti II, Perum Telkom Satwika Permai", AddressLine2: "Blok A5, No. 8", AddressLine3: "Bekasi 17425 - INDONESIA",
			Accounts: []Account{{ID: "b1", Kind: AccountBank, Name: "BRI", AccountNumber: "210801000370308", IsActive: true}, {ID: "b2", Kind: AccountBank, Name: "BSI Operasional", AccountNumber: "7185595751", IsActive: true}, {ID: "k1", Kind: AccountKas, Name: "GoPay", IsActive: true}, {ID: "l1", Kind: AccountLainnya, Name: "Hutang Reimburse KSO", IsActive: true}}}}
	proofs := []bundleProof{
		{Title: "Row 3 - Bukti Transaksi", Requester: "Salisah", Filename: "WhatsApp Image.jpeg", ContentType: "image/jpeg", Data: sampleJPEG(t)},
		{Title: "Row 3 - Bukti Pembayaran (1/2)", Requester: "Salisah", Filename: "INV01023.pdf", ContentType: "application/pdf", Data: samplePDF(t, 2)},
	}
	bundle, err := renderBundle(doc, proofs)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pageCount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 6 {
		t.Fatalf("pages=%d", pages)
	}
	if out := os.Getenv("VOUCHER_SAMPLE"); out != "" {
		if err := os.WriteFile(out, bundle, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.HasPrefix(string(bundle), "%PDF") {
		t.Fatal("not a pdf")
	}
}
