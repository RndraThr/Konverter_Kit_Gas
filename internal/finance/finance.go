// Package finance runs pengajuan keuangan: a team leader asks for cashless (a
// direct payment to the payee) or kasbon (an advance that must be reported
// afterwards), and the finance staff assigned to the team review and pay it.
package finance

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	TypeCashless = "cashless"
	TypeKasbon   = "kasbon"

	StatusDraft               = "draft"
	StatusSubmitted           = "submitted"
	StatusRevision            = "revision"
	StatusRejected            = "rejected"
	StatusCancelled           = "cancelled"
	StatusApproved            = "approved"
	StatusPaid                = "paid"
	StatusSettlementSubmitted = "settlement_submitted"
	StatusSettlementRevision  = "settlement_revision"
	StatusDone                = "done"

	ActionSubmit         = "submit"
	ActionCancel         = "cancel"
	ActionApprove        = "approve"
	ActionReject         = "reject"
	ActionRevision       = "revision"
	ActionPay            = "pay"
	ActionSettle         = "settle"
	ActionSettleApprove  = "settle_approve"
	ActionSettleRevision = "settle_revision"

	KindEvidence   = "evidence"
	KindPayment    = "payment"
	KindSettlement = "settlement"
	// Proofs of one row, printed behind the voucher.
	KindItemTransaction = "item_transaction"
	KindItemPayment     = "item_payment"

	EntryDebit  = "debit"
	EntryKredit = "kredit"

	// MaxAttachmentBytes caps one nota or payment proof.
	MaxAttachmentBytes = 15 << 20

	maxItems          = 100
	maxAttachments    = 10
	maxRowAttachments = 300
	maxLineAmount     = int64(10_000_000_000_000)
	maxShortText      = 200
	maxLongText       = 2000
	minSettlementDays = 3
	minAccountDigits  = 5
	maxAccountDigits  = 34
	dateLayout        = "2006-01-02"
	numberPrefix      = "PGJ"
	financeLinkPrefix = "/keuangan/"
)

// Categories the form offers.
var Categories = []string{"Transportasi", "Akomodasi", "Konsumsi", "Logistik", "Material", "Upah", "Lainnya"}

var jakarta = time.FixedZone("Asia/Jakarta", 7*60*60)

var (
	ErrNotFound       = errors.New("finance request not found")
	ErrForbidden      = errors.New("not allowed on this finance request")
	ErrInvalidInput   = errors.New("finance request input is invalid")
	ErrIncomplete     = errors.New("finance request is incomplete")
	ErrInvalidStatus  = errors.New("finance request status does not allow this")
	ErrNotLeader      = errors.New("user does not lead this team")
	ErrNoHandler      = errors.New("team has no finance assigned")
	ErrSettlementSoon = errors.New("kasbon report date is too soon")
	ErrNoteRequired   = errors.New("a note is required")
	ErrProofRequired  = errors.New("payment proof is required")
	ErrSelfReview     = errors.New("finance cannot process their own request")
	ErrTeamInUse      = errors.New("team has finance requests")
	ErrTeamNameInUse  = errors.New("team name is in use")
	ErrInvalidMember  = errors.New("team member lacks the needed access")
	ErrAttachmentType = errors.New("attachment must be a JPG, PNG or PDF")
	ErrAttachmentSize = errors.New("attachment is too large")
	ErrTooManyFiles   = errors.New("too many attachments")
	errMissingStorage = errors.New("finance storage is not configured")
)

// Access is what the account's permissions allow.
type Access struct {
	CanRequest bool
	CanReview  bool
	CanMonitor bool
	CanManage  bool
}

type Item struct {
	// Key stays with the row across saves, so its proofs stay attached.
	Key         string `json:"key"`
	Entry       string `json:"entry"`
	Note        string `json:"note"`
	ProjectCode string `json:"project_code"`
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Amount      int64  `json:"amount"`
}

type Attachment struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	ItemKey     string    `json:"item_key"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

type Event struct {
	Action     string    `json:"action"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Note       string    `json:"note"`
	ActorName  string    `json:"actor_name"`
	CreatedAt  time.Time `json:"created_at"`
}

// Request is one pengajuan. Items, Attachments, Events and Actions are filled
// on the detail only.
type Request struct {
	ID               string       `json:"id"`
	Number           string       `json:"number"`
	Type             string       `json:"type"`
	Status           string       `json:"status"`
	TeamID           string       `json:"team_id"`
	TeamName         string       `json:"team_name"`
	RequesterID      string       `json:"requester_id"`
	RequesterName    string       `json:"requester_name"`
	RegencyID        string       `json:"regency_id"`
	RegencyName      string       `json:"regency_name"`
	RegencyCode      string       `json:"regency_code"`
	NeededDate       string       `json:"needed_date"`
	Category         string       `json:"category"`
	Description      string       `json:"description"`
	TotalAmount      int64        `json:"total_amount"`
	PayeeName        string       `json:"payee_name"`
	BankName         string       `json:"bank_name"`
	AccountNumber    string       `json:"account_number"`
	AccountName      string       `json:"account_name"`
	InvoiceNumber    string       `json:"invoice_number"`
	SettlementDue    string       `json:"settlement_due"`
	SettlementAmount *int64       `json:"settlement_amount"`
	SettlementNote   string       `json:"settlement_note"`
	ReviewNote       string       `json:"review_note"`
	ReviewedByName   string       `json:"reviewed_by_name"`
	PaidByName       string       `json:"paid_by_name"`
	SubmittedAt      *time.Time   `json:"submitted_at"`
	ReviewedAt       *time.Time   `json:"reviewed_at"`
	PaidAt           *time.Time   `json:"paid_at"`
	DoneAt           *time.Time   `json:"done_at"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Items            []Item       `json:"items,omitempty"`
	SettlementItems  []Item       `json:"settlement_items,omitempty"`
	Attachments      []Attachment `json:"attachments,omitempty"`
	Events           []Event      `json:"events,omitempty"`
	Actions          []string     `json:"actions,omitempty"`
	// Editable and UploadKinds tell the client what this viewer may change now.
	Editable    bool     `json:"editable,omitempty"`
	UploadKinds []string `json:"upload_kinds,omitempty"`
	// Vouchers are the vouchers this request went into. CanIssueVoucher says
	// this viewer may put it on one.
	Vouchers        []VoucherRef `json:"vouchers,omitempty"`
	CanIssueVoucher bool         `json:"can_issue_voucher,omitempty"`
}

// DraftInput is the form; a draft may be incomplete.
type DraftInput struct {
	Type          string `json:"type"`
	TeamID        string `json:"team_id"`
	RegencyID     string `json:"regency_id"`
	NeededDate    string `json:"needed_date"`
	Category      string `json:"category"`
	Description   string `json:"description"`
	PayeeName     string `json:"payee_name"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	InvoiceNumber string `json:"invoice_number"`
	SettlementDue string `json:"settlement_due"`
	Items         []Item `json:"items"`
}

type SettlementInput struct {
	Note  string `json:"note"`
	Items []Item `json:"items"`
}

// normalize trims the form and rejects what no draft may hold: an unknown
// type, a malformed date, text over the limit, or amounts that do not fit.
func (input *DraftInput) normalize() error {
	if input.Type != TypeCashless && input.Type != TypeKasbon {
		return ErrInvalidInput
	}
	short := []*string{&input.TeamID, &input.RegencyID, &input.Category, &input.PayeeName, &input.BankName, &input.AccountNumber, &input.AccountName, &input.InvoiceNumber}
	for _, field := range short {
		*field = strings.TrimSpace(*field)
		if len(*field) > maxShortText {
			return ErrInvalidInput
		}
	}
	input.Description = strings.TrimSpace(input.Description)
	if len(input.Description) > maxLongText {
		return ErrInvalidInput
	}
	input.AccountNumber = strings.NewReplacer(" ", "", "-", "").Replace(input.AccountNumber)
	if !validDate(input.NeededDate) || !validDate(input.SettlementDue) {
		return ErrInvalidInput
	}
	var err error
	input.Items, _, err = normalizeItems(input.Items)
	return err
}

// normalizeItems recomputes each line (the client's sums are not trusted),
// keeping half-typed lines of a draft.
func normalizeItems(items []Item) ([]Item, int64, error) {
	if len(items) > maxItems {
		return nil, 0, ErrInvalidInput
	}
	result := make([]Item, 0, len(items))
	var total int64
	for _, item := range items {
		item.Description = strings.TrimSpace(item.Description)
		item.Note, item.ProjectCode = strings.TrimSpace(item.Note), strings.TrimSpace(item.ProjectCode)
		if len(item.Description) > maxShortText || len(item.Note) > maxShortText || len(item.ProjectCode) > maxShortText || item.Quantity < 0 || item.UnitPrice < 0 {
			return nil, 0, ErrInvalidInput
		}
		switch item.Entry {
		case "":
			item.Entry = EntryDebit
		case EntryDebit, EntryKredit:
		default:
			return nil, 0, ErrInvalidInput
		}
		if item.Key == "" {
			item.Key = uuid.NewString()
		} else if _, err := uuid.Parse(item.Key); err != nil {
			return nil, 0, ErrInvalidInput
		}
		if item.Quantity > 0 && item.UnitPrice > maxLineAmount/int64(item.Quantity) {
			return nil, 0, ErrInvalidInput
		}
		item.Amount = int64(item.Quantity) * item.UnitPrice
		total += signedAmount(item)
		result = append(result, item)
	}
	// A half-typed draft may be negative for a moment; Submit checks the total.
	return result, max(total, 0), nil
}

// signedAmount: a kredit row (such as PPh withheld) lowers the total.
func signedAmount(item Item) int64 {
	if item.Entry == EntryKredit {
		return -item.Amount
	}
	return item.Amount
}

func validDate(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse(dateLayout, value)
	return err == nil
}

// completeItems: every line must be finished before it is sent.
func completeItems(items []Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Description == "" || item.Amount <= 0 {
			return false
		}
	}
	return true
}

func validCategory(value string) bool {
	for _, category := range Categories {
		if category == value {
			return true
		}
	}
	return false
}

func validAccountNumber(value string) bool {
	if len(value) < minAccountDigits || len(value) > maxAccountDigits {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// earliestSettlement is the first report date a kasbon may have: H+3 of the
// day the money is needed.
func earliestSettlement(needed string) (string, error) {
	date, err := time.Parse(dateLayout, needed)
	if err != nil {
		return "", ErrIncomplete
	}
	return date.AddDate(0, 0, minSettlementDays).Format(dateLayout), nil
}

// checkComplete is what Submit asks of a request, on top of normalize.
func checkComplete(r Request) error {
	if r.TeamID == "" || r.NeededDate == "" || !validCategory(r.Category) || r.Description == "" || !completeItems(r.Items) {
		return ErrIncomplete
	}
	if _, total, err := normalizeItems(r.Items); err != nil || total <= 0 {
		return ErrIncomplete
	}
	switch r.Type {
	case TypeCashless:
		if r.PayeeName == "" || r.BankName == "" || r.AccountName == "" || !validAccountNumber(r.AccountNumber) {
			return ErrIncomplete
		}
	case TypeKasbon:
		if r.SettlementDue == "" {
			return ErrIncomplete
		}
		earliest, err := earliestSettlement(r.NeededDate)
		if err != nil {
			return err
		}
		if r.SettlementDue < earliest {
			return ErrSettlementSoon
		}
	}
	return nil
}

// roleOf says who may run an action; requester actions belong to the leader,
// the rest to the team's finance.
func requesterAction(action string) bool {
	switch action {
	case ActionSubmit, ActionCancel, ActionSettle:
		return true
	}
	return false
}

// nextStatus is the whole workflow: the status an action leads to from a
// status, or false when it does not apply.
func nextStatus(requestType, status, action string) (string, bool) {
	switch action {
	case ActionSubmit:
		return StatusSubmitted, status == StatusDraft || status == StatusRevision
	case ActionCancel:
		return StatusCancelled, status == StatusSubmitted || status == StatusRevision
	case ActionApprove:
		return StatusApproved, status == StatusSubmitted
	case ActionReject:
		return StatusRejected, status == StatusSubmitted
	case ActionRevision:
		return StatusRevision, status == StatusSubmitted
	case ActionPay:
		if requestType == TypeKasbon {
			return StatusPaid, status == StatusApproved
		}
		return StatusDone, status == StatusApproved
	case ActionSettle:
		return StatusSettlementSubmitted, requestType == TypeKasbon && (status == StatusPaid || status == StatusSettlementRevision)
	case ActionSettleApprove:
		return StatusDone, status == StatusSettlementSubmitted
	case ActionSettleRevision:
		return StatusSettlementRevision, status == StatusSettlementSubmitted
	}
	return "", false
}

func noteRequired(action string) bool {
	return action == ActionReject || action == ActionRevision || action == ActionSettleRevision
}

// viewerActions lists what the viewer can do now, so clients show only the
// buttons the server would accept.
func viewerActions(r Request, requester, handler bool) []string {
	actions := []string{}
	for _, action := range []string{ActionSubmit, ActionCancel, ActionApprove, ActionReject, ActionRevision, ActionPay, ActionSettle, ActionSettleApprove, ActionSettleRevision} {
		if _, ok := nextStatus(r.Type, r.Status, action); !ok {
			continue
		}
		if requesterAction(action) && requester || !requesterAction(action) && handler && !requester {
			actions = append(actions, action)
		}
	}
	return actions
}

func editable(status string) bool {
	return status == StatusDraft || status == StatusRevision
}

func settlementEditable(status string) bool {
	return status == StatusPaid || status == StatusSettlementRevision
}

// uploadKinds: which attachments the viewer may add or remove now.
func uploadKinds(r Request, requester, handler bool) []string {
	kinds := []string{}
	if requester && editable(r.Status) {
		kinds = append(kinds, KindEvidence, KindItemTransaction, KindItemPayment)
	}
	if requester && r.Type == TypeKasbon && settlementEditable(r.Status) {
		kinds = append(kinds, KindSettlement, KindItemTransaction, KindItemPayment)
	}
	if handler && !requester && r.Status == StatusApproved {
		kinds = append(kinds, KindPayment)
	}
	return kinds
}

func typeLabel(value string) string {
	if value == TypeKasbon {
		return "Kasbon"
	}
	return "Cashless"
}

// statusLabels name each status in reports.
var statusLabels = map[string]string{
	StatusDraft: "Draf", StatusSubmitted: "Menunggu review", StatusRevision: "Perlu revisi",
	StatusRejected: "Ditolak", StatusCancelled: "Dibatalkan", StatusApproved: "Disetujui",
	StatusPaid: "Menunggu laporan", StatusSettlementSubmitted: "Laporan direview",
	StatusSettlementRevision: "Laporan perlu revisi", StatusDone: "Selesai",
}
