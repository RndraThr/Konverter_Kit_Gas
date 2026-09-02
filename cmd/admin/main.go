package main

import (
	"context"
	"fmt"
	"log"
	"os"

	admincommand "konkit/internal/admin"
	"konkit/internal/auth"
	"konkit/internal/config"
	"konkit/internal/database"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "create" {
		log.Fatal("usage: go run ./cmd/admin create")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := auth.NewRepository(pool)
	readPassword := func(prompt string) (string, error) {
		if _, err := fmt.Fprint(os.Stdout, prompt); err != nil {
			return "", err
		}
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stdout)
		return string(password), err
	}

	if err := admincommand.RunCreate(ctx, os.Stdin, os.Stdout, readPassword, repository); err != nil {
		log.Fatal(err)
	}
}
