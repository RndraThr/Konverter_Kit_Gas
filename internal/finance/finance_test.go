package finance

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"bytes"
	"github.com/xuri/excelize/v2"
)

func TestNextStatusCoversBothWorkflows(t *testing.T) {
	cases := []struct {
		requestType, status, action, want string
		ok                                bool
	}{
		{TypeCashless, StatusDraft, ActionSubmit, StatusSubmitted, true},
		{TypeCashless, StatusRevision, ActionSubmit, StatusSubmitted, true},
		{TypeCashless, StatusSubmitted, ActionApprove, StatusApproved, true},
		{TypeCashless, StatusSubmitted, ActionReject, StatusRejected, true},
		{TypeCashless, StatusSubmitted, ActionRevision, StatusRevision, true},
		{TypeCashless, StatusApproved, ActionPay, StatusDone, true},
		{TypeKasbon, StatusApproved, ActionPay, StatusPaid, true},
		{TypeKasbon, StatusPaid, ActionSettle, StatusSettlementSubmitted, true},
		{TypeKasbon, StatusSettlementRevision, ActionSettle, StatusSettlementSubmitted, true},
		{TypeKasbon, StatusSettlementSubmitted, ActionSettleApprove, StatusDone, true},
		{TypeKasbon, StatusSettlementSubmitted, ActionSettleRevision, StatusSettlementRevision, true},
		{TypeCashless, StatusDraft, ActionApprove, "", false},
		{TypeCashless, StatusSubmitted, ActionPay, "", false},
		{TypeCashless, StatusDone, ActionSettle, "", false},
		{TypeKasbon, StatusRejected, ActionSubmit, "", false},
		{TypeKasbon, StatusApproved, ActionCancel, "", false},
	}
	for _, c := range cases {
		got, ok := nextStatus(c.requestType, c.status, c.action)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s %s %s = %s %v", c.requestType, c.status, c.action, got, ok)
		}
	}
}

func TestViewerActionsKeepRolesApart(t *testing.T) {
	r := Request{Type: TypeCashless, Status: StatusSubmitted}
	if got := viewerActions(r, true, false); !slices.Equal(got, []string{ActionCancel}) {
		t.Fatalf("requester: %v", got)
	}
	if got := viewerActions(r, false, true); !slices.Equal(got, []string{ActionApprove, ActionReject, ActionRevision}) {
		t.Fatalf("handler: %v", got)
	}
	// A leader who is also the team's finance cannot review their own request.
	if got := viewerActions(r, true, true); slices.Contains(got, ActionApprove) {
		t.Fatalf("self review offered: %v", got)
	}
	if got := viewerActions(r, false, false); len(got) != 0 {
		t.Fatalf("monitor: %v", got)
	}
}

func TestUploadKindsFollowTheStage(t *testing.T) {
	if got := uploadKinds(Request{Type: TypeCashless, Status: StatusDraft}, true, false); !slices.Equal(got, []string{KindEvidence, KindItemTransaction, KindItemPayment}) {
		t.Fatalf("draft: %v", got)
	}
	if got := uploadKinds(Request{Type: TypeKasbon, Status: StatusPaid}, true, false); !slices.Equal(got, []string{KindSettlement, KindItemTransaction, KindItemPayment}) {
		t.Fatalf("paid: %v", got)
	}
	if got := uploadKinds(Request{Type: TypeCashless, Status: StatusApproved}, false, true); !slices.Equal(got, []string{KindPayment}) {
		t.Fatalf("approved: %v", got)
	}
	if got := uploadKinds(Request{Type: TypeCashless, Status: StatusApproved}, true, true); len(got) != 0 {
		t.Fatalf("own payment proof: %v", got)
	}
}

func TestNormalizeRecomputesAmountsAndAcceptsPartialDraft(t *testing.T) {
	input := DraftInput{Type: TypeCashless, AccountNumber: " 123-456 78 ", Description: "  bensin ", Items: []Item{
		{Description: " Solar ", Quantity: 2, UnitPrice: 1500, Amount: 999999},
		{Description: "", Quantity: 0, UnitPrice: 0},
	}}
	if err := input.normalize(); err != nil {
		t.Fatal(err)
	}
	if input.Items[0].Amount != 3000 || input.Items[0].Description != "Solar" || input.AccountNumber != "12345678" || input.Description != "bensin" {
		t.Fatalf("input=%+v", input)
	}
	for _, bad := range []DraftInput{
		{Type: "tunai"},
		{Type: TypeKasbon, NeededDate: "05-11-2026"},
		{Type: TypeKasbon, Items: []Item{{Quantity: -1}}},
		{Type: TypeKasbon, Items: []Item{{Quantity: 1000000, UnitPrice: 100_000_000_000}}},
		{Type: TypeKasbon, Description: strings.Repeat("x", maxLongText+1)},
	} {
		if err := bad.normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%+v: %v", bad.Type, err)
		}
	}
}

func TestCheckCompleteByType(t *testing.T) {
	items := []Item{{Description: "Sewa", Quantity: 1, UnitPrice: 1000, Amount: 1000}}
	cashless := Request{Type: TypeCashless, TeamID: "t", NeededDate: "2026-11-10", Category: "Logistik", Description: "Sewa truk", Items: items,
		PayeeName: "CV Maju", BankName: "BCA", AccountName: "CV Maju", AccountNumber: "1234567890"}
	if err := checkComplete(cashless); err != nil {
		t.Fatal(err)
	}
	broken := cashless
	broken.AccountNumber = "12ab"
	if err := checkComplete(broken); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("account: %v", err)
	}
	broken = cashless
	broken.Items = []Item{{Description: "", Amount: 1000}}
	if err := checkComplete(broken); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("items: %v", err)
	}
	kasbon := Request{Type: TypeKasbon, TeamID: "t", NeededDate: "2026-11-10", Category: "Konsumsi", Description: "Uang makan", Items: items, SettlementDue: "2026-11-09"}
	if err := checkComplete(kasbon); !errors.Is(err, ErrSettlementSoon) {
		t.Fatalf("due before need: %v", err)
	}
	kasbon.SettlementDue = "2026-11-12"
	if err := checkComplete(kasbon); !errors.Is(err, ErrSettlementSoon) {
		t.Fatalf("H+2: %v", err)
	}
	kasbon.SettlementDue = "2026-11-13"
	if err := checkComplete(kasbon); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsHideAmountsAndAccounts(t *testing.T) {
	r := Request{ID: "r1", Number: "PGJ-202611-0001", Type: TypeCashless, RequesterName: "Dwi", TotalAmount: 1234567, AccountNumber: "9988776655"}
	for _, action := range []string{ActionSubmit, ActionCancel, ActionSettle, ActionApprove, ActionReject, ActionRevision, ActionPay, ActionSettleApprove, ActionSettleRevision} {
		message, ok := notifyMessage(r, action)
		if !ok || message.Link != "/keuangan/r1" {
			t.Fatalf("%s: %+v", action, message)
		}
		text := message.Title + message.Body
		if strings.Contains(text, "1234567") || strings.Contains(text, "1.234.567") || strings.Contains(text, "9988776655") {
			t.Fatalf("%s leaks: %s", action, text)
		}
	}
}

func TestBuildReportKeepsTextAsText(t *testing.T) {
	spent := int64(800)
	data, err := buildReport([]Request{
		{Number: "PGJ-1", Type: TypeKasbon, Status: StatusDone, Description: "=HYPERLINK(\"http://x\")", AccountNumber: "001234567890123", TotalAmount: 1000, SettlementAmount: &spent},
	}, "Laporan harian")
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if formula, _ := file.GetCellFormula("Pengajuan", "H2"); formula != "" {
		t.Fatalf("formula kept: %s", formula)
	}
	if value, _ := file.GetCellValue("Pengajuan", "N2"); value != "001234567890123" {
		t.Fatalf("account=%s", value)
	}
	if value, _ := file.GetCellValue("Pengajuan", "W2"); strings.ReplaceAll(value, ",", "") != "200" {
		t.Fatalf("difference=%s", value)
	}
	if value, _ := file.GetCellValue("Ringkasan", "A4"); value != "Kasbon - Selesai" {
		t.Fatalf("recap=%s", value)
	}
}
