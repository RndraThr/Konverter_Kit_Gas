package health

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
)

type Component struct {
	Status    string `json:"status"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

type Report struct {
	Status           string    `json:"status"`
	Database         Component `json:"database"`
	MigrationVersion int64     `json:"migration_version"`
	Environment      string    `json:"environment"`
	Version          string    `json:"version"`
	UptimeSeconds    int64     `json:"uptime_seconds"`
	CheckedAt        time.Time `json:"checked_at"`
}

type Probe interface {
	Ping(context.Context) error
	MigrationVersion(context.Context) (int64, error)
}

type Service struct {
	probe     Probe
	env       string
	version   string
	startedAt time.Time
	now       func() time.Time
}

func NewService(probe Probe, environment, version string, startedAt time.Time) *Service {
	return &Service{probe: probe, env: environment, version: version, startedAt: startedAt, now: time.Now}
}

func (s *Service) Check(ctx context.Context) Report {
	checkedAt := s.now().UTC()
	uptime := int64(checkedAt.Sub(s.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}
	report := Report{
		Status:        StatusHealthy,
		Environment:   s.env,
		Version:       s.version,
		UptimeSeconds: uptime,
		CheckedAt:     checkedAt,
		Database:      Component{Status: StatusHealthy, Message: "PostgreSQL siap"},
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	started := time.Now()
	if err := s.probe.Ping(checkCtx); err != nil {
		report.Status = StatusUnhealthy
		report.Database = Component{
			Status:    StatusUnhealthy,
			Code:      "database_unavailable",
			Message:   "PostgreSQL tidak dapat dijangkau",
			LatencyMS: time.Since(started).Milliseconds(),
		}
		return report
	}
	report.Database.LatencyMS = time.Since(started).Milliseconds()

	version, err := s.probe.MigrationVersion(checkCtx)
	if err != nil {
		report.Status = StatusDegraded
		report.Database.Status = StatusDegraded
		report.Database.Code = "migration_state_unavailable"
		report.Database.Message = "PostgreSQL siap, versi migration tidak dapat dibaca"
		return report
	}
	report.MigrationVersion = version
	return report
}

type PostgresProbe struct {
	pool *pgxpool.Pool
}

func NewPostgresProbe(pool *pgxpool.Pool) *PostgresProbe {
	return &PostgresProbe{pool: pool}
}

func (p *PostgresProbe) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PostgresProbe) MigrationVersion(ctx context.Context) (int64, error) {
	var version int64
	if err := p.pool.QueryRow(ctx, `SELECT COALESCE(max(version_id), 0) FROM goose_db_version WHERE is_applied = true`).Scan(&version); err != nil {
		return 0, fmt.Errorf("query migration version: %w", err)
	}
	return version, nil
}
