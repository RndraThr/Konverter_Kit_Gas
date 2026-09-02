package audit

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"konkit/internal/database"
	"konkit/internal/database/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestIntegrationRepositoryListsNewestFilteredEvents(t *testing.T) {
	pool := auditIntegrationPool(t)
	ctx := context.Background()
	const action = "audit.integration.updated"
	_, _ = pool.Exec(ctx, "DELETE FROM audit_logs WHERE action = $1", action)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE action = $1", action) })

	for _, resourceID := range []string{"old", "new"} {
		if err := Record(ctx, pool, Event{
			Action:       action,
			ResourceType: "integration",
			ResourceID:   resourceID,
			Metadata:     map[string]any{"position": resourceID},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_logs SET created_at = $1 WHERE action = $2 AND resource_id = 'old'", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), action); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_logs SET created_at = $1 WHERE action = $2 AND resource_id = 'new'", time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), action); err != nil {
		t.Fatal(err)
	}

	page, err := NewRepository(pool).List(ctx, Filter{Action: action, Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ResourceID != "new" {
		t.Fatalf("unexpected audit page: %+v", page)
	}
	if page.Items[0].Metadata["position"] != "new" {
		t.Fatalf("unexpected metadata: %+v", page.Items[0].Metadata)
	}
}

func auditIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require database konkit_test, got %q", config.ConnConfig.Database)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		db.Close()
		t.Fatal(err)
	}
	_ = db.Close()

	pool, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
