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

	"konkit/internal/administration"
	apihttp "konkit/internal/api"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/config"
	"konkit/internal/database"
	"konkit/internal/dcp3"
	"konkit/internal/health"
	"konkit/internal/profile"
	"konkit/internal/programs"
	"konkit/internal/settings"
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
	apiHandler := apihttp.NewHandler(apihttp.Dependencies{
		Auth:           authService,
		Profile:        profile.NewService(profile.NewRepository(pool)),
		Administration: administration.NewService(administration.NewRepository(pool)),
		Settings:       settings.NewService(settings.NewRepository(pool)),
		Health:         health.NewService(health.NewPostgresProbe(pool), cfg.Env, "dev", time.Now()),
		Audit:          audit.NewRepository(pool),
		Programs:       programs.NewService(programs.NewRepository(pool)),
		DCP3:           dcp3.NewImportService(dcp3.NewRepository(pool), dcp3.ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100}),
		SessionSecret:  cfg.SessionSecret,
	})
	handler := web.NewHandler(web.Dependencies{
		Auth:                authService,
		API:                 apiHandler,
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
