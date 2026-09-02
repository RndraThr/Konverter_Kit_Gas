package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "konkit/internal/api"
	"konkit/internal/auth"
	"konkit/internal/config"
	"konkit/internal/database"
	"konkit/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repository := auth.NewRepository(pool)
	authService := auth.NewService(repository, cfg.SessionTTL, cfg.RememberTTL)
	handler := web.NewHandler(web.Dependencies{
		Auth:                authService,
		API:                 apihttp.NewHandler(),
		SessionSecret:       cfg.SessionSecret,
		SessionCookieSecure: cfg.SessionCookieSecure,
		SessionTTL:          cfg.SessionTTL,
		RememberTTL:         cfg.RememberTTL,
	})
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Konkit web server listening on %s", cfg.BaseURL)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		return nil
	}
}
