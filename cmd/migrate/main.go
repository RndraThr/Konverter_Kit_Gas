package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"konkit/internal/config"
	"konkit/internal/database/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if len(os.Args) != 2 || !validCommand(os.Args[1]) {
		log.Fatal("usage: go run ./cmd/migrate up|down|status")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.RunContext(context.Background(), os.Args[1], db, "."); err != nil {
		log.Fatal(fmt.Errorf("run migration %s: %w", os.Args[1], err))
	}
}

func validCommand(command string) bool {
	switch command {
	case "up", "down", "status":
		return true
	default:
		return false
	}
}
