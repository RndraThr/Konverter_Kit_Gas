package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/config"
	"konkit/internal/database"
)

const (
	seedUsername = "e2e.admin"
	seedEmail    = "e2e.admin@konkit.test"
	seedPassword = "Konkit-E2E-Password-2026"
)

func main() {
	cleanup := len(os.Args) == 2 && os.Args[1] == "cleanup"
	if len(os.Args) > 2 || (len(os.Args) == 2 && !cleanup) {
		log.Fatal("usage: go run ./cmd/e2eseed [cleanup]")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := validateSeedTarget(cfg.Env, cfg.DatabaseURL); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "DELETE FROM users WHERE username LIKE 'petugas.e2e.%' OR username = $1", seedUsername); err != nil {
		log.Fatal(err)
	}
	if cleanup {
		if err := tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
		_, _ = fmt.Fprintln(os.Stdout, "E2E accounts removed")
		return
	}
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		log.Fatal(err)
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (full_name, username, email, password_hash, is_active)
		VALUES ('Admin E2E', $1, $2, $3, true)
		RETURNING id::text
	`, seedUsername, seedEmail, hash).Scan(&userID)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = 'super_admin'
		ON CONFLICT DO NOTHING
	`, userID); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "E2E account ready")
}

func validateSeedTarget(environment, databaseURL string) error {
	if environment != "test" {
		return fmt.Errorf("e2e seed requires APP_ENV=test")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || strings.TrimPrefix(parsed.Path, "/") != "konkit_test" {
		return fmt.Errorf("e2e seed requires the konkit_test database")
	}
	return nil
}
