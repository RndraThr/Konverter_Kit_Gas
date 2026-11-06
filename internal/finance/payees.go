package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"konkit/internal/auth"
)

// Payee is a bank account the team has paid, picked instead of typed again.
type Payee struct {
	ID            string `json:"id"`
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
}

func (p *Payee) normalize() error {
	p.AccountName = strings.Join(strings.Fields(p.AccountName), " ")
	p.BankName = strings.ToUpper(strings.Join(strings.Fields(p.BankName), " "))
	p.AccountNumber = strings.NewReplacer(" ", "", "-", "").Replace(p.AccountNumber)
	if p.AccountName == "" || p.BankName == "" || len(p.AccountName) > maxShortText || len(p.BankName) > maxShortText || !validAccountNumber(p.AccountNumber) {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) checkLeader(ctx context.Context, actor auth.Principal, access Access, teamID string) error {
	if _, err := uuid.Parse(teamID); err != nil {
		return ErrInvalidInput
	}
	leader, err := isMember(ctx, s.pool, teamID, actor.UserID, "leader")
	if err != nil {
		return err
	}
	if !access.CanRequest || !leader {
		return ErrNotLeader
	}
	return nil
}

// Payees lists the team's saved accounts, the latest first.
func (s *Service) Payees(ctx context.Context, actor auth.Principal, access Access, teamID string) ([]Payee, error) {
	if err := s.checkLeader(ctx, actor, access, teamID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, account_name, bank_name, account_number FROM finance_payees WHERE team_id = $1::uuid ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list finance payees: %w", err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Payee, error) {
		var p Payee
		return p, row.Scan(&p.ID, &p.AccountName, &p.BankName, &p.AccountNumber)
	})
	if err != nil {
		return nil, fmt.Errorf("list finance payees: %w", err)
	}
	return result, nil
}

// SavePayee adds an account. The same bank and number again only renames it,
// so a typing slip in the name can be fixed by adding it once more.
func (s *Service) SavePayee(ctx context.Context, actor auth.Principal, access Access, teamID string, input Payee) (Payee, error) {
	if err := s.checkLeader(ctx, actor, access, teamID); err != nil {
		return Payee{}, err
	}
	if err := input.normalize(); err != nil {
		return Payee{}, err
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO finance_payees (team_id, account_name, bank_name, account_number, created_by) VALUES ($1::uuid, $2, $3, $4, $5::uuid)
		ON CONFLICT (team_id, bank_name, account_number) DO UPDATE SET account_name = EXCLUDED.account_name RETURNING id::text`,
		teamID, input.AccountName, input.BankName, input.AccountNumber, actor.UserID).Scan(&input.ID)
	if err != nil {
		return Payee{}, fmt.Errorf("save finance payee: %w", err)
	}
	return input, nil
}

// DeletePayee takes an account off the list; requests that used it keep theirs.
func (s *Service) DeletePayee(ctx context.Context, actor auth.Principal, access Access, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrNotFound
	}
	var teamID string
	err := s.pool.QueryRow(ctx, `SELECT team_id::text FROM finance_payees WHERE id = $1::uuid`, id).Scan(&teamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load finance payee: %w", err)
	}
	if err := s.checkLeader(ctx, actor, access, teamID); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM finance_payees WHERE id = $1::uuid`, id); err != nil {
		return fmt.Errorf("delete finance payee: %w", err)
	}
	return nil
}
