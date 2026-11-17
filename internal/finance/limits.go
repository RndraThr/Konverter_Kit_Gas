package finance

import (
	"context"
	"errors"
	"fmt"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrRegencyRequired = errors.New("finance request needs a regency")
	ErrKasbonLimit     = errors.New("kasbon is above the regency's remaining limit")
	ErrPettyCashLimit  = errors.New("request is above the regency's remaining petty cash")
)

// Requests that still hold money: a kasbon until its report is accepted, a
// cashless on petty cash until finance has paid it back.
const (
	kasbonHeld    = `('submitted', 'revision', 'approved', 'paid', 'settlement_submitted', 'settlement_revision')`
	pettyCashHeld = `('submitted', 'revision', 'approved')`
)

const limitUsage = `SELECT regency_id,
	COALESCE(sum(total_amount) FILTER (WHERE request_type = 'kasbon' AND status IN ` + kasbonHeld + `), 0) AS kasbon,
	COALESCE(sum(total_amount) FILTER (WHERE request_type = 'cashless' AND use_petty_cash AND status IN ` + pettyCashHeld + `), 0) AS petty_cash
	FROM finance_requests WHERE regency_id IS NOT NULL GROUP BY regency_id`

// RegencyLimit is what finance sets and sees. Leaders never get it.
type RegencyLimit struct {
	RegencyID          string `json:"regency_id"`
	RegencyName        string `json:"regency_name"`
	KasbonLimit        int64  `json:"kasbon_limit"`
	KasbonUsed         int64  `json:"kasbon_used"`
	KasbonAvailable    int64  `json:"kasbon_available"`
	PettyCashLimit     int64  `json:"petty_cash_limit"`
	PettyCashUsed      int64  `json:"petty_cash_used"`
	PettyCashAvailable int64  `json:"petty_cash_available"`
}

// RegencyBalance is all a leader sees: what is left, never the limit behind it.
type RegencyBalance struct {
	RegencyID          string `json:"regency_id"`
	KasbonLimited      bool   `json:"kasbon_limited"`
	KasbonAvailable    int64  `json:"kasbon_available"`
	PettyCashAvailable int64  `json:"petty_cash_available"`
}

// LimitInput is the two limits of one regency.
type LimitInput struct {
	KasbonLimit    int64 `json:"kasbon_limit"`
	PettyCashLimit int64 `json:"petty_cash_limit"`
}

func remaining(limit, used int64) int64 {
	return max(limit-used, 0)
}

// regencyLimits lists the regencies of the teams the user holds this role in,
// or of every active team when role is empty.
func (s *Service) regencyLimits(ctx context.Context, userID, role string) ([]RegencyLimit, error) {
	rows, err := s.pool.Query(ctx, `WITH scope AS (
			SELECT DISTINCT tr.regency_id FROM finance_team_regencies tr JOIN finance_teams t ON t.id = tr.team_id
			WHERE t.is_active AND ($2 = '' OR EXISTS (SELECT 1 FROM finance_team_members m WHERE m.team_id = t.id AND m.user_id = $1::uuid AND m.member_role = $2))
		)
		SELECT g.id::text, g.name, COALESCE(l.kasbon_limit, 0), COALESCE(u.kasbon, 0), COALESCE(l.petty_cash_limit, 0), COALESCE(u.petty_cash, 0)
		FROM scope JOIN regencies g ON g.id = scope.regency_id
		LEFT JOIN finance_limits l ON l.regency_id = g.id
		LEFT JOIN (`+limitUsage+`) u ON u.regency_id = g.id
		ORDER BY g.name`, userID, role)
	if err != nil {
		return nil, fmt.Errorf("list finance limits: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RegencyLimit, error) {
		var l RegencyLimit
		err := row.Scan(&l.RegencyID, &l.RegencyName, &l.KasbonLimit, &l.KasbonUsed, &l.PettyCashLimit, &l.PettyCashUsed)
		l.KasbonAvailable = remaining(l.KasbonLimit, l.KasbonUsed)
		l.PettyCashAvailable = remaining(l.PettyCashLimit, l.PettyCashUsed)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("list finance limits: %w", err)
	}
	return result, nil
}

// Limits is the finance view of the limits it may set.
func (s *Service) Limits(ctx context.Context, actor auth.Principal, access Access) ([]RegencyLimit, error) {
	switch {
	case access.CanManage:
		return s.regencyLimits(ctx, actor.UserID, "")
	case access.CanReview:
		return s.regencyLimits(ctx, actor.UserID, "finance")
	}
	return nil, ErrForbidden
}

// Balances is the leader view: the remainder of each regency they work in.
func (s *Service) Balances(ctx context.Context, actor auth.Principal, access Access) ([]RegencyBalance, error) {
	if !access.CanRequest {
		return nil, ErrForbidden
	}
	limits, err := s.regencyLimits(ctx, actor.UserID, "leader")
	if err != nil {
		return nil, err
	}
	result := make([]RegencyBalance, len(limits))
	for index, limit := range limits {
		result[index] = RegencyBalance{RegencyID: limit.RegencyID, KasbonLimited: limit.KasbonLimit > 0, KasbonAvailable: limit.KasbonAvailable, PettyCashAvailable: limit.PettyCashAvailable}
	}
	return result, nil
}

// SetLimit replaces the limits of a regency the user may manage. A limit below
// what is already held only leaves nothing available.
func (s *Service) SetLimit(ctx context.Context, actor auth.Principal, access Access, regencyID string, input LimitInput, meta auth.ClientMeta) error {
	if _, err := uuid.Parse(regencyID); err != nil {
		return ErrNotFound
	}
	if input.KasbonLimit < 0 || input.PettyCashLimit < 0 || input.KasbonLimit > maxLineAmount || input.PettyCashLimit > maxLineAmount {
		return ErrInvalidInput
	}
	limits, err := s.Limits(ctx, actor, access)
	if err != nil {
		return err
	}
	allowed := false
	for _, limit := range limits {
		allowed = allowed || limit.RegencyID == regencyID
	}
	if !allowed {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finance limit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO finance_limits (regency_id, kasbon_limit, petty_cash_limit, updated_by) VALUES ($1::uuid, $2, $3, $4::uuid)
		ON CONFLICT (regency_id) DO UPDATE SET kasbon_limit = EXCLUDED.kasbon_limit, petty_cash_limit = EXCLUDED.petty_cash_limit, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		regencyID, input.KasbonLimit, input.PettyCashLimit, actor.UserID); err != nil {
		return fmt.Errorf("save finance limit: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance.limit_saved", ResourceType: "finance_limit", ResourceID: regencyID,
		Metadata: map[string]any{"kasbon_limit": input.KasbonLimit, "petty_cash_limit": input.PettyCashLimit}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finance limit: %w", err)
	}
	return nil
}

// checkLimits runs inside the submit. The limit row stays locked until the
// commit, so two requests cannot both take the last of a balance.
func checkLimits(ctx context.Context, tx pgx.Tx, r Request) error {
	if r.RegencyID == "" {
		var regencies int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM finance_team_regencies WHERE team_id = $1::uuid`, r.TeamID).Scan(&regencies); err != nil {
			return fmt.Errorf("count team regencies: %w", err)
		}
		if regencies > 0 || r.UsePettyCash {
			return ErrRegencyRequired
		}
		return nil
	}
	if r.Type != TypeKasbon && !r.UsePettyCash {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_limits (regency_id) VALUES ($1::uuid) ON CONFLICT (regency_id) DO NOTHING`, r.RegencyID); err != nil {
		return fmt.Errorf("prepare finance limit: %w", err)
	}
	var kasbonLimit, pettyCashLimit, kasbonUsed, pettyCashUsed int64
	if err := tx.QueryRow(ctx, `SELECT kasbon_limit, petty_cash_limit FROM finance_limits WHERE regency_id = $1::uuid FOR UPDATE`, r.RegencyID).Scan(&kasbonLimit, &pettyCashLimit); err != nil {
		return fmt.Errorf("lock finance limit: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(total_amount) FILTER (WHERE request_type = 'kasbon' AND status IN `+kasbonHeld+`), 0),
		COALESCE(sum(total_amount) FILTER (WHERE request_type = 'cashless' AND use_petty_cash AND status IN `+pettyCashHeld+`), 0)
		FROM finance_requests WHERE regency_id = $1::uuid AND id <> $2::uuid`, r.RegencyID, r.ID).Scan(&kasbonUsed, &pettyCashUsed); err != nil {
		return fmt.Errorf("sum finance limit: %w", err)
	}
	if r.Type == TypeKasbon && kasbonLimit > 0 && r.TotalAmount > remaining(kasbonLimit, kasbonUsed) {
		return ErrKasbonLimit
	}
	if r.UsePettyCash && r.TotalAmount > remaining(pettyCashLimit, pettyCashUsed) {
		return ErrPettyCashLimit
	}
	return nil
}
