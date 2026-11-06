package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"konkit/internal/auth"
	"konkit/internal/database"
	"konkit/internal/media"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pngBytes is a real one-pixel image: uploads must decode.
var pngBytes = func() []byte {
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		panic(err)
	}
	return out.Bytes()
}()

type fixture struct {
	pool                          *pgxpool.Pool
	service                       *Service
	leader, handler, other, admin auth.Principal
	teamID                        string
}

func integrationFixture(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	storage, err := media.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{pool: pool, service: NewService(pool, storage, nil)}
	suffix := uuid.NewString()[:8]
	user := func(name string) auth.Principal {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users (username, email, password_hash, full_name) VALUES ($1, $2, 'x', $3) RETURNING id::text`,
			name+suffix, name+suffix+"@test.local", name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return auth.Principal{UserID: id, Username: name}
	}
	f.leader, f.handler, f.other, f.admin = user("leader"), user("finance"), user("finance2"), user("admin")
	if err := pool.QueryRow(ctx, `INSERT INTO finance_teams (name) VALUES ($1) RETURNING id::text`, "Tim "+suffix).Scan(&f.teamID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO finance_team_members (team_id, user_id, member_role) VALUES ($1::uuid, $2::uuid, 'leader'), ($1::uuid, $3::uuid, 'finance')`,
		f.teamID, f.leader.UserID, f.handler.UserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		_, _ = pool.Exec(cleanup, `DELETE FROM finance_requests WHERE team_id = $1::uuid`, f.teamID)
		_, _ = pool.Exec(cleanup, `DELETE FROM finance_teams WHERE id = $1::uuid`, f.teamID)
		_, _ = pool.Exec(cleanup, `DELETE FROM notifications WHERE user_id = ANY($1::uuid[])`, []string{f.leader.UserID, f.handler.UserID, f.other.UserID})
		_, _ = pool.Exec(cleanup, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{f.leader.UserID, f.handler.UserID, f.other.UserID, f.admin.UserID})
	})
	return f
}

var (
	leaderAccess  = Access{CanRequest: true}
	handlerAccess = Access{CanReview: true}
	adminAccess   = Access{CanMonitor: true, CanManage: true}
)

func (f fixture) cashless(needed string) DraftInput {
	return DraftInput{Type: TypeCashless, TeamID: f.teamID, NeededDate: needed, Category: "Logistik", Description: "Sewa truk",
		PayeeName: "CV Maju", BankName: "BCA", AccountNumber: "1234567890", AccountName: "CV Maju",
		Items: []Item{{Description: "Sewa", Quantity: 2, UnitPrice: 500000}}}
}

func (f fixture) kasbon(needed, due string) DraftInput {
	return DraftInput{Type: TypeKasbon, TeamID: f.teamID, NeededDate: needed, SettlementDue: due, Category: "Konsumsi", Description: "Uang makan tim",
		Items: []Item{{Description: "Makan", Quantity: 10, UnitPrice: 25000}}}
}

func (f fixture) notificationCount(t *testing.T, userID, category string) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND category = $2`, userID, category).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestIntegrationCashlessFlow(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	meta := auth.ClientMeta{}
	id := uuid.NewString()

	// Autosave repeats safely with the client's id.
	for range 2 {
		draft, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.cashless("2026-11-10"))
		if err != nil {
			t.Fatal(err)
		}
		if draft.TotalAmount != 1000000 || draft.Status != StatusDraft || !draft.Editable || len(draft.Items) != 1 {
			t.Fatalf("draft=%+v", draft)
		}
	}
	// Nobody else sees a draft, not even monitoring.
	if _, err := f.service.Get(ctx, f.handler, handlerAccess, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("handler sees draft: %v", err)
	}
	if _, err := f.service.Get(ctx, f.admin, adminAccess, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin sees draft: %v", err)
	}
	if _, err := f.service.SaveDraft(ctx, f.handler, leaderAccess, id, f.cashless("2026-11-10")); err == nil {
		t.Fatal("another user saved the draft")
	}

	submitted, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSubmit, "", meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(submitted.Number, "PGJ-") || submitted.Status != StatusSubmitted || submitted.Editable {
		t.Fatalf("submitted=%+v", submitted)
	}
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.cashless("2026-11-11")); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("edit after submit: %v", err)
	}
	if f.notificationCount(t, f.handler.UserID, "finance_review") != 1 {
		t.Fatal("finance was not told")
	}

	// Finance of another team is told it does not exist.
	if _, err := f.service.Act(ctx, f.other, handlerAccess, id, ActionApprove, "", meta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other finance: %v", err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionReject, "", meta); !errors.Is(err, ErrNoteRequired) {
		t.Fatalf("reject without note: %v", err)
	}
	revised, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionRevision, "Rekening salah", meta)
	if err != nil || revised.Status != StatusRevision {
		t.Fatalf("revision=%+v err=%v", revised, err)
	}
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.cashless("2026-11-12")); err != nil {
		t.Fatal(err)
	}
	resubmitted, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSubmit, "", meta)
	if err != nil || resubmitted.Number != submitted.Number {
		t.Fatalf("resubmitted=%+v err=%v", resubmitted, err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionApprove, "", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionPay, "", meta); !errors.Is(err, ErrProofRequired) {
		t.Fatalf("pay without proof: %v", err)
	}
	if _, err := f.service.AddAttachment(ctx, f.handler, handlerAccess, id, KindPayment, "", "bukti.png", bytes.NewReader([]byte("<html>"))); !errors.Is(err, ErrAttachmentType) {
		t.Fatalf("html proof: %v", err)
	}
	if _, err := f.service.AddAttachment(ctx, f.leader, leaderAccess, id, KindPayment, "", "bukti.png", bytes.NewReader(pngBytes)); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("leader proof: %v", err)
	}
	proof, err := f.service.AddAttachment(ctx, f.handler, handlerAccess, id, KindPayment, "", "bukti.png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	paid, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionPay, "", meta)
	if err != nil || paid.Status != StatusDone || paid.PaidAt == nil || paid.DoneAt == nil {
		t.Fatalf("paid=%+v err=%v", paid, err)
	}
	if f.notificationCount(t, f.leader.UserID, "finance_payment") != 1 {
		t.Fatal("leader was not told about the payment")
	}
	if len(paid.Events) != 5 {
		t.Fatalf("events=%d", len(paid.Events))
	}
	content, err := f.service.OpenAttachment(ctx, f.admin, adminAccess, id, proof.ID)
	if err != nil {
		t.Fatal(err)
	}
	content.Reader.Close()
	if _, err := f.service.OpenAttachment(ctx, f.other, handlerAccess, id, proof.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other finance opened proof: %v", err)
	}
}

func TestIntegrationKasbonSettlementAndReminder(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	meta := auth.ClientMeta{}
	id := uuid.NewString()
	yesterday := time.Now().In(jakarta).AddDate(0, 0, -1).Format(dateLayout)
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.kasbon(time.Now().In(jakarta).AddDate(0, 0, -5).Format(dateLayout), yesterday)); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		actor  auth.Principal
		access Access
		action string
	}{{f.leader, leaderAccess, ActionSubmit}, {f.handler, handlerAccess, ActionApprove}} {
		if _, err := f.service.Act(ctx, step.actor, step.access, id, step.action, "", meta); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.AddAttachment(ctx, f.handler, handlerAccess, id, KindPayment, "", "bukti.png", bytes.NewReader(pngBytes)); err != nil {
		t.Fatal(err)
	}
	paid, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionPay, "", meta)
	if err != nil || paid.Status != StatusPaid {
		t.Fatalf("paid=%+v err=%v", paid, err)
	}

	// The overdue reminder goes out once a day.
	for range 2 {
		if err := f.service.remind(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.notificationCount(t, f.leader.UserID, "finance_reminder"); got != 1 {
		t.Fatalf("reminders=%d", got)
	}

	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSettle, "", meta); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("settle empty: %v", err)
	}
	saved, err := f.service.SaveSettlement(ctx, f.leader, leaderAccess, id, SettlementInput{Note: "Sisa dikembalikan", Items: []Item{{Description: "Makan", Quantity: 8, UnitPrice: 25000}}})
	if err != nil || saved.SettlementAmount == nil || *saved.SettlementAmount != 200000 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	if _, err := f.service.AddAttachment(ctx, f.leader, leaderAccess, id, KindSettlement, "", "nota.png", bytes.NewReader(pngBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSettle, "", meta); err != nil {
		t.Fatal(err)
	}
	done, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionSettleApprove, "", meta)
	if err != nil || done.Status != StatusDone || done.DoneAt == nil {
		t.Fatalf("done=%+v err=%v", done, err)
	}
}

func TestIntegrationScopesAndReports(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	meta := auth.ClientMeta{}
	id := uuid.NewString()
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.cashless("2026-11-10")); err != nil {
		t.Fatal(err)
	}
	queue := Filter{Scope: ScopeQueue}
	if page, err := f.service.List(ctx, f.handler, handlerAccess, queue); err != nil || page.Total != 0 {
		t.Fatalf("draft in queue: %+v %v", page.Total, err)
	}
	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSubmit, "", meta); err != nil {
		t.Fatal(err)
	}
	if page, err := f.service.List(ctx, f.handler, handlerAccess, queue); err != nil || page.Total != 1 {
		t.Fatalf("queue: %+v %v", page.Total, err)
	}
	if page, err := f.service.List(ctx, f.other, handlerAccess, queue); err != nil || page.Total != 0 {
		t.Fatalf("other queue: %+v %v", page.Total, err)
	}
	if _, err := f.service.List(ctx, f.leader, leaderAccess, Filter{Scope: ScopeAll}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("leader all: %v", err)
	}
	if _, err := f.service.List(ctx, f.leader, leaderAccess, queue); !errors.Is(err, ErrForbidden) {
		t.Fatalf("leader queue: %v", err)
	}
	summary, err := f.service.Summary(ctx, f.handler, handlerAccess, Filter{Scope: ScopeQueue})
	if err != nil || summary.WaitingReview != 1 || len(summary.Teams) != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	today := time.Now().In(jakarta).Format(dateLayout)
	data, err := f.service.DailyReport(ctx, f.handler, handlerAccess, today, ScopeQueue, "")
	if err != nil || len(data) == 0 {
		t.Fatalf("daily len=%d err=%v", len(data), err)
	}
	if _, err := f.service.ExportReport(ctx, f.handler, handlerAccess, Filter{Scope: ScopeAll}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("finance export all: %v", err)
	}
	if _, err := f.service.ExportReport(ctx, f.admin, adminAccess, Filter{Scope: ScopeAll, From: today, To: today}); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationSubmitNeedsAHandlerAndALeader(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	if _, err := f.service.SaveDraft(ctx, f.other, leaderAccess, uuid.NewString(), f.cashless("2026-11-10")); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("not a leader: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM finance_team_members WHERE team_id = $1::uuid AND member_role = 'finance'`, f.teamID); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.cashless("2026-11-10")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSubmit, "", auth.ClientMeta{}); !errors.Is(err, ErrNoHandler) {
		t.Fatalf("no handler: %v", fmt.Sprint(err))
	}
	if err := f.service.Delete(ctx, f.leader, id); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationVoucherBundle(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	meta := auth.ClientMeta{}
	id := uuid.NewString()
	due := time.Now().In(jakarta).Format(dateLayout)
	if _, err := f.service.SaveDraft(ctx, f.leader, leaderAccess, id, f.kasbon(due, time.Now().In(jakarta).AddDate(0, 0, minSettlementDays).Format(dateLayout))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSubmit, "", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionApprove, "", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AddAttachment(ctx, f.handler, handlerAccess, id, KindPayment, "", "bukti.png", bytes.NewReader(pngBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionPay, "", meta); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	if _, err := f.service.SaveSettlement(ctx, f.leader, leaderAccess, id, SettlementInput{Items: []Item{
		{Key: key, Description: "Makan", Quantity: 8, UnitPrice: 25000}, {Description: "PPh", Quantity: 1, UnitPrice: 1000, Entry: EntryKredit},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AddAttachment(ctx, f.leader, leaderAccess, id, KindItemTransaction, uuid.NewString(), "x.png", bytes.NewReader(pngBytes)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown row: %v", err)
	}
	if _, err := f.service.AddAttachment(ctx, f.leader, leaderAccess, id, KindItemPayment, key, "nota makan.png", bytes.NewReader(pngBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AddAttachment(ctx, f.leader, leaderAccess, id, KindItemPayment, key, "rusak.png", bytes.NewReader([]byte("\x89PNG\r\n\x1a\nrusak"))); !errors.Is(err, ErrAttachmentType) {
		t.Fatalf("broken image: %v", err)
	}
	if _, err := f.service.Act(ctx, f.leader, leaderAccess, id, ActionSettle, "", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ComposeVoucher(ctx, f.handler, handlerAccess, []string{id}); !errors.Is(err, ErrVoucherNotReady) {
		t.Fatalf("early voucher: %v", err)
	}
	if _, err := f.service.Act(ctx, f.handler, handlerAccess, id, ActionSettleApprove, "", meta); err != nil {
		t.Fatal(err)
	}
	draft, err := f.service.ComposeVoucher(ctx, f.handler, handlerAccess, []string{id, id})
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.RequestIDs) != 1 || !draft.Lines[0].IsBank || draft.PayTo == "" {
		t.Fatalf("draft=%+v", draft)
	}
	if _, err := f.service.ComposeVoucher(ctx, f.leader, leaderAccess, []string{id}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("leader composed: %v", err)
	}
	settings, err := f.service.VoucherSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	number := "T" + strings.ToUpper(uuid.NewString()[:8])
	input := VoucherInput{Number: number, Date: due, AccountID: settings.Accounts[0].ID, PayTo: "CV Maju", Lines: draft.Lines, RequestIDs: draft.RequestIDs}
	input.Lines = append(input.Lines, VoucherLine{Description: "Biaya Transfer", Debit: 2500}, VoucherLine{Description: "Biaya bank", Kredit: 2500})
	blank := input
	blank.Number = ""
	preview, err := f.service.PreviewVoucher(ctx, f.handler, handlerAccess, blank)
	if err != nil || !bytes.HasPrefix(preview, []byte("%PDF")) {
		t.Fatalf("preview=%d err=%v", len(preview), err)
	}
	if _, err := f.service.PreviewVoucher(ctx, f.leader, leaderAccess, blank); !errors.Is(err, ErrForbidden) {
		t.Fatalf("leader preview: %v", err)
	}
	var kept int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM finance_voucher_requests WHERE request_id = $1::uuid`, id).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("preview kept %d err=%v", kept, err)
	}
	voucher, err := f.service.SaveVoucher(ctx, f.handler, handlerAccess, "", input, meta)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM finance_vouchers WHERE number LIKE $1`, number+"%")
	})
	if voucher.PageCount != 3 || len(voucher.Lines) != len(input.Lines) || voucher.Lines[0].Description == "" {
		t.Fatalf("voucher=%+v", voucher)
	}
	if _, err := f.service.SaveVoucher(ctx, f.handler, handlerAccess, "", input, meta); !errors.Is(err, ErrVoucherNumber) {
		t.Fatalf("same number: %v", err)
	}
	// A request may go on a second voucher, and shows both.
	input.Number = number + "B"
	if _, err := f.service.SaveVoucher(ctx, f.handler, handlerAccess, "", input, meta); err != nil {
		t.Fatal(err)
	}
	seen, err := f.service.Get(ctx, f.leader, leaderAccess, id)
	if err != nil || len(seen.Vouchers) != 2 {
		t.Fatalf("refs=%+v err=%v", seen.Vouchers, err)
	}
	input.Number, input.PayTo = number, "CV Maju Jaya"
	updated, err := f.service.SaveVoucher(ctx, f.handler, handlerAccess, voucher.ID, input, meta)
	if err != nil || updated.ID != voucher.ID || updated.PayTo != "CV Maju Jaya" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	content, err := f.service.OpenVoucher(ctx, f.leader, leaderAccess, voucher.ID)
	if err != nil {
		t.Fatal(err)
	}
	content.Reader.Close()
	if content.Filename != "Voucher_Bundle_"+number+".pdf" {
		t.Fatalf("filename=%s", content.Filename)
	}
	if _, err := f.service.OpenVoucher(ctx, f.other, handlerAccess, voucher.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other finance opened: %v", err)
	}
	day := time.Now().In(jakarta).Format(dateLayout)
	candidates, err := f.service.VoucherCandidates(ctx, f.handler, handlerAccess, day, day)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range candidates {
		found = found || (candidate.ID == id && len(candidate.Vouchers) == 2)
	}
	if !found {
		t.Fatalf("candidates=%d", len(candidates))
	}
}

func TestIntegrationManualVoucherHasNoRequests(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	settings, err := f.service.VoucherSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	number := "M" + strings.ToUpper(uuid.NewString()[:8])
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM finance_vouchers WHERE number = $1`, number)
	})
	input := VoucherInput{Number: number, Date: "2026-11-06", AccountID: settings.Accounts[0].ID, PayTo: "CV Maju", Lines: []VoucherLine{{IsBank: true, Kredit: 2500}, {Description: "Biaya Transfer", Debit: 2500}}}
	voucher, err := f.service.SaveVoucher(ctx, f.handler, handlerAccess, "", input, auth.ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if voucher.PageCount != 1 || len(voucher.RequestIDs) != 0 {
		t.Fatalf("voucher=%+v", voucher)
	}
	if _, err := f.service.OpenVoucher(ctx, f.other, handlerAccess, voucher.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other finance opened a manual voucher: %v", err)
	}
	content, err := f.service.OpenVoucher(ctx, f.admin, adminAccess, voucher.ID)
	if err != nil {
		t.Fatalf("monitoring: %v", err)
	}
	content.Reader.Close()
}

func TestIntegrationPayees(t *testing.T) {
	f := integrationFixture(t)
	ctx := context.Background()
	saved, err := f.service.SavePayee(ctx, f.leader, leaderAccess, f.teamID, Payee{AccountName: " CV  Maju ", BankName: "bri", AccountNumber: "1234-5678 90"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccountName != "CV Maju" || saved.BankName != "BRI" || saved.AccountNumber != "1234567890" {
		t.Fatalf("saved=%+v", saved)
	}
	// The same account again renames it instead of adding a second row.
	again, err := f.service.SavePayee(ctx, f.leader, leaderAccess, f.teamID, Payee{AccountName: "CV Maju Jaya", BankName: "BRI", AccountNumber: "1234567890"})
	if err != nil || again.ID != saved.ID {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	rows, err := f.service.Payees(ctx, f.leader, leaderAccess, f.teamID)
	if err != nil || len(rows) != 1 || rows[0].AccountName != "CV Maju Jaya" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if _, err := f.service.SavePayee(ctx, f.leader, leaderAccess, f.teamID, Payee{AccountName: "X", BankName: "BRI", AccountNumber: "12"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("short number: %v", err)
	}
	if _, err := f.service.Payees(ctx, f.other, leaderAccess, f.teamID); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("other team: %v", err)
	}
	if err := f.service.DeletePayee(ctx, f.other, leaderAccess, saved.ID); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("other deleted: %v", err)
	}
	if err := f.service.DeletePayee(ctx, f.leader, leaderAccess, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeletePayee(ctx, f.leader, leaderAccess, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
