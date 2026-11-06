package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // reads the size of JPEG proofs
	_ "image/png"  // reads the size of PNG proofs
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/media"
	"konkit/internal/notifications"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notifier wakes the push worker after a commit.
type Notifier interface{ Wake() }

type Service struct {
	pool     *pgxpool.Pool
	storage  media.Storage
	notifier Notifier
}

func NewService(pool *pgxpool.Pool, storage media.Storage, notifier Notifier) *Service {
	return &Service{pool: pool, storage: storage, notifier: notifier}
}

const requestColumns = `r.id::text, COALESCE(r.number, ''), r.request_type, r.status, COALESCE(r.team_id::text, ''), COALESCE(t.name, ''),
	r.requester_id::text, COALESCE(NULLIF(u.full_name, ''), u.username), COALESCE(r.regency_id::text, ''), COALESCE(g.name, ''), COALESCE(g.document_code, ''),
	COALESCE(r.needed_date::text, ''), r.category, r.description, r.total_amount, r.payee_name, r.bank_name, r.account_number, r.account_name,
	r.invoice_number, COALESCE(r.settlement_due::text, ''), r.settlement_amount, r.settlement_note, r.review_note,
	COALESCE(NULLIF(rv.full_name, ''), rv.username, ''), COALESCE(NULLIF(pd.full_name, ''), pd.username, ''),
	r.submitted_at, r.reviewed_at, r.paid_at, r.done_at, r.created_at, r.updated_at`

const requestJoins = ` FROM finance_requests r
	JOIN users u ON u.id = r.requester_id
	LEFT JOIN finance_teams t ON t.id = r.team_id
	LEFT JOIN regencies g ON g.id = r.regency_id
	LEFT JOIN users rv ON rv.id = r.reviewed_by
	LEFT JOIN users pd ON pd.id = r.paid_by`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.TeamID, &r.TeamName, &r.RequesterID, &r.RequesterName, &r.RegencyID, &r.RegencyName, &r.RegencyCode,
		&r.NeededDate, &r.Category, &r.Description, &r.TotalAmount, &r.PayeeName, &r.BankName, &r.AccountNumber, &r.AccountName,
		&r.InvoiceNumber, &r.SettlementDue, &r.SettlementAmount, &r.SettlementNote, &r.ReviewNote, &r.ReviewedByName, &r.PaidByName,
		&r.SubmittedAt, &r.ReviewedAt, &r.PaidAt, &r.DoneAt, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadRequest(ctx context.Context, q querier, id string, lock bool) (Request, error) {
	query := `SELECT ` + requestColumns + requestJoins + ` WHERE r.id = $1::uuid`
	if lock {
		query += ` FOR UPDATE OF r`
	}
	r, err := scanRequest(q.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, fmt.Errorf("load finance request: %w", err)
	}
	return r, nil
}

func isMember(ctx context.Context, q querier, teamID, userID, role string) (bool, error) {
	if teamID == "" {
		return false, nil
	}
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM finance_team_members m JOIN finance_teams t ON t.id = m.team_id
		WHERE m.team_id = $1::uuid AND m.user_id = $2::uuid AND m.member_role = $3 AND t.is_active)`, teamID, userID, role).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check team member: %w", err)
	}
	return found, nil
}

// relation is how the viewer stands to a request. Someone with no relation
// is told it does not exist.
func (s *Service) relation(ctx context.Context, q querier, r Request, actor auth.Principal, access Access) (requester, handler bool, err error) {
	requester = r.RequesterID == actor.UserID
	if access.CanReview && r.Status != StatusDraft {
		if handler, err = isMember(ctx, q, r.TeamID, actor.UserID, "finance"); err != nil {
			return false, false, err
		}
	}
	if !requester && !handler && (!access.CanMonitor || r.Status == StatusDraft) {
		return false, false, ErrNotFound
	}
	return requester, handler, nil
}

// Get returns the detail with what the viewer may do next.
func (s *Service) Get(ctx context.Context, actor auth.Principal, access Access, id string) (Request, error) {
	r, err := loadRequest(ctx, s.pool, id, false)
	if err != nil {
		return Request{}, err
	}
	requester, handler, err := s.relation(ctx, s.pool, r, actor, access)
	if err != nil {
		return Request{}, err
	}
	if r.Items, err = loadItems(ctx, s.pool, id, "request"); err != nil {
		return Request{}, err
	}
	if r.SettlementItems, err = loadItems(ctx, s.pool, id, "settlement"); err != nil {
		return Request{}, err
	}
	if r.Attachments, err = s.attachments(ctx, id); err != nil {
		return Request{}, err
	}
	if r.Events, err = s.events(ctx, id); err != nil {
		return Request{}, err
	}
	r.Actions = viewerActions(r, requester, handler)
	r.Editable = requester && editable(r.Status)
	r.UploadKinds = uploadKinds(r, requester, handler)
	if r.Vouchers, err = s.voucherRefs(ctx, id); err != nil {
		return Request{}, err
	}
	r.CanIssueVoucher = handler && !requester && r.Status == StatusDone
	return r, nil
}

func loadItems(ctx context.Context, q querier, id, phase string) ([]Item, error) {
	rows, err := q.Query(ctx, `SELECT item_key::text, entry, note, project_code, description, quantity, unit_price, amount FROM finance_request_items WHERE request_id = $1::uuid AND phase = $2 ORDER BY position`, id, phase)
	if err != nil {
		return nil, fmt.Errorf("list finance items: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var item Item
		return item, row.Scan(&item.Key, &item.Entry, &item.Note, &item.ProjectCode, &item.Description, &item.Quantity, &item.UnitPrice, &item.Amount)
	})
	if err != nil {
		return nil, fmt.Errorf("list finance items: %w", err)
	}
	return items, nil
}

func (s *Service) attachments(ctx context.Context, id string) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, kind, COALESCE(item_key::text, ''), COALESCE(NULLIF(original_name, ''), filename), content_type, size_bytes, created_at FROM finance_request_attachments WHERE request_id = $1::uuid ORDER BY created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("list finance attachments: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attachment, error) {
		var a Attachment
		return a, row.Scan(&a.ID, &a.Kind, &a.ItemKey, &a.Filename, &a.ContentType, &a.SizeBytes, &a.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("list finance attachments: %w", err)
	}
	return result, nil
}

func (s *Service) events(ctx context.Context, id string) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.action, e.from_status, e.to_status, e.note, COALESCE(NULLIF(u.full_name, ''), u.username, ''), e.created_at
		FROM finance_request_events e LEFT JOIN users u ON u.id = e.actor_id WHERE e.request_id = $1::uuid ORDER BY e.created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("list finance events: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		return e, row.Scan(&e.Action, &e.FromStatus, &e.ToStatus, &e.Note, &e.ActorName, &e.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("list finance events: %w", err)
	}
	return result, nil
}

// SaveDraft creates or updates the request with the id the client chose, so
// autosave can repeat safely after a lost connection.
func (s *Service) SaveDraft(ctx context.Context, actor auth.Principal, access Access, id string, input DraftInput) (Request, error) {
	if !access.CanRequest {
		return Request{}, ErrForbidden
	}
	if _, err := uuid.Parse(id); err != nil {
		return Request{}, ErrInvalidInput
	}
	if err := input.normalize(); err != nil {
		return Request{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("begin finance draft: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.checkTeam(ctx, tx, actor.UserID, input.TeamID, input.RegencyID); err != nil {
		return Request{}, err
	}
	current, err := loadRequest(ctx, tx, id, true)
	switch {
	case errors.Is(err, ErrNotFound):
		if _, err := tx.Exec(ctx, `INSERT INTO finance_requests (id, request_type, requester_id) VALUES ($1::uuid, $2, $3::uuid) ON CONFLICT (id) DO NOTHING`, id, input.Type, actor.UserID); err != nil {
			return Request{}, fmt.Errorf("create finance draft: %w", err)
		}
		if current, err = loadRequest(ctx, tx, id, true); err != nil {
			return Request{}, err
		}
	case err != nil:
		return Request{}, err
	}
	if current.RequesterID != actor.UserID {
		return Request{}, ErrNotFound
	}
	if !editable(current.Status) {
		return Request{}, ErrInvalidStatus
	}
	_, total, err := normalizeItems(input.Items)
	if err != nil {
		return Request{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE finance_requests SET request_type = $2, team_id = NULLIF($3, '')::uuid, regency_id = NULLIF($4, '')::uuid,
		needed_date = NULLIF($5, '')::date, category = $6, description = $7, total_amount = $8, payee_name = $9, bank_name = $10,
		account_number = $11, account_name = $12, invoice_number = $13, settlement_due = NULLIF($14, '')::date, updated_at = now()
		WHERE id = $1::uuid`, id, input.Type, input.TeamID, input.RegencyID, input.NeededDate, input.Category, input.Description, total,
		input.PayeeName, input.BankName, input.AccountNumber, input.AccountName, input.InvoiceNumber, input.SettlementDue); err != nil {
		return Request{}, fmt.Errorf("save finance draft: %w", err)
	}
	orphans, err := replaceItems(ctx, tx, id, "request", input.Items)
	if err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("commit finance draft: %w", err)
	}
	s.removeFiles(orphans)
	return s.Get(ctx, actor, access, id)
}

// checkTeam: a draft may have no team yet; a chosen team must be led by the
// user, and its regency must belong to the team.
func (s *Service) checkTeam(ctx context.Context, q querier, userID, teamID, regencyID string) error {
	if teamID == "" {
		if regencyID != "" {
			return ErrInvalidInput
		}
		return nil
	}
	leader, err := isMember(ctx, q, teamID, userID, "leader")
	if err != nil {
		return err
	}
	if !leader {
		return ErrNotLeader
	}
	if regencyID == "" {
		return nil
	}
	var found bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM finance_team_regencies WHERE team_id = $1::uuid AND regency_id = $2::uuid)`, teamID, regencyID).Scan(&found); err != nil {
		return fmt.Errorf("check team regency: %w", err)
	}
	if !found {
		return ErrInvalidInput
	}
	return nil
}

// replaceItems stores the rows of a phase. Proofs of a removed row go too;
// their storage keys come back to delete after the commit.
func replaceItems(ctx context.Context, tx pgx.Tx, id, phase string, items []Item) ([]string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM finance_request_items WHERE request_id = $1::uuid AND phase = $2`, id, phase); err != nil {
		return nil, fmt.Errorf("replace finance items: %w", err)
	}
	keys := make([]string, 0, len(items))
	for position, item := range items {
		keys = append(keys, item.Key)
		if _, err := tx.Exec(ctx, `INSERT INTO finance_request_items (request_id, phase, position, item_key, entry, note, project_code, description, quantity, unit_price, amount)
			VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11)`, id, phase, position, item.Key, item.Entry, item.Note, item.ProjectCode, item.Description, item.Quantity, item.UnitPrice, item.Amount); err != nil {
			return nil, fmt.Errorf("replace finance items: %w", err)
		}
	}
	rows, err := tx.Query(ctx, `DELETE FROM finance_request_attachments a WHERE a.request_id = $1::uuid AND a.item_key IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM finance_request_items i WHERE i.request_id = a.request_id AND i.item_key = a.item_key) RETURNING a.storage_key`, id)
	if err != nil {
		return nil, fmt.Errorf("clean finance proofs: %w", err)
	}
	orphans, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("clean finance proofs: %w", err)
	}
	return orphans, nil
}

func (s *Service) removeFiles(keys []string) {
	for _, key := range keys {
		s.removeFile(key)
	}
}

// Delete removes a draft that was never sent, with its files.
func (s *Service) Delete(ctx context.Context, actor auth.Principal, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finance delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := loadRequest(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if r.RequesterID != actor.UserID {
		return ErrNotFound
	}
	if r.Status != StatusDraft {
		return ErrInvalidStatus
	}
	rows, err := tx.Query(ctx, `SELECT storage_key FROM finance_request_attachments WHERE request_id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("list finance files: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("list finance files: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM finance_requests WHERE id = $1::uuid`, id); err != nil {
		return fmt.Errorf("delete finance draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finance delete: %w", err)
	}
	for _, key := range keys {
		s.removeFile(key)
	}
	return nil
}

func (s *Service) removeFile(key string) {
	if s.storage == nil {
		return
	}
	if err := s.storage.Delete(context.Background(), key); err != nil {
		log.Printf("finance: delete file %s: %v", key, err)
	}
}

// Act runs one workflow action. Only the leader who asked runs the requester
// actions; only the team's finance runs the others, never on their own
// request.
func (s *Service) Act(ctx context.Context, actor auth.Principal, access Access, id, action, note string, meta auth.ClientMeta) (Request, error) {
	note = strings.TrimSpace(note)
	if len(note) > maxLongText {
		return Request{}, ErrInvalidInput
	}
	if noteRequired(action) && note == "" {
		return Request{}, ErrNoteRequired
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("begin finance action: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := loadRequest(ctx, tx, id, true)
	if err != nil {
		return Request{}, err
	}
	requester, handler, err := s.relation(ctx, tx, r, actor, access)
	if err != nil {
		return Request{}, err
	}
	if requesterAction(action) && !requester {
		return Request{}, ErrForbidden
	}
	if !requesterAction(action) {
		if requester {
			return Request{}, ErrSelfReview
		}
		if !handler {
			return Request{}, ErrForbidden
		}
	}
	next, ok := nextStatus(r.Type, r.Status, action)
	if !ok {
		return Request{}, ErrInvalidStatus
	}
	if err := s.applyAction(ctx, tx, &r, actor, action, next, note); err != nil {
		return Request{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_request_events (request_id, action, from_status, to_status, note, actor_id) VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid)`,
		id, action, r.Status, next, note, actor.UserID); err != nil {
		return Request{}, fmt.Errorf("record finance event: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance." + action, ResourceType: "finance_request", ResourceID: id,
		Metadata: map[string]any{"number": r.Number, "from_status": r.Status, "to_status": next}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Request{}, err
	}
	if err := s.notify(ctx, tx, r, action); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("commit finance action: %w", err)
	}
	s.wake()
	return s.Get(ctx, actor, access, id)
}

func (s *Service) wake() {
	if s.notifier != nil {
		s.notifier.Wake()
	}
}

// applyAction checks what the action needs and writes its columns.
func (s *Service) applyAction(ctx context.Context, tx pgx.Tx, r *Request, actor auth.Principal, action, next, note string) error {
	var query string
	args := []any{r.ID, next, actor.UserID, note}
	switch action {
	case ActionSubmit:
		if err := s.prepareSubmit(ctx, tx, r); err != nil {
			return err
		}
		query = `UPDATE finance_requests SET status = $2, number = $3, submitted_at = now(), review_note = '', updated_at = now() WHERE id = $1::uuid`
		args = []any{r.ID, next, r.Number}
	case ActionCancel:
		query = `UPDATE finance_requests SET status = $2, updated_at = now() WHERE id = $1::uuid`
		args = []any{r.ID, next}
	case ActionApprove, ActionReject, ActionRevision:
		query = `UPDATE finance_requests SET status = $2, reviewed_by = $3::uuid, reviewed_at = now(), review_note = $4, updated_at = now() WHERE id = $1::uuid`
	case ActionPay:
		count, err := attachmentCount(ctx, tx, r.ID, KindPayment)
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrProofRequired
		}
		query = `UPDATE finance_requests SET status = $2, paid_by = $3::uuid, paid_at = now(), review_note = $4, updated_at = now(),
			done_at = CASE WHEN $2::text = 'done' THEN now() END WHERE id = $1::uuid`
	case ActionSettle:
		items, err := loadItems(ctx, tx, r.ID, "settlement")
		if err != nil {
			return err
		}
		count, err := settlementProofs(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		if !completeItems(items) || count == 0 {
			return ErrIncomplete
		}
		query = `UPDATE finance_requests SET status = $2, review_note = '', updated_at = now() WHERE id = $1::uuid`
		args = []any{r.ID, next}
	case ActionSettleApprove:
		query = `UPDATE finance_requests SET status = $2, reviewed_by = $3::uuid, reviewed_at = now(), review_note = $4, done_at = now(), updated_at = now() WHERE id = $1::uuid`
	case ActionSettleRevision:
		query = `UPDATE finance_requests SET status = $2, reviewed_by = $3::uuid, reviewed_at = now(), review_note = $4, updated_at = now() WHERE id = $1::uuid`
	default:
		return ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update finance request: %w", err)
	}
	return nil
}

// prepareSubmit validates the request and gives it its number at the first
// submit; a revision keeps it.
func (s *Service) prepareSubmit(ctx context.Context, tx pgx.Tx, r *Request) error {
	items, err := loadItems(ctx, tx, r.ID, "request")
	if err != nil {
		return err
	}
	r.Items = items
	if err := checkComplete(*r); err != nil {
		return err
	}
	if err := s.checkTeam(ctx, tx, r.RequesterID, r.TeamID, r.RegencyID); err != nil {
		return err
	}
	handlers, err := teamHandlers(ctx, tx, r.TeamID)
	if err != nil {
		return err
	}
	if len(handlers) == 0 {
		return ErrNoHandler
	}
	if r.Number != "" {
		return nil
	}
	period := time.Now().In(jakarta).Format("200601")
	var value int
	if err := tx.QueryRow(ctx, `INSERT INTO finance_request_counters (period, last_value) VALUES ($1, 1)
		ON CONFLICT (period) DO UPDATE SET last_value = finance_request_counters.last_value + 1 RETURNING last_value`, period).Scan(&value); err != nil {
		return fmt.Errorf("next finance number: %w", err)
	}
	r.Number = fmt.Sprintf("%s-%s-%04d", numberPrefix, period, value)
	return nil
}

// settlementProofs counts the nota of a kasbon report, by row or as a whole.
func settlementProofs(ctx context.Context, q querier, id string) (int, error) {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM finance_request_attachments WHERE request_id = $1::uuid AND kind IN ('settlement', 'item_transaction', 'item_payment')`, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count kasbon proofs: %w", err)
	}
	return count, nil
}

func attachmentCount(ctx context.Context, q querier, id, kind string) (int, error) {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM finance_request_attachments WHERE request_id = $1::uuid AND kind = $2`, id, kind).Scan(&count); err != nil {
		return 0, fmt.Errorf("count finance attachments: %w", err)
	}
	return count, nil
}

func teamHandlers(ctx context.Context, q querier, teamID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT m.user_id::text FROM finance_team_members m JOIN users u ON u.id = m.user_id
		WHERE m.team_id = $1::uuid AND m.member_role = 'finance' AND u.is_active`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team finance: %w", err)
	}
	result, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list team finance: %w", err)
	}
	return result, nil
}

// notifyMessage is what an action tells whom. Nominal and account details stay
// out of it: a push shows on the lock screen.
func notifyMessage(r Request, action string) (notifications.Message, bool) {
	label := typeLabel(r.Type)
	message := notifications.Message{Link: financeLinkPrefix + r.ID}
	switch action {
	case ActionSubmit:
		message.Category, message.Title, message.Body = notifications.CategoryReview, "Pengajuan perlu direview", fmt.Sprintf("%s %s dari %s", label, r.Number, r.RequesterName)
	case ActionCancel:
		message.Category, message.Title, message.Body = notifications.CategoryReview, "Pengajuan dibatalkan", fmt.Sprintf("%s %s dibatalkan %s", label, r.Number, r.RequesterName)
	case ActionSettle:
		message.Category, message.Title, message.Body = notifications.CategoryReview, "Laporan kasbon perlu direview", fmt.Sprintf("%s dari %s", r.Number, r.RequesterName)
	case ActionApprove:
		message.Category, message.Title, message.Body = notifications.CategoryResult, "Pengajuan disetujui", fmt.Sprintf("%s %s disetujui", label, r.Number)
	case ActionReject:
		message.Category, message.Title, message.Body = notifications.CategoryResult, "Pengajuan ditolak", fmt.Sprintf("%s %s ditolak. Buka untuk melihat alasannya.", label, r.Number)
	case ActionRevision:
		message.Category, message.Title, message.Body = notifications.CategoryResult, "Pengajuan perlu revisi", fmt.Sprintf("%s %s perlu diperbaiki", label, r.Number)
	case ActionPay:
		message.Category, message.Title, message.Body = notifications.CategoryPayment, "Pengajuan sudah dibayar", fmt.Sprintf("%s %s sudah dibayar", label, r.Number)
	case ActionSettleApprove:
		message.Category, message.Title, message.Body = notifications.CategoryResult, "Laporan kasbon diterima", fmt.Sprintf("Kasbon %s selesai", r.Number)
	case ActionSettleRevision:
		message.Category, message.Title, message.Body = notifications.CategoryResult, "Laporan kasbon perlu revisi", fmt.Sprintf("Kasbon %s perlu diperbaiki", r.Number)
	default:
		return notifications.Message{}, false
	}
	return message, true
}

func (s *Service) notify(ctx context.Context, tx pgx.Tx, r Request, action string) error {
	message, ok := notifyMessage(r, action)
	if !ok {
		return nil
	}
	recipients := []string{r.RequesterID}
	if requesterAction(action) {
		var err error
		if recipients, err = teamHandlers(ctx, tx, r.TeamID); err != nil {
			return err
		}
	}
	return notifications.Enqueue(ctx, tx, recipients, message)
}

// SaveSettlement stores the kasbon's realization while it can still change.
func (s *Service) SaveSettlement(ctx context.Context, actor auth.Principal, access Access, id string, input SettlementInput) (Request, error) {
	items, total, err := normalizeItems(input.Items)
	if err != nil {
		return Request{}, err
	}
	note := strings.TrimSpace(input.Note)
	if len(note) > maxLongText {
		return Request{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("begin finance settlement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := loadRequest(ctx, tx, id, true)
	if err != nil {
		return Request{}, err
	}
	if r.RequesterID != actor.UserID {
		return Request{}, ErrNotFound
	}
	if r.Type != TypeKasbon || !settlementEditable(r.Status) {
		return Request{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `UPDATE finance_requests SET settlement_amount = $2, settlement_note = $3, updated_at = now() WHERE id = $1::uuid`, id, total, note); err != nil {
		return Request{}, fmt.Errorf("save finance settlement: %w", err)
	}
	orphans, err := replaceItems(ctx, tx, id, "settlement", items)
	if err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("commit finance settlement: %w", err)
	}
	s.removeFiles(orphans)
	return s.Get(ctx, actor, access, id)
}

// AttachmentContent is one stored file.
type AttachmentContent struct {
	Reader      io.ReadCloser
	Filename    string
	ContentType string
}

// attachmentExtensions are the file types accepted, by sniffed content.
var attachmentExtensions = map[string]string{"image/jpeg": "jpg", "image/png": "png", "application/pdf": "pdf"}

// AddAttachment stores a nota or payment proof. The file's type comes from
// its content, not from what the client claims.
func (s *Service) AddAttachment(ctx context.Context, actor auth.Principal, access Access, id, kind, itemKey, originalName string, source io.Reader) (Attachment, error) {
	if s.storage == nil {
		return Attachment{}, errMissingStorage
	}
	data, err := io.ReadAll(io.LimitReader(source, MaxAttachmentBytes+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("read finance attachment: %w", err)
	}
	if len(data) == 0 {
		return Attachment{}, ErrAttachmentType
	}
	if len(data) > MaxAttachmentBytes {
		return Attachment{}, ErrAttachmentSize
	}
	contentType := http.DetectContentType(data)
	extension, ok := attachmentExtensions[contentType]
	if !ok {
		return Attachment{}, ErrAttachmentType
	}
	if contentType != "application/pdf" {
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return Attachment{}, ErrAttachmentType
		}
	}
	r, err := s.checkUpload(ctx, s.pool, actor, access, id, kind)
	if err != nil {
		return Attachment{}, err
	}
	if err := checkItemKey(ctx, s.pool, id, kind, itemKey); err != nil {
		return Attachment{}, err
	}
	originalName = strings.TrimSpace(originalName)
	if len(originalName) > maxShortText {
		originalName = originalName[:maxShortText]
	}
	count, err := attachmentCount(ctx, s.pool, id, kind)
	if err != nil {
		return Attachment{}, err
	}
	if (itemKey == "" && count >= maxAttachments) || count >= maxRowAttachments {
		return Attachment{}, ErrTooManyFiles
	}
	attachmentID := uuid.NewString()
	filename := fmt.Sprintf("%s-%s-%s.%s", attachmentName(r), kind, attachmentID[:8], extension)
	folder := []string{"KEUANGAN", time.Now().In(jakarta).Format("2006-01")}
	key, size, _, err := media.PutNamed(ctx, s.storage, attachmentID, filename, folder, bytes.NewReader(data))
	if err != nil {
		return Attachment{}, fmt.Errorf("store finance attachment: %w", err)
	}
	attachment := Attachment{ID: attachmentID, Kind: kind, ItemKey: itemKey, Filename: filename, ContentType: contentType, SizeBytes: size, CreatedAt: time.Now()}
	if originalName != "" {
		attachment.Filename = originalName
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO finance_request_attachments (id, request_id, kind, storage_key, filename, content_type, size_bytes, uploaded_by, item_key, original_name)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8::uuid, NULLIF($9, '')::uuid, $10)`, attachmentID, id, kind, key, filename, contentType, size, actor.UserID, itemKey, originalName); err != nil {
		s.removeFile(key)
		return Attachment{}, fmt.Errorf("save finance attachment: %w", err)
	}
	return attachment, nil
}

// checkItemKey: a row proof names a row that is saved; other files name none.
func checkItemKey(ctx context.Context, q querier, id, kind, itemKey string) error {
	rowKind := kind == KindItemTransaction || kind == KindItemPayment
	if rowKind != (itemKey != "") {
		return ErrInvalidInput
	}
	if !rowKind {
		return nil
	}
	if _, err := uuid.Parse(itemKey); err != nil {
		return ErrInvalidInput
	}
	var found bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM finance_request_items WHERE request_id = $1::uuid AND item_key = $2::uuid)`, id, itemKey).Scan(&found); err != nil {
		return fmt.Errorf("check finance row: %w", err)
	}
	if !found {
		return ErrInvalidInput
	}
	return nil
}

func attachmentName(r Request) string {
	if r.Number != "" {
		return r.Number
	}
	return "DRAF-" + r.ID[:8]
}

func (s *Service) checkUpload(ctx context.Context, q querier, actor auth.Principal, access Access, id, kind string) (Request, error) {
	r, err := loadRequest(ctx, q, id, false)
	if err != nil {
		return Request{}, err
	}
	requester, handler, err := s.relation(ctx, q, r, actor, access)
	if err != nil {
		return Request{}, err
	}
	for _, allowed := range uploadKinds(r, requester, handler) {
		if allowed == kind {
			return r, nil
		}
	}
	return Request{}, ErrInvalidStatus
}

func (s *Service) DeleteAttachment(ctx context.Context, actor auth.Principal, access Access, id, attachmentID string) error {
	var kind, key string
	err := s.pool.QueryRow(ctx, `SELECT kind, storage_key FROM finance_request_attachments WHERE id = $1::uuid AND request_id = $2::uuid`, attachmentID, id).Scan(&kind, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load finance attachment: %w", err)
	}
	if _, err := s.checkUpload(ctx, s.pool, actor, access, id, kind); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM finance_request_attachments WHERE id = $1::uuid`, attachmentID); err != nil {
		return fmt.Errorf("delete finance attachment: %w", err)
	}
	s.removeFile(key)
	return nil
}

func (s *Service) OpenAttachment(ctx context.Context, actor auth.Principal, access Access, id, attachmentID string) (AttachmentContent, error) {
	r, err := loadRequest(ctx, s.pool, id, false)
	if err != nil {
		return AttachmentContent{}, err
	}
	if _, _, err := s.relation(ctx, s.pool, r, actor, access); err != nil {
		return AttachmentContent{}, err
	}
	var key string
	content := AttachmentContent{}
	err = s.pool.QueryRow(ctx, `SELECT storage_key, filename, content_type FROM finance_request_attachments WHERE id = $1::uuid AND request_id = $2::uuid`, attachmentID, id).
		Scan(&key, &content.Filename, &content.ContentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttachmentContent{}, ErrNotFound
	}
	if err != nil {
		return AttachmentContent{}, fmt.Errorf("load finance attachment: %w", err)
	}
	if content.Reader, err = s.storage.Open(ctx, key); err != nil {
		return AttachmentContent{}, fmt.Errorf("open finance attachment: %w", err)
	}
	return content, nil
}

// RunReminders tells the leader and the team's finance, once a day, about each
// kasbon past its report date.
func (s *Service) RunReminders(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := s.remind(ctx); err != nil && ctx.Err() == nil {
			log.Printf("finance: reminders: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) remind(ctx context.Context) error {
	today := time.Now().In(jakarta).Format(dateLayout)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finance reminders: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT r.id::text, r.number, r.requester_id::text, r.team_id::text FROM finance_requests r
		WHERE r.request_type = 'kasbon' AND r.status IN ('paid', 'settlement_revision') AND r.settlement_due < $1::date
		AND (r.reminded_on IS NULL OR r.reminded_on < $1::date) FOR UPDATE`, today)
	if err != nil {
		return fmt.Errorf("list overdue kasbon: %w", err)
	}
	type overdue struct{ id, number, requester, team string }
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (overdue, error) {
		var o overdue
		return o, row.Scan(&o.id, &o.number, &o.requester, &o.team)
	})
	if err != nil {
		return fmt.Errorf("list overdue kasbon: %w", err)
	}
	for _, item := range due {
		handlers, err := teamHandlers(ctx, tx, item.team)
		if err != nil {
			return err
		}
		message := notifications.Message{Category: notifications.CategoryReminder, Title: "Laporan kasbon terlambat",
			Body: fmt.Sprintf("Kasbon %s belum dilaporkan", item.number), Link: financeLinkPrefix + item.id}
		if err := notifications.Enqueue(ctx, tx, append(handlers, item.requester), message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE finance_requests SET reminded_on = $2::date WHERE id = $1::uuid`, item.id, today); err != nil {
			return fmt.Errorf("mark kasbon reminder: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finance reminders: %w", err)
	}
	if len(due) > 0 {
		s.wake()
	}
	return nil
}
