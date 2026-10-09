package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheckReturnsHealthyReport(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	service := NewService(&fakeProbe{migrationVersion: 2}, "local", "dev", "local", false, now.Add(-90*time.Second))
	service.now = func() time.Time { return now }

	report := service.Check(context.Background())
	if report.Status != StatusHealthy || report.Database.Status != StatusHealthy {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.MigrationVersion != 2 || report.UptimeSeconds != 90 || report.CheckedAt.Location() != time.UTC {
		t.Fatalf("unexpected report details: %+v", report)
	}
}

func TestCheckIncludesStorageWorkerAndMediaMoveStats(t *testing.T) {
	probe := &fakeProbe{migrationVersion: 56, stats: OperationalStats{
		Queued: 3, Processing: 1, Retry: 2, Failed: 4,
	}}
	service := NewService(probe, "staging", "dev", "gdrive", true, time.Unix(100, 0))
	service.now = func() time.Time { return time.Unix(160, 0) }

	report := service.Check(context.Background())

	if report.Status != StatusHealthy || report.StorageBackend != "gdrive" || report.MediaWorkerStatus != "active" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.MediaMoves.Queued != 3 || report.MediaMoves.Processing != 1 || report.MediaMoves.Retry != 2 || report.MediaMoves.Failed != 4 {
		t.Fatalf("unexpected media moves: %#v", report.MediaMoves)
	}
}

func TestCheckDegradesWhenOperationalStatsCannotBeRead(t *testing.T) {
	probe := &fakeProbe{migrationVersion: 56, statsErr: errors.New("secret database detail")}
	service := NewService(probe, "staging", "dev", "gdrive", true, time.Now())

	report := service.Check(context.Background())

	if report.Status != StatusDegraded || report.Operations.Code != "operational_stats_unavailable" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Operations.Message == "secret database detail" {
		t.Fatalf("raw error leaked: %q", report.Operations.Message)
	}
}

func TestCheckReturnsDegradedWhenMigrationVersionFails(t *testing.T) {
	service := NewService(&fakeProbe{migrationErr: errors.New("driver detail")}, "production", "v1", "local", false, time.Now())
	report := service.Check(context.Background())
	if report.Status != StatusDegraded || report.Database.Code != "migration_state_unavailable" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Database.Message == "driver detail" {
		t.Fatal("raw database error leaked")
	}
}

func TestCheckReturnsUnhealthyWhenPingFails(t *testing.T) {
	service := NewService(&fakeProbe{pingErr: errors.New("postgres://user:password@host/database")}, "production", "v1", "local", false, time.Now())
	report := service.Check(context.Background())
	if report.Status != StatusUnhealthy || report.Database.Code != "database_unavailable" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Database.Message == "postgres://user:password@host/database" {
		t.Fatal("database credential leaked")
	}
}

type fakeProbe struct {
	pingErr          error
	migrationVersion int64
	migrationErr     error
	stats            OperationalStats
	statsErr         error
}

func (f *fakeProbe) Ping(context.Context) error { return f.pingErr }
func (f *fakeProbe) MigrationVersion(context.Context) (int64, error) {
	return f.migrationVersion, f.migrationErr
}
func (f *fakeProbe) OperationalStats(context.Context) (OperationalStats, error) {
	return f.stats, f.statsErr
}
