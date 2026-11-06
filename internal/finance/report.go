package finance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"
)

const (
	ScopeMine  = "mine"
	ScopeQueue = "queue"
	ScopeAll   = "all"

	maxPageSize   = 100
	maxExportRows = 20000
)

// Filter selects requests for a list, a summary or an export.
type Filter struct {
	Scope  string
	Type   string
	Status string
	TeamID string
	From   string
	To     string
	Search string
	Limit  int
	Offset int
}

type Page struct {
	Rows  []Request `json:"rows"`
	Total int       `json:"total"`
}

// where builds the conditions of a filter. Scope is the access rule: mine is
// the user's own, queue the teams they handle (never drafts), all needs the
// monitoring permission.
func where(actor auth.Principal, access Access, filter Filter) (string, []any, error) {
	args := []any{}
	arg := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	conditions := []string{}
	switch filter.Scope {
	case ScopeMine:
		conditions = append(conditions, "r.requester_id = "+arg(actor.UserID)+"::uuid")
	case ScopeQueue:
		if !access.CanReview {
			return "", nil, ErrForbidden
		}
		conditions = append(conditions, "r.status <> 'draft'", `r.team_id IN (SELECT team_id FROM finance_team_members WHERE member_role = 'finance' AND user_id = `+arg(actor.UserID)+`::uuid)`)
	case ScopeAll:
		if !access.CanMonitor {
			return "", nil, ErrForbidden
		}
		conditions = append(conditions, "r.status <> 'draft'")
	default:
		return "", nil, ErrInvalidInput
	}
	if filter.Type != "" {
		conditions = append(conditions, "r.request_type = "+arg(filter.Type))
	}
	if filter.Status != "" {
		conditions = append(conditions, "r.status = ANY("+arg(strings.Split(filter.Status, ","))+")")
	}
	if filter.TeamID != "" {
		conditions = append(conditions, "r.team_id = "+arg(filter.TeamID)+"::uuid")
	}
	if !validDate(filter.From) || !validDate(filter.To) {
		return "", nil, ErrInvalidInput
	}
	if filter.From != "" {
		conditions = append(conditions, "COALESCE(r.submitted_at, r.created_at) >= ("+arg(filter.From)+"::date::timestamp AT TIME ZONE 'Asia/Jakarta')")
	}
	if filter.To != "" {
		conditions = append(conditions, "COALESCE(r.submitted_at, r.created_at) < (("+arg(filter.To)+"::date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta')")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := arg("%" + search + "%")
		conditions = append(conditions, "(r.number ILIKE "+pattern+" OR r.description ILIKE "+pattern+" OR r.payee_name ILIKE "+pattern+" OR t.name ILIKE "+pattern+")")
	}
	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}

func (s *Service) List(ctx context.Context, actor auth.Principal, access Access, filter Filter) (Page, error) {
	condition, args, err := where(actor, access, filter)
	if err != nil {
		return Page{}, err
	}
	limit := filter.Limit
	if limit < 1 || limit > maxPageSize {
		limit = 50
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*)`+requestJoins+condition, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count finance requests: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+requestColumns+requestJoins+condition+
		` ORDER BY COALESCE(r.submitted_at, r.updated_at) DESC LIMIT `+strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(max(filter.Offset, 0)), args...)
	if err != nil {
		return Page{}, fmt.Errorf("list finance requests: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Request, error) { return scanRequest(row) })
	if err != nil {
		return Page{}, fmt.Errorf("list finance requests: %w", err)
	}
	return Page{Rows: result, Total: total}, nil
}

type StatusTotal struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Amount int64  `json:"amount"`
}

type TeamTotal struct {
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Paid   int64  `json:"paid"`
	Open   int64  `json:"open"`
}

// Summary is the monitoring view of the filtered requests.
type Summary struct {
	Statuses []StatusTotal `json:"statuses"`
	Teams    []TeamTotal   `json:"teams"`
	// KasbonOpen is money advanced and not yet reported; KasbonOverdue the
	// part of those past the report date.
	KasbonOpen      int64 `json:"kasbon_open"`
	KasbonOpenCount int   `json:"kasbon_open_count"`
	KasbonOverdue   int   `json:"kasbon_overdue"`
	WaitingReview   int   `json:"waiting_review"`
	WaitingPayment  int   `json:"waiting_payment"`
	TotalPaid       int64 `json:"total_paid"`
}

func (s *Service) Summary(ctx context.Context, actor auth.Principal, access Access, filter Filter) (Summary, error) {
	filter.Status = ""
	condition, args, err := where(actor, access, filter)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Statuses: []StatusTotal{}, Teams: []TeamTotal{}}
	rows, err := s.pool.Query(ctx, `SELECT r.status, count(*), COALESCE(sum(r.total_amount), 0)`+requestJoins+condition+` GROUP BY r.status ORDER BY r.status`, args...)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize finance status: %w", err)
	}
	summary.Statuses, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StatusTotal, error) {
		var total StatusTotal
		return total, row.Scan(&total.Status, &total.Count, &total.Amount)
	})
	if err != nil {
		return Summary{}, fmt.Errorf("summarize finance status: %w", err)
	}
	for _, total := range summary.Statuses {
		switch total.Status {
		case StatusSubmitted, StatusSettlementSubmitted:
			summary.WaitingReview += total.Count
		case StatusApproved:
			summary.WaitingPayment += total.Count
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT COALESCE(r.team_id::text, ''), COALESCE(t.name, '-'), count(*),
		COALESCE(sum(r.total_amount) FILTER (WHERE r.paid_at IS NOT NULL), 0),
		COALESCE(sum(r.total_amount) FILTER (WHERE r.status IN ('submitted', 'approved')), 0)`+requestJoins+condition+` GROUP BY 1, 2 ORDER BY 2`, args...)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize finance teams: %w", err)
	}
	summary.Teams, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TeamTotal, error) {
		var total TeamTotal
		return total, row.Scan(&total.TeamID, &total.Name, &total.Count, &total.Paid, &total.Open)
	})
	if err != nil {
		return Summary{}, fmt.Errorf("summarize finance teams: %w", err)
	}
	for _, total := range summary.Teams {
		summary.TotalPaid += total.Paid
	}
	today := time.Now().In(jakarta).Format(dateLayout)
	overdueArg := "$" + strconv.Itoa(len(args)+1)
	err = s.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(r.total_amount), 0), count(*) FILTER (WHERE r.settlement_due < `+overdueArg+`::date)`+requestJoins+condition+
		` AND r.request_type = 'kasbon' AND r.status IN ('paid', 'settlement_submitted', 'settlement_revision')`, append(args, today)...).
		Scan(&summary.KasbonOpenCount, &summary.KasbonOpen, &summary.KasbonOverdue)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize open kasbon: %w", err)
	}
	return summary, nil
}

var reportHeaders = []string{"No", "Nomor", "Jenis", "Tim", "Kabupaten", "Pemohon", "Kategori", "Keterangan", "Tanggal butuh", "Diajukan", "Total (Rp)",
	"Penerima", "Bank", "No. rekening", "Atas nama", "Status", "Direview oleh", "Tanggal review", "Dibayar oleh", "Tanggal bayar",
	"Target laporan", "Realisasi (Rp)", "Selisih (Rp)", "Catatan review"}

// DailyReport is the Excel of one day: every request that moved that day.
func (s *Service) DailyReport(ctx context.Context, actor auth.Principal, access Access, date, scope, teamID string) ([]byte, error) {
	if date == "" || !validDate(date) {
		return nil, ErrInvalidInput
	}
	condition, args, err := where(actor, access, Filter{Scope: scope, TeamID: teamID})
	if err != nil {
		return nil, err
	}
	args = append(args, date)
	day := "$" + strconv.Itoa(len(args))
	condition += ` AND EXISTS (SELECT 1 FROM finance_request_events e WHERE e.request_id = r.id
		AND e.created_at >= (` + day + `::date::timestamp AT TIME ZONE 'Asia/Jakarta')
		AND e.created_at < ((` + day + `::date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta'))`
	requests, err := s.reportRequests(ctx, condition, args)
	if err != nil {
		return nil, err
	}
	return buildReport(requests, "Laporan harian "+date)
}

// ExportReport is the Excel of the whole filter, for monitoring.
func (s *Service) ExportReport(ctx context.Context, actor auth.Principal, access Access, filter Filter) ([]byte, error) {
	condition, args, err := where(actor, access, filter)
	if err != nil {
		return nil, err
	}
	requests, err := s.reportRequests(ctx, condition, args)
	if err != nil {
		return nil, err
	}
	title := "Laporan pengajuan"
	if filter.From != "" || filter.To != "" {
		title = fmt.Sprintf("%s %s s.d. %s", title, filter.From, filter.To)
	}
	return buildReport(requests, title)
}

func (s *Service) reportRequests(ctx context.Context, condition string, args []any) ([]Request, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+requestColumns+requestJoins+condition+` ORDER BY r.submitted_at NULLS LAST, r.number LIMIT `+strconv.Itoa(maxExportRows), args...)
	if err != nil {
		return nil, fmt.Errorf("list finance report: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Request, error) { return scanRequest(row) })
	if err != nil {
		return nil, fmt.Errorf("list finance report: %w", err)
	}
	return result, nil
}

func reportTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.In(jakarta).Format("02/01/2006 15:04")
}

func reportValues(index int, r Request) []any {
	values := []any{index, r.Number, typeLabel(r.Type), r.TeamName, r.RegencyName, r.RequesterName, r.Category, r.Description, r.NeededDate, reportTime(r.SubmittedAt), r.TotalAmount,
		r.PayeeName, r.BankName, r.AccountNumber, r.AccountName, statusLabels[r.Status], r.ReviewedByName, reportTime(r.ReviewedAt), r.PaidByName, reportTime(r.PaidAt),
		r.SettlementDue, "", "", r.ReviewNote}
	if r.SettlementAmount != nil {
		values[21], values[22] = *r.SettlementAmount, r.TotalAmount-*r.SettlementAmount
	}
	return values
}

// buildReport writes the rows and a recap. Text goes in with SetCellStr so a
// value such as =SUM(...) stays text and a long account number keeps its digits.
func buildReport(requests []Request, title string) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	const sheet, recap = "Pengajuan", "Ringkasan"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("name finance sheet: %w", err)
	}
	if _, err := file.NewSheet(recap); err != nil {
		return nil, fmt.Errorf("add finance recap: %w", err)
	}
	bold, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, fmt.Errorf("style finance report: %w", err)
	}
	money, err := file.NewStyle(&excelize.Style{NumFmt: 3})
	if err != nil {
		return nil, fmt.Errorf("style finance report: %w", err)
	}
	put := func(name string, column, row int, value any) error {
		cell, err := excelize.CoordinatesToCellName(column, row)
		if err != nil {
			return err
		}
		if text, ok := value.(string); ok {
			return file.SetCellStr(name, cell, text)
		}
		return file.SetCellValue(name, cell, value)
	}
	for column, header := range reportHeaders {
		if err := put(sheet, column+1, 1, header); err != nil {
			return nil, fmt.Errorf("write finance header: %w", err)
		}
	}
	if err := file.SetRowStyle(sheet, 1, 1, bold); err != nil {
		return nil, fmt.Errorf("style finance header: %w", err)
	}
	totals := map[string]*StatusTotal{}
	order := []string{}
	for index, r := range requests {
		for column, value := range reportValues(index+1, r) {
			if err := put(sheet, column+1, index+2, value); err != nil {
				return nil, fmt.Errorf("write finance row: %w", err)
			}
		}
		key := typeLabel(r.Type) + " - " + statusLabels[r.Status]
		if totals[key] == nil {
			totals[key] = &StatusTotal{Status: key}
			order = append(order, key)
		}
		totals[key].Count++
		totals[key].Amount += r.TotalAmount
	}
	for _, column := range []string{"K", "V", "W"} {
		if err := file.SetColStyle(sheet, column, money); err != nil {
			return nil, fmt.Errorf("style finance money: %w", err)
		}
	}
	if err := file.SetColWidth(sheet, "B", "X", 18); err != nil {
		return nil, fmt.Errorf("size finance columns: %w", err)
	}
	if err := file.SetRowStyle(sheet, 1, 1, bold); err != nil {
		return nil, fmt.Errorf("style finance header: %w", err)
	}
	if err := put(recap, 1, 1, title); err != nil {
		return nil, fmt.Errorf("write finance recap: %w", err)
	}
	for column, header := range []string{"Jenis dan status", "Jumlah", "Total (Rp)"} {
		if err := put(recap, column+1, 3, header); err != nil {
			return nil, fmt.Errorf("write finance recap: %w", err)
		}
	}
	if err := file.SetRowStyle(recap, 3, 3, bold); err != nil {
		return nil, fmt.Errorf("style finance recap: %w", err)
	}
	for index, key := range order {
		row := index + 4
		for column, value := range []any{key, totals[key].Count, totals[key].Amount} {
			if err := put(recap, column+1, row, value); err != nil {
				return nil, fmt.Errorf("write finance recap: %w", err)
			}
		}
	}
	if err := file.SetColStyle(recap, "C", money); err != nil {
		return nil, fmt.Errorf("style finance recap: %w", err)
	}
	if err := file.SetColWidth(recap, "A", "A", 34); err != nil {
		return nil, fmt.Errorf("size finance recap: %w", err)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode finance report: %w", err)
	}
	return buffer.Bytes(), nil
}
