package finance

import (
	"bytes"
	"context"
	_ "embed" // the default letterhead logo
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxLogoBytes      = 2 << 20
	maxVoucherLines   = 500
	maxVoucherRefRows = 200

	AccountBank    = "bank"
	AccountKas     = "kas"
	AccountLainnya = "lainnya"
)

//go:embed voucher_logo.png
var defaultVoucherLogo []byte

var voucherNumberPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

var (
	ErrVoucherNotReady = errors.New("finance request is not ready for a voucher")
	ErrVoucherSettings = errors.New("voucher settings are incomplete")
	ErrUnbalanced      = errors.New("voucher debit and kredit differ")
	ErrVoucherNumber   = errors.New("voucher number is in use")
)

// Account is a bank, cash box or other source a voucher can be paid from.
type Account struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	IsActive      bool   `json:"is_active"`
}

// Label is how the account reads on a journal row.
func (a Account) Label() string {
	switch a.Kind {
	case AccountBank:
		return strings.TrimSuffix(fmt.Sprintf("Bank %s - %s", a.Name, a.AccountNumber), " - ")
	case AccountKas:
		return "Kas " + a.Name
	}
	return a.Name
}

// boxLabel is how the account reads in the voucher's Bank, Kas, Lainnya box.
func (a Account) boxLabel() string {
	if a.Kind == AccountBank {
		return strings.TrimSuffix(fmt.Sprintf("Bank %s — %s", a.Name, a.AccountNumber), " — ")
	}
	return a.Label()
}

// VoucherSettings is the voucher's letterhead and account list.
type VoucherSettings struct {
	CompanyName  string    `json:"company_name"`
	AddressLine1 string    `json:"address_line1"`
	AddressLine2 string    `json:"address_line2"`
	AddressLine3 string    `json:"address_line3"`
	HasLogo      bool      `json:"has_logo"`
	Accounts     []Account `json:"accounts"`
	logoKey      string
}

// VoucherLine is one journal row. The bank row (IsBank) takes its description
// from the chosen account. ItemKey ties a row to the proofs of a request item.
type VoucherLine struct {
	ItemKey     string `json:"item_key"`
	Description string `json:"description"`
	Debit       int64  `json:"debit"`
	Kredit      int64  `json:"kredit"`
	Note        string `json:"note"`
	ProjectCode string `json:"project_code"`
	IsBank      bool   `json:"is_bank"`
}

// VoucherInput is the sheet finance fills in.
type VoucherInput struct {
	Number      string        `json:"number"`
	Date        string        `json:"date"`
	AccountID   string        `json:"account_id"`
	PayTo       string        `json:"pay_to"`
	Description string        `json:"description"`
	Lines       []VoucherLine `json:"lines"`
	RequestIDs  []string      `json:"request_ids"`
}

// Voucher is an issued voucher; Lines and RequestIDs are filled on the detail.
type Voucher struct {
	ID            string        `json:"id"`
	Number        string        `json:"number"`
	Date          string        `json:"date"`
	AccountID     string        `json:"account_id"`
	PayTo         string        `json:"pay_to"`
	Description   string        `json:"description"`
	TotalAmount   int64         `json:"total_amount"`
	PageCount     int           `json:"page_count"`
	CreatedByName string        `json:"created_by_name"`
	CreatedAt     time.Time     `json:"created_at"`
	Lines         []VoucherLine `json:"lines,omitempty"`
	RequestIDs    []string      `json:"request_ids,omitempty"`
}

// VoucherRef is a voucher a request went into.
type VoucherRef struct {
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

// VoucherDraft is what the chosen requests put on a new sheet.
type VoucherDraft struct {
	PayTo       string        `json:"pay_to"`
	Description string        `json:"description"`
	Lines       []VoucherLine `json:"lines"`
	RequestIDs  []string      `json:"request_ids"`
}

// VoucherContent is a stored voucher bundle.
type VoucherContent struct {
	Reader   io.ReadCloser
	Filename string
}

// requestLines are the journal rows of one request, and what it took out of
// the bank: negative when a kasbon brings money back.
func requestLines(r Request) ([]VoucherLine, int64) {
	items := r.Items
	if r.Type == TypeKasbon {
		items = r.SettlementItems
	}
	project := projectCode(r.RegencyCode)
	rows := make([]VoucherLine, 0, len(items)+1)
	var net int64
	if r.Type == TypeKasbon {
		description := "EX UMK a.n " + r.RequesterName
		if r.PaidAt != nil {
			description += r.PaidAt.In(jakarta).Format(" (Tgl 02/01/2006)")
		}
		rows = append(rows, VoucherLine{Description: description, Kredit: r.TotalAmount, ProjectCode: "EX UMK"})
		net = -r.TotalAmount
	}
	for _, item := range items {
		row := VoucherLine{ItemKey: item.Key, Description: item.Description, Note: item.Note, ProjectCode: item.ProjectCode}
		if row.ProjectCode == "" {
			row.ProjectCode = project
		}
		if item.Entry == EntryKredit {
			row.Kredit = item.Amount
			net -= item.Amount
		} else {
			row.Debit = item.Amount
			net += item.Amount
		}
		rows = append(rows, row)
	}
	return rows, net
}

// composeLines opens with the bank row, as the voucher does, and balances it
// against the rows of every request.
func composeLines(requests []Request) []VoucherLine {
	lines := []VoucherLine{{IsBank: true}}
	var net int64
	for index, r := range requests {
		rows, out := requestLines(r)
		lines = append(lines, rows...)
		net += out
		if index == 0 {
			lines[0].ProjectCode = projectCode(r.RegencyCode)
		}
	}
	if net >= 0 {
		lines[0].Kredit = net
	} else {
		lines[0].Debit = -net
	}
	return lines
}

// projectCode joins the program and the regency's document code: KONKIT-BKA.
func projectCode(regencyCode string) string {
	if regencyCode == "" {
		return ""
	}
	return "KONKIT-" + regencyCode
}

func voucherTotals(lines []VoucherLine) (debit, kredit int64) {
	for _, line := range lines {
		debit += line.Debit
		kredit += line.Kredit
	}
	return debit, kredit
}

func (s *Service) VoucherSettings(ctx context.Context) (VoucherSettings, error) {
	var settings VoucherSettings
	if err := s.pool.QueryRow(ctx, `SELECT company_name, address_line1, address_line2, address_line3, logo_key FROM finance_voucher_settings`).
		Scan(&settings.CompanyName, &settings.AddressLine1, &settings.AddressLine2, &settings.AddressLine3, &settings.logoKey); err != nil {
		return VoucherSettings{}, fmt.Errorf("load voucher settings: %w", err)
	}
	settings.HasLogo = true
	rows, err := s.pool.Query(ctx, `SELECT id::text, kind, name, account_number, is_active FROM finance_accounts ORDER BY position, name`)
	if err != nil {
		return VoucherSettings{}, fmt.Errorf("list finance accounts: %w", err)
	}
	settings.Accounts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) {
		var account Account
		return account, row.Scan(&account.ID, &account.Kind, &account.Name, &account.AccountNumber, &account.IsActive)
	})
	if err != nil {
		return VoucherSettings{}, fmt.Errorf("list finance accounts: %w", err)
	}
	return settings, nil
}

// SaveVoucherSettings replaces the letterhead and accounts. An account left
// out only goes inactive, because issued vouchers still name it.
func (s *Service) SaveVoucherSettings(ctx context.Context, actor auth.Principal, input VoucherSettings, meta auth.ClientMeta) (VoucherSettings, error) {
	company := strings.TrimSpace(input.CompanyName)
	address1, address2, address3 := strings.TrimSpace(input.AddressLine1), strings.TrimSpace(input.AddressLine2), strings.TrimSpace(input.AddressLine3)
	if company == "" || len(company) > maxShortText || len(address1) > maxShortText || len(address2) > maxShortText || len(address3) > maxShortText {
		return VoucherSettings{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return VoucherSettings{}, fmt.Errorf("begin voucher settings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE finance_voucher_settings SET company_name = $1, address_line1 = $2, address_line2 = $3, address_line3 = $4, updated_at = now()`, company, address1, address2, address3); err != nil {
		return VoucherSettings{}, fmt.Errorf("save voucher settings: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE finance_accounts SET is_active = false`); err != nil {
		return VoucherSettings{}, fmt.Errorf("save finance accounts: %w", err)
	}
	for position, account := range input.Accounts {
		name, number := strings.TrimSpace(account.Name), strings.TrimSpace(account.AccountNumber)
		if name == "" || (account.Kind != AccountBank && account.Kind != AccountKas && account.Kind != AccountLainnya) {
			return VoucherSettings{}, ErrInvalidInput
		}
		if account.ID == "" {
			account.ID = uuid.NewString()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO finance_accounts (id, kind, name, account_number, is_active, position) VALUES ($1::uuid, $2, $3, $4, true, $5)
			ON CONFLICT (id) DO UPDATE SET kind = EXCLUDED.kind, name = EXCLUDED.name, account_number = EXCLUDED.account_number, is_active = true, position = EXCLUDED.position`,
			account.ID, account.Kind, name, number, position); err != nil {
			return VoucherSettings{}, ErrInvalidInput
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance.voucher_settings_saved", ResourceType: "finance_voucher_settings", ResourceID: "voucher", IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return VoucherSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return VoucherSettings{}, fmt.Errorf("commit voucher settings: %w", err)
	}
	return s.VoucherSettings(ctx)
}

// SaveLogo stores the letterhead logo.
func (s *Service) SaveLogo(ctx context.Context, source io.Reader) error {
	if s.storage == nil {
		return errMissingStorage
	}
	data, err := io.ReadAll(io.LimitReader(source, maxLogoBytes+1))
	if err != nil {
		return fmt.Errorf("read voucher logo: %w", err)
	}
	if len(data) > maxLogoBytes {
		return ErrAttachmentSize
	}
	contentType := http.DetectContentType(data)
	if contentType != "image/png" && contentType != "image/jpeg" {
		return ErrAttachmentType
	}
	settings, err := s.VoucherSettings(ctx)
	if err != nil {
		return err
	}
	key, _, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), "logo-voucher."+attachmentExtensions[contentType], []string{"KEUANGAN"}, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("store voucher logo: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE finance_voucher_settings SET logo_key = $1, updated_at = now()`, key); err != nil {
		s.removeFile(key)
		return fmt.Errorf("save voucher logo: %w", err)
	}
	if settings.logoKey != "" {
		s.removeFile(settings.logoKey)
	}
	return nil
}

// Logo is the uploaded letterhead logo, or the company's default one.
func (s *Service) Logo(ctx context.Context) ([]byte, error) {
	settings, err := s.VoucherSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.logoKey == "" || s.storage == nil {
		return defaultVoucherLogo, nil
	}
	return s.readFile(ctx, settings.logoKey)
}

func (s *Service) readFile(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.storage.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open finance file: %w", err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// voucherRequests loads requests that may go on a voucher: finished, of this
// finance's teams, and not their own.
func (s *Service) voucherRequests(ctx context.Context, actor auth.Principal, access Access, ids []string) ([]Request, []string, error) {
	seen := map[string]bool{}
	requests, unique := []Request{}, []string{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := uuid.Parse(id); err != nil {
			return nil, nil, ErrInvalidInput
		}
		r, err := s.Get(ctx, actor, access, id)
		if err != nil {
			return nil, nil, err
		}
		if !r.CanIssueVoucher {
			return nil, nil, ErrVoucherNotReady
		}
		requests = append(requests, r)
		unique = append(unique, id)
	}
	return requests, unique, nil
}

// VoucherCandidates lists finished requests of a period, by the day each was
// finished, with the vouchers they already went into.
func (s *Service) VoucherCandidates(ctx context.Context, actor auth.Principal, access Access, from, to string) ([]Request, error) {
	if from == "" || to == "" {
		return nil, ErrInvalidInput
	}
	condition, args, err := where(actor, access, Filter{Scope: ScopeQueue, Status: StatusDone, From: "", To: ""})
	if err != nil {
		return nil, err
	}
	if !validDate(from) || !validDate(to) {
		return nil, ErrInvalidInput
	}
	args = append(args, from, to, actor.UserID)
	condition += fmt.Sprintf(` AND r.done_at >= ($%d::date::timestamp AT TIME ZONE 'Asia/Jakarta') AND r.done_at < (($%d::date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta') AND r.requester_id <> $%d::uuid`,
		len(args)-2, len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, `SELECT `+requestColumns+requestJoins+condition+` ORDER BY r.done_at DESC LIMIT `+fmt.Sprint(maxVoucherRefRows), args...)
	if err != nil {
		return nil, fmt.Errorf("list voucher candidates: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Request, error) { return scanRequest(row) })
	if err != nil {
		return nil, fmt.Errorf("list voucher candidates: %w", err)
	}
	for index := range result {
		if result[index].Vouchers, err = s.voucherRefs(ctx, result[index].ID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ComposeVoucher fills a new sheet from the chosen requests.
func (s *Service) ComposeVoucher(ctx context.Context, actor auth.Principal, access Access, ids []string) (VoucherDraft, error) {
	if !access.CanReview {
		return VoucherDraft{}, ErrForbidden
	}
	requests, unique, err := s.voucherRequests(ctx, actor, access, ids)
	if err != nil {
		return VoucherDraft{}, err
	}
	draft := VoucherDraft{Lines: composeLines(requests), RequestIDs: unique}
	if len(requests) == 1 {
		draft.PayTo = requests[0].PayeeName
		if draft.PayTo == "" {
			draft.PayTo = requests[0].RequesterName
		}
		draft.Description = requests[0].Description
	}
	return draft, nil
}

// normalize checks the sheet: every row filled on one side, and the two sides
// equal. The bank row reads as the chosen account.
func (input *VoucherInput) normalize(account Account) error {
	input.Number, input.PayTo, input.Description = strings.TrimSpace(input.Number), strings.TrimSpace(input.PayTo), strings.TrimSpace(input.Description)
	if !voucherNumberPattern.MatchString(input.Number) || input.PayTo == "" || len(input.PayTo) > maxShortText || len(input.Description) > maxLongText {
		return ErrInvalidInput
	}
	if _, err := time.Parse(dateLayout, input.Date); err != nil {
		return ErrInvalidInput
	}
	if len(input.Lines) == 0 || len(input.Lines) > maxVoucherLines {
		return ErrInvalidInput
	}
	for index := range input.Lines {
		line := &input.Lines[index]
		if line.IsBank {
			line.Description = account.Label()
		}
		line.Description, line.Note, line.ProjectCode = strings.TrimSpace(line.Description), strings.TrimSpace(line.Note), strings.TrimSpace(line.ProjectCode)
		if line.Description == "" || len(line.Description) > maxShortText || len(line.Note) > maxShortText || len(line.ProjectCode) > maxShortText {
			return ErrInvalidInput
		}
		if line.Debit < 0 || line.Kredit < 0 || line.Debit > maxLineAmount || line.Kredit > maxLineAmount || (line.Debit > 0 && line.Kredit > 0) {
			return ErrInvalidInput
		}
		if line.ItemKey != "" {
			if _, err := uuid.Parse(line.ItemKey); err != nil {
				return ErrInvalidInput
			}
		}
	}
	debit, kredit := voucherTotals(input.Lines)
	if debit != kredit || debit == 0 {
		return ErrUnbalanced
	}
	return nil
}

// checkVoucher finds the chosen account and checks the sheet against it.
func (s *Service) checkVoucher(ctx context.Context, access Access, input *VoucherInput) (VoucherSettings, Account, error) {
	if !access.CanReview {
		return VoucherSettings{}, Account{}, ErrForbidden
	}
	if s.storage == nil {
		return VoucherSettings{}, Account{}, errMissingStorage
	}
	settings, err := s.VoucherSettings(ctx)
	if err != nil {
		return VoucherSettings{}, Account{}, err
	}
	for _, candidate := range settings.Accounts {
		if candidate.ID == input.AccountID && candidate.IsActive && settings.CompanyName != "" {
			return settings, candidate, input.normalize(candidate)
		}
	}
	return VoucherSettings{}, Account{}, ErrVoucherSettings
}

// buildBundle renders the voucher and every proof into one PDF.
func (s *Service) buildBundle(ctx context.Context, actor auth.Principal, access Access, input VoucherInput, settings VoucherSettings, account Account) ([]byte, []string, error) {
	requests, requestIDs, err := s.voucherRequests(ctx, actor, access, input.RequestIDs)
	if err != nil {
		return nil, nil, err
	}
	logo, err := s.Logo(ctx)
	if err != nil {
		return nil, nil, err
	}
	proofs, err := s.bundleProofs(ctx, requests, input.Lines)
	if err != nil {
		return nil, nil, err
	}
	bundle, err := renderBundle(voucherDocument{Number: input.Number, Date: input.Date, PayTo: input.PayTo, Description: input.Description,
		Settings: settings, AccountID: account.ID, Logo: logo, Lines: input.Lines}, proofs)
	if err != nil {
		return nil, nil, err
	}
	return bundle, requestIDs, nil
}

// PreviewVoucher renders the sheet exactly as it would issue, and keeps
// nothing. A number or payee not filled in yet does not stop it.
func (s *Service) PreviewVoucher(ctx context.Context, actor auth.Principal, access Access, input VoucherInput) ([]byte, error) {
	if strings.TrimSpace(input.Number) == "" {
		input.Number = "PREVIEW"
	}
	if strings.TrimSpace(input.PayTo) == "" {
		input.PayTo = "-"
	}
	settings, account, err := s.checkVoucher(ctx, access, &input)
	if err != nil {
		return nil, err
	}
	bundle, _, err := s.buildBundle(ctx, actor, access, input, settings, account)
	return bundle, err
}

// SaveVoucher builds the bundle and archives it: a new voucher with an empty
// id, otherwise the given one rebuilt in place.
func (s *Service) SaveVoucher(ctx context.Context, actor auth.Principal, access Access, id string, input VoucherInput, meta auth.ClientMeta) (Voucher, error) {
	settings, account, err := s.checkVoucher(ctx, access, &input)
	if err != nil {
		return Voucher{}, err
	}
	var old string
	if id != "" {
		current, err := s.Voucher(ctx, actor, access, id)
		if err != nil {
			return Voucher{}, err
		}
		if err := s.pool.QueryRow(ctx, `SELECT storage_key FROM finance_vouchers WHERE id = $1::uuid`, current.ID).Scan(&old); err != nil {
			return Voucher{}, fmt.Errorf("load voucher file: %w", err)
		}
	}
	var taken bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM finance_vouchers WHERE number = $1 AND id::text <> $2)`, input.Number, id).Scan(&taken); err != nil {
		return Voucher{}, fmt.Errorf("check voucher number: %w", err)
	}
	if taken {
		return Voucher{}, ErrVoucherNumber
	}
	bundle, requestIDs, err := s.buildBundle(ctx, actor, access, input, settings, account)
	if err != nil {
		return Voucher{}, err
	}
	pages, err := pageCount(bundle)
	if err != nil {
		return Voucher{}, err
	}
	key, _, _, err := media.PutNamed(ctx, s.storage, uuid.NewString(), voucherFilename(input.Number), []string{"KEUANGAN", "VOUCHER", input.Date[:7]}, bytes.NewReader(bundle))
	if err != nil {
		return Voucher{}, fmt.Errorf("store voucher bundle: %w", err)
	}
	id, err = s.storeVoucher(ctx, actor, id, input, account.ID, requestIDs, key, pages, meta)
	if err != nil {
		s.removeFile(key)
		return Voucher{}, err
	}
	if old != "" {
		s.removeFile(old)
	}
	return s.Voucher(ctx, actor, access, id)
}

func (s *Service) storeVoucher(ctx context.Context, actor auth.Principal, id string, input VoucherInput, accountID string, requestIDs []string, key string, pages int, meta auth.ClientMeta) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin voucher: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	total, _ := voucherTotals(input.Lines)
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO finance_vouchers (number, voucher_date, account_id, pay_to, description, total_amount, page_count, storage_key, created_by)
			VALUES ($1, $2::date, $3::uuid, $4, $5, $6, $7, $8, $9::uuid) RETURNING id::text`,
			input.Number, input.Date, accountID, input.PayTo, input.Description, total, pages, key, actor.UserID).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE finance_vouchers SET number = $2, voucher_date = $3::date, account_id = $4::uuid, pay_to = $5, description = $6, total_amount = $7,
			page_count = $8, storage_key = $9, updated_at = now() WHERE id = $1::uuid`, id, input.Number, input.Date, accountID, input.PayTo, input.Description, total, pages, key)
	}
	var unique *pgconn.PgError
	if errors.As(err, &unique) && unique.Code == "23505" {
		return "", ErrVoucherNumber
	}
	if err != nil {
		return "", fmt.Errorf("save voucher: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM finance_voucher_lines WHERE voucher_id = $1::uuid`, id); err != nil {
		return "", fmt.Errorf("replace voucher lines: %w", err)
	}
	for position, line := range input.Lines {
		if _, err := tx.Exec(ctx, `INSERT INTO finance_voucher_lines (voucher_id, position, description, debit, kredit, note, project_code, is_bank, item_key)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid)`, id, position, line.Description, line.Debit, line.Kredit, line.Note, line.ProjectCode, line.IsBank, line.ItemKey); err != nil {
			return "", fmt.Errorf("replace voucher lines: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM finance_voucher_requests WHERE voucher_id = $1::uuid`, id); err != nil {
		return "", fmt.Errorf("replace voucher requests: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_voucher_requests (voucher_id, request_id) SELECT $1::uuid, unnest($2::uuid[])`, id, requestIDs); err != nil {
		return "", fmt.Errorf("replace voucher requests: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance.voucher_saved", ResourceType: "finance_voucher", ResourceID: id,
		Metadata: map[string]any{"voucher_number": input.Number, "pages": pages}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit voucher: %w", err)
	}
	return id, nil
}

func voucherFilename(number string) string {
	return "Voucher_Bundle_" + number + ".pdf"
}

// voucherVisible limits vouchers to their maker, monitoring, the finance of a
// request's team and the requester. Its arguments are $1 user, $2 monitor,
// $3 review.
const voucherVisible = `(v.created_by = $1::uuid OR $2 OR EXISTS (SELECT 1 FROM finance_voucher_requests vr JOIN finance_requests q ON q.id = vr.request_id
	WHERE vr.voucher_id = v.id AND (q.requester_id = $1::uuid OR ($3 AND q.team_id IN (SELECT team_id FROM finance_team_members WHERE member_role = 'finance' AND user_id = $1::uuid)))))`

const voucherSelect = `v.id::text, v.number, v.voucher_date::text, COALESCE(v.account_id::text, ''), v.pay_to, v.description, v.total_amount, v.page_count,
	COALESCE(NULLIF(u.full_name, ''), u.username, ''), v.created_at FROM finance_vouchers v LEFT JOIN users u ON u.id = v.created_by`

func scanVoucher(row pgx.Row) (Voucher, error) {
	var v Voucher
	err := row.Scan(&v.ID, &v.Number, &v.Date, &v.AccountID, &v.PayTo, &v.Description, &v.TotalAmount, &v.PageCount, &v.CreatedByName, &v.CreatedAt)
	return v, err
}

// Vouchers lists what the viewer may see, the latest first.
func (s *Service) Vouchers(ctx context.Context, actor auth.Principal, access Access) ([]Voucher, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+voucherSelect+` WHERE `+voucherVisible+` ORDER BY v.voucher_date DESC, v.created_at DESC LIMIT 200`, actor.UserID, access.CanMonitor, access.CanReview)
	if err != nil {
		return nil, fmt.Errorf("list vouchers: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Voucher, error) { return scanVoucher(row) })
	if err != nil {
		return nil, fmt.Errorf("list vouchers: %w", err)
	}
	return result, nil
}

// Voucher returns one voucher with its rows, to open on the sheet again.
func (s *Service) Voucher(ctx context.Context, actor auth.Principal, access Access, id string) (Voucher, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Voucher{}, ErrNotFound
	}
	voucher, err := scanVoucher(s.pool.QueryRow(ctx, `SELECT `+voucherSelect+` WHERE `+voucherVisible+` AND v.id = $4::uuid`, actor.UserID, access.CanMonitor, access.CanReview, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Voucher{}, ErrNotFound
	}
	if err != nil {
		return Voucher{}, fmt.Errorf("load voucher: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT COALESCE(item_key::text, ''), description, debit, kredit, note, project_code, is_bank FROM finance_voucher_lines WHERE voucher_id = $1::uuid ORDER BY position`, id)
	if err != nil {
		return Voucher{}, fmt.Errorf("load voucher lines: %w", err)
	}
	voucher.Lines, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (VoucherLine, error) {
		var line VoucherLine
		return line, row.Scan(&line.ItemKey, &line.Description, &line.Debit, &line.Kredit, &line.Note, &line.ProjectCode, &line.IsBank)
	})
	if err != nil {
		return Voucher{}, fmt.Errorf("load voucher lines: %w", err)
	}
	links, err := s.pool.Query(ctx, `SELECT request_id::text FROM finance_voucher_requests WHERE voucher_id = $1::uuid`, id)
	if err != nil {
		return Voucher{}, fmt.Errorf("load voucher requests: %w", err)
	}
	voucher.RequestIDs, err = pgx.CollectRows(links, pgx.RowTo[string])
	if err != nil {
		return Voucher{}, fmt.Errorf("load voucher requests: %w", err)
	}
	return voucher, nil
}

// OpenVoucher opens the stored bundle.
func (s *Service) OpenVoucher(ctx context.Context, actor auth.Principal, access Access, id string) (VoucherContent, error) {
	voucher, err := s.Voucher(ctx, actor, access, id)
	if err != nil {
		return VoucherContent{}, err
	}
	var key string
	if err := s.pool.QueryRow(ctx, `SELECT storage_key FROM finance_vouchers WHERE id = $1::uuid`, voucher.ID).Scan(&key); err != nil {
		return VoucherContent{}, fmt.Errorf("load voucher file: %w", err)
	}
	reader, err := s.storage.Open(ctx, key)
	if err != nil {
		return VoucherContent{}, fmt.Errorf("open voucher: %w", err)
	}
	return VoucherContent{Reader: reader, Filename: voucherFilename(voucher.Number)}, nil
}

// voucherRefs are the vouchers a request went into, the latest first.
func (s *Service) voucherRefs(ctx context.Context, requestID string) ([]VoucherRef, error) {
	rows, err := s.pool.Query(ctx, `SELECT v.id::text, v.number, v.voucher_date::text, v.created_at FROM finance_voucher_requests vr JOIN finance_vouchers v ON v.id = vr.voucher_id
		WHERE vr.request_id = $1::uuid ORDER BY v.created_at DESC`, requestID)
	if err != nil {
		return nil, fmt.Errorf("list voucher refs: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (VoucherRef, error) {
		var ref VoucherRef
		return ref, row.Scan(&ref.ID, &ref.Number, &ref.Date, &ref.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("list voucher refs: %w", err)
	}
	return result, nil
}

// paymentRows finds the row each request's payment proof prints under: the EX
// UMK row of a kasbon, otherwise the request's first row. A request whose rows
// were removed from the sheet has none.
func paymentRows(requests []Request, lines []VoucherLine) map[string]int {
	rows := map[string]int{}
	taken := map[int]bool{}
	for _, r := range requests {
		keys := map[string]bool{}
		for _, item := range append(append([]Item{}, r.Items...), r.SettlementItems...) {
			keys[item.Key] = true
		}
		advance := "EX UMK a.n " + r.RequesterName
		for index, line := range lines {
			if r.Type == TypeKasbon && !taken[index] && strings.HasPrefix(line.Description, advance) {
				rows[r.ID], taken[index] = index, true
				break
			}
		}
		if _, found := rows[r.ID]; found {
			continue
		}
		for index, line := range lines {
			if line.ItemKey != "" && keys[line.ItemKey] {
				rows[r.ID] = index
				break
			}
		}
	}
	return rows
}

// bundleProofs lists every proof in bundle order: row by row, what the leader
// sent before what finance paid with, then what belongs to a whole request.
func (s *Service) bundleProofs(ctx context.Context, requests []Request, lines []VoucherLine) ([]bundleProof, error) {
	type stored struct{ id, kind, item, key, contentType, name, request string }
	ids := make([]string, len(requests))
	names := map[string]string{}
	for index, r := range requests {
		ids[index] = r.ID
		names[r.ID] = r.RequesterName
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, kind, COALESCE(item_key::text, ''), storage_key, content_type, COALESCE(NULLIF(original_name, ''), filename), request_id::text
		FROM finance_request_attachments WHERE request_id = ANY($1::uuid[]) ORDER BY created_at`, ids)
	if err != nil {
		return nil, fmt.Errorf("list voucher proofs: %w", err)
	}
	files, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stored, error) {
		var f stored
		return f, row.Scan(&f.id, &f.kind, &f.item, &f.key, &f.contentType, &f.name, &f.request)
	})
	if err != nil {
		return nil, fmt.Errorf("list voucher proofs: %w", err)
	}
	proofs := []bundleProof{}
	add := func(rowLabel string, matches []stored, label string) error {
		for index, file := range matches {
			data, err := s.readFile(ctx, file.key)
			if err != nil {
				return err
			}
			title := label
			if len(matches) > 1 {
				title = fmt.Sprintf("%s (%d/%d)", label, index+1, len(matches))
			}
			if rowLabel != "" {
				title = rowLabel + " - " + title
			}
			proofs = append(proofs, bundleProof{Title: title, Requester: names[file.request], Filename: file.name, ContentType: file.contentType, Data: data})
		}
		return nil
	}
	pick := func(match func(stored) bool) []stored {
		result := []stored{}
		for _, file := range files {
			if match(file) {
				result = append(result, file)
			}
		}
		return result
	}
	payments := paymentRows(requests, lines)
	for index, line := range lines {
		rowLabel := fmt.Sprintf("Row %d", index+1)
		if key := line.ItemKey; key != "" {
			if err := add(rowLabel, pick(func(f stored) bool { return f.item == key && f.kind == KindItemTransaction }), "Bukti Transaksi"); err != nil {
				return nil, err
			}
			if err := add(rowLabel, pick(func(f stored) bool { return f.item == key && f.kind == KindItemPayment }), "Bukti Pembayaran"); err != nil {
				return nil, err
			}
		}
		for _, r := range requests {
			if row, found := payments[r.ID]; found && row == index {
				if err := add(rowLabel, pick(func(f stored) bool { return f.request == r.ID && f.kind == KindPayment }), kindTitles[KindPayment]); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, r := range requests {
		kinds := []string{KindEvidence, KindSettlement}
		if _, found := payments[r.ID]; !found {
			kinds = append(kinds, KindPayment)
		}
		for _, kind := range kinds {
			if err := add("", pick(func(f stored) bool { return f.request == r.ID && f.kind == kind }), kindTitles[kind]); err != nil {
				return nil, err
			}
		}
	}
	return proofs, nil
}

var kindTitles = map[string]string{KindEvidence: "Dokumen Pendukung", KindSettlement: "Nota Realisasi", KindPayment: "Bukti Pembayaran"}
