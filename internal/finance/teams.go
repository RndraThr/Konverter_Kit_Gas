package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Team struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	IsActive  bool     `json:"is_active"`
	Leaders   []Option `json:"leaders"`
	Finance   []Option `json:"finance"`
	Regencies []Option `json:"regencies"`
}

type TeamInput struct {
	Name       string   `json:"name"`
	IsActive   bool     `json:"is_active"`
	LeaderIDs  []string `json:"leader_ids"`
	FinanceIDs []string `json:"finance_ids"`
	RegencyIDs []string `json:"regency_ids"`
}

type TeamOptions struct {
	Leaders   []Option `json:"leaders"`
	Finance   []Option `json:"finance"`
	Regencies []Option `json:"regencies"`
}

// Profile is what a client needs to choose its screens and fill the form.
type Profile struct {
	CanRequest bool     `json:"can_request"`
	CanReview  bool     `json:"can_review"`
	CanMonitor bool     `json:"can_monitor"`
	CanManage  bool     `json:"can_manage"`
	Categories []string `json:"categories"`
	// LeaderTeams are the teams the user can ask for; FinanceTeams those they handle.
	LeaderTeams  []Team `json:"leader_teams"`
	FinanceTeams []Team `json:"finance_teams"`
}

func (s *Service) Profile(ctx context.Context, actor auth.Principal, access Access) (Profile, error) {
	profile := Profile{CanRequest: access.CanRequest, CanReview: access.CanReview, CanMonitor: access.CanMonitor, CanManage: access.CanManage,
		Categories: Categories, LeaderTeams: []Team{}, FinanceTeams: []Team{}}
	teams, err := s.Teams(ctx)
	if err != nil {
		return Profile{}, err
	}
	for _, team := range teams {
		if !team.IsActive {
			continue
		}
		if access.CanRequest && hasOption(team.Leaders, actor.UserID) {
			profile.LeaderTeams = append(profile.LeaderTeams, team)
		}
		if access.CanReview && hasOption(team.Finance, actor.UserID) {
			profile.FinanceTeams = append(profile.FinanceTeams, team)
		}
	}
	return profile, nil
}

func hasOption(options []Option, id string) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) Teams(ctx context.Context) ([]Team, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, name, is_active FROM finance_teams ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list finance teams: %w", err)
	}
	teams, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Team, error) {
		team := Team{Leaders: []Option{}, Finance: []Option{}, Regencies: []Option{}}
		return team, row.Scan(&team.ID, &team.Name, &team.IsActive)
	})
	if err != nil {
		return nil, fmt.Errorf("list finance teams: %w", err)
	}
	byID := map[string]*Team{}
	for i := range teams {
		byID[teams[i].ID] = &teams[i]
	}
	members, err := s.pool.Query(ctx, `SELECT m.team_id::text, m.member_role, m.user_id::text, COALESCE(NULLIF(u.full_name, ''), u.username)
		FROM finance_team_members m JOIN users u ON u.id = m.user_id ORDER BY 4`)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer members.Close()
	for members.Next() {
		var teamID, role string
		var option Option
		if err := members.Scan(&teamID, &role, &option.ID, &option.Name); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		if team := byID[teamID]; team != nil && role == "leader" {
			team.Leaders = append(team.Leaders, option)
		} else if team != nil {
			team.Finance = append(team.Finance, option)
		}
	}
	if err := members.Err(); err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	regencies, err := s.pool.Query(ctx, `SELECT tr.team_id::text, g.id::text, g.name FROM finance_team_regencies tr JOIN regencies g ON g.id = tr.regency_id ORDER BY g.name`)
	if err != nil {
		return nil, fmt.Errorf("list team regencies: %w", err)
	}
	defer regencies.Close()
	for regencies.Next() {
		var teamID string
		var option Option
		if err := regencies.Scan(&teamID, &option.ID, &option.Name); err != nil {
			return nil, fmt.Errorf("scan team regency: %w", err)
		}
		if team := byID[teamID]; team != nil {
			team.Regencies = append(team.Regencies, option)
		}
	}
	return teams, regencies.Err()
}

func (s *Service) usersWith(ctx context.Context, permission string) ([]Option, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT u.id::text, COALESCE(NULLIF(u.full_name, ''), u.username) AS name
		FROM users u JOIN user_roles ur ON ur.user_id = u.id JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id WHERE u.is_active AND p.code = $1 ORDER BY name`, permission)
	if err != nil {
		return nil, fmt.Errorf("list users with %s: %w", permission, err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Option, error) {
		var option Option
		return option, row.Scan(&option.ID, &option.Name)
	})
	if err != nil {
		return nil, fmt.Errorf("list users with %s: %w", permission, err)
	}
	return result, nil
}

// TeamOptions lists who can lead, who can handle, and the regencies.
func (s *Service) TeamOptions(ctx context.Context) (TeamOptions, error) {
	leaders, err := s.usersWith(ctx, "finance.request")
	if err != nil {
		return TeamOptions{}, err
	}
	handlers, err := s.usersWith(ctx, "finance.review")
	if err != nil {
		return TeamOptions{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM regencies ORDER BY name`)
	if err != nil {
		return TeamOptions{}, fmt.Errorf("list regencies: %w", err)
	}
	regencies, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Option, error) {
		var option Option
		return option, row.Scan(&option.ID, &option.Name)
	})
	if err != nil {
		return TeamOptions{}, fmt.Errorf("list regencies: %w", err)
	}
	return TeamOptions{Leaders: leaders, Finance: handlers, Regencies: regencies}, nil
}

// SaveTeam creates a team (empty id) or replaces one. Each member must hold
// the permission of their part, so a team cannot be set up to fail later.
func (s *Service) SaveTeam(ctx context.Context, actor auth.Principal, id string, input TeamInput, meta auth.ClientMeta) (Team, error) {
	input.Name = strings.Join(strings.Fields(input.Name), " ")
	if input.Name == "" || len(input.Name) > 120 {
		return Team{}, ErrInvalidInput
	}
	options, err := s.TeamOptions(ctx)
	if err != nil {
		return Team{}, err
	}
	if !allOptions(options.Leaders, input.LeaderIDs) || !allOptions(options.Finance, input.FinanceIDs) {
		return Team{}, ErrInvalidMember
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Team{}, fmt.Errorf("begin finance team: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO finance_teams (name, is_active) VALUES ($1, $2) RETURNING id::text`, input.Name, input.IsActive).Scan(&id)
	} else {
		var tag pgconn.CommandTag
		tag, err = tx.Exec(ctx, `UPDATE finance_teams SET name = $2, is_active = $3, updated_at = now() WHERE id = $1::uuid`, id, input.Name, input.IsActive)
		if err == nil && tag.RowsAffected() == 0 {
			return Team{}, ErrNotFound
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Team{}, ErrTeamNameInUse
	}
	if err != nil {
		return Team{}, fmt.Errorf("save finance team: %w", err)
	}
	if err := replaceMembers(ctx, tx, id, input); err != nil {
		return Team{}, err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance.team_saved", ResourceType: "finance_team", ResourceID: id,
		Metadata: map[string]any{"name": input.Name, "leaders": len(input.LeaderIDs), "finance": len(input.FinanceIDs)}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return Team{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Team{}, fmt.Errorf("commit finance team: %w", err)
	}
	teams, err := s.Teams(ctx)
	if err != nil {
		return Team{}, err
	}
	for _, team := range teams {
		if team.ID == id {
			return team, nil
		}
	}
	return Team{}, ErrNotFound
}

func allOptions(options []Option, ids []string) bool {
	for _, id := range ids {
		if !hasOption(options, id) {
			return false
		}
	}
	return true
}

func replaceMembers(ctx context.Context, tx pgx.Tx, id string, input TeamInput) error {
	if _, err := tx.Exec(ctx, `DELETE FROM finance_team_members WHERE team_id = $1::uuid`, id); err != nil {
		return fmt.Errorf("replace team members: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM finance_team_regencies WHERE team_id = $1::uuid`, id); err != nil {
		return fmt.Errorf("replace team regencies: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_team_members (team_id, user_id, member_role) SELECT $1::uuid, unnest($2::uuid[]), 'leader' ON CONFLICT DO NOTHING`, id, input.LeaderIDs); err != nil {
		return fmt.Errorf("save team leaders: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_team_members (team_id, user_id, member_role) SELECT $1::uuid, unnest($2::uuid[]), 'finance' ON CONFLICT DO NOTHING`, id, input.FinanceIDs); err != nil {
		return fmt.Errorf("save team finance: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finance_team_regencies (team_id, regency_id) SELECT $1::uuid, unnest($2::uuid[]) ON CONFLICT DO NOTHING`, id, input.RegencyIDs); err != nil {
		return fmt.Errorf("save team regencies: %w", err)
	}
	return nil
}

func (s *Service) DeleteTeam(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finance team delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `DELETE FROM finance_teams WHERE id = $1::uuid`, id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrTeamInUse
	}
	if err != nil {
		return fmt.Errorf("delete finance team: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "finance.team_deleted", ResourceType: "finance_team", ResourceID: id, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
