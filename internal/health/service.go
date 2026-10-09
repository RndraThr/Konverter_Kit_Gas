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

type OperationalStats struct {
	Queued     int64 `json:"queued"`
	Processing int64 `json:"processing"`
	Retry      int64 `json:"retry"`
	Failed     int64 `json:"failed"`
}

type Report struct {
	Status            string           `json:"status"`
	Database          Component        `json:"database"`
	Operations        Component        `json:"operations"`
	MigrationVersion  int64            `json:"migration_version"`
	Environment       string           `json:"environment"`
	Version           string           `json:"version"`
	StorageBackend    string           `json:"storage_backend"`
	MediaWorkerStatus string           `json:"media_worker_status"`
	MediaMoves        OperationalStats `json:"media_moves"`
	UptimeSeconds     int64            `json:"uptime_seconds"`
	CheckedAt         time.Time        `json:"checked_at"`
}

type Probe interface {
	Ping(context.Context) error
	MigrationVersion(context.Context) (int64, error)
	OperationalStats(context.Context) (OperationalStats, error)
}

type Service struct {
	probe     Probe
	env       string
	version   string
	storage   string
	worker    bool
	startedAt time.Time
	now       func() time.Time
}

func NewService(probe Probe, environment, version, storageBackend string, mediaWorkerActive bool, startedAt time.Time) *Service {
	return &Service{
		probe: probe, env: environment, version: version, storage: storageBackend,
		worker: mediaWorkerActive, startedAt: startedAt, now: time.Now,
	}
}

func (s *Service) Check(ctx context.Context) Report {
	checkedAt := s.now().UTC()
	uptime := int64(checkedAt.Sub(s.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}
	report := Report{
		Status:            StatusHealthy,
		Environment:       s.env,
		Version:           s.version,
		StorageBackend:    s.storage,
		MediaWorkerStatus: "not_applicable",
		UptimeSeconds:     uptime,
		CheckedAt:         checkedAt,
		Database:          Component{Status: StatusHealthy, Message: "PostgreSQL siap"},
		Operations:        Component{Status: StatusHealthy, Message: "Statistik operasional siap"},
	}
	if s.worker {
		report.MediaWorkerStatus = "active"
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
	stats, err := s.probe.OperationalStats(checkCtx)
	if err != nil {
		report.Status = StatusDegraded
		report.Operations = Component{
			Status:  StatusDegraded,
			Code:    "operational_stats_unavailable",
			Message: "Statistik operasional tidak dapat dibaca",
		}
		return report
	}
	report.MediaMoves = stats
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

func (p *PostgresProbe) OperationalStats(ctx context.Context) (OperationalStats, error) {
	var stats OperationalStats
	err := p.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'queued'),
			count(*) FILTER (WHERE status = 'processing'),
			count(*) FILTER (WHERE status = 'retry'),
			(SELECT count(*) FROM media_files WHERE storage_state = 'move_failed')
		FROM distribution_media_move_jobs
	`).Scan(&stats.Queued, &stats.Processing, &stats.Retry, &stats.Failed)
	if err != nil {
		return OperationalStats{}, fmt.Errorf("query operational stats: %w", err)
	}
	return stats, nil
}
