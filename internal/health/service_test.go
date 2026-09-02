package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheckReturnsHealthyReport(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	service := NewService(&fakeProbe{migrationVersion: 2}, "local", "dev", now.Add(-90*time.Second))
	service.now = func() time.Time { return now }

	report := service.Check(context.Background())
	if report.Status != StatusHealthy || report.Database.Status != StatusHealthy {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.MigrationVersion != 2 || report.UptimeSeconds != 90 || report.CheckedAt.Location() != time.UTC {
		t.Fatalf("unexpected report details: %+v", report)
	}
}

func TestCheckReturnsDegradedWhenMigrationVersionFails(t *testing.T) {
	service := NewService(&fakeProbe{migrationErr: errors.New("driver detail")}, "production", "v1", time.Now())
	report := service.Check(context.Background())
	if report.Status != StatusDegraded || report.Database.Code != "migration_state_unavailable" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Database.Message == "driver detail" {
		t.Fatal("raw database error leaked")
	}
}

func TestCheckReturnsUnhealthyWhenPingFails(t *testing.T) {
	service := NewService(&fakeProbe{pingErr: errors.New("postgres://user:password@host/database")}, "production", "v1", time.Now())
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
}

func (f *fakeProbe) Ping(context.Context) error { return f.pingErr }
func (f *fakeProbe) MigrationVersion(context.Context) (int64, error) {
	return f.migrationVersion, f.migrationErr
}
