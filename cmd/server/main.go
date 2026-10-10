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

	"konkit/internal/activities"
	"konkit/internal/administration"
	apihttp "konkit/internal/api"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/bast"
	"konkit/internal/config"
	"konkit/internal/database"
	"konkit/internal/dcp3"
	"konkit/internal/distribution"
	"konkit/internal/health"
	"konkit/internal/media"
	"konkit/internal/profile"
	"konkit/internal/programs"
	"konkit/internal/realtime"
	"konkit/internal/recipients"
	"konkit/internal/reports"
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
	var mediaStorage media.Storage
	switch cfg.StorageBackend {
	case "gdrive":
		driveCache := media.NewPostgresFolderCache(pool)
		mediaStorage, err = media.NewGoogleDriveStorage(ctx, cfg.GDriveOAuthClientID, cfg.GDriveOAuthClientSecret, cfg.GDriveOAuthTokenJSON, cfg.GDriveRootFolderID, driveCache)
		if err != nil {
			return fmt.Errorf("initialize google drive storage: %w", err)
		}
	default:
		mediaStorage, err = media.NewLocalStorage(cfg.StoragePath)
		if err != nil {
			return err
		}
	}
	programService := programs.NewService(programs.NewRepository(pool))
	videoLimiter := media.NewVideoLimiter(cfg.MaxConcurrentVideoUploads)
	distributionRepository := distribution.NewRepository(pool)
	mediaMoveWorkerActive := startMediaMoveWorker(ctx, distributionRepository, mediaStorage)
	applicationLocation, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return fmt.Errorf("load application timezone: %w", err)
	}
	bastRepository := bast.NewRepository(pool)
	bastService := bast.NewBundleService(bastRepository, mediaStorage, applicationLocation)
	bastBrandingService := bast.NewBrandingService(bastRepository, mediaStorage)
	bastScheduleSettingsService := bast.NewScheduleSettingsService(bastRepository)
	bastDP3Service := bast.NewDP3Service(bastRepository, mediaStorage, applicationLocation)
	bastDailyRecapService := bast.NewDailyRecapService(bastRepository, mediaStorage, applicationLocation)
	bastClosingService := bast.NewClosingTitikSerahService(bastRepository, mediaStorage, applicationLocation)
	bastClosingKabupatenService := bast.NewClosingKabupatenService(bastRepository, mediaStorage, applicationLocation)
	bastServisBerkalaService := bast.NewServisBerkalaService(bastRepository, mediaStorage, applicationLocation)
	bastTKDNService := bast.NewTKDNService(bastRepository, mediaStorage)
	bastPemeriksaanService := bast.NewPemeriksaanService(bastRepository, mediaStorage)
	bastItemService := bast.NewItemService(bastRepository)
	bastRakordaService := bast.NewRakordaService(bastRepository, mediaStorage)
	bastSosialisasiService := bast.NewSosialisasiService(bastRepository, mediaStorage)
	bastTraining10Service := bast.NewTraining10Service(bastRepository, mediaStorage)
	bastTraining100Service := bast.NewTraining100Service(bastRepository, mediaStorage)
	// Change signals for the mobile app: Postgres NOTIFY -> WebSocket clients.
	realtimeHub := realtime.NewHub()
	go realtime.Listen(ctx, cfg.DatabaseURL, realtimeHub)

	apiHandler := apihttp.NewHandler(apihttp.Dependencies{
		Auth:              authService,
		Profile:           profile.NewService(profile.NewRepository(pool)),
		Administration:    administration.NewService(administration.NewRepository(pool)),
		Settings:          settings.NewService(settings.NewRepository(pool)),
		Health:            health.NewService(health.NewPostgresProbe(pool), cfg.Env, "dev", cfg.StorageBackend, mediaMoveWorkerActive, time.Now()),
		Audit:             audit.NewRepository(pool),
		Programs:          programService,
		DCP3:              dcp3.NewImportService(dcp3.NewRepository(pool), dcp3.ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100}),
		Distribution:      distribution.NewService(distributionRepository, mediaStorage, videoLimiter),
		Reports:           reports.NewService(reports.NewRepository(pool)),
		Recipients:        recipients.NewService(recipients.NewRepository(pool)),
		Activities:        activities.NewService(activities.NewRepository(pool), mediaStorage, programService, videoLimiter),
		BAST:              bastService,
		BASTBranding:      bastBrandingService,
		BASTSettings:      bastScheduleSettingsService,
		DP3:               bastDP3Service,
		DailyRecap:        bastDailyRecapService,
		ClosingTitikSerah: bastClosingService,
		ClosingKabupaten:  bastClosingKabupatenService,
		ServisBerkala:     bastServisBerkalaService,
		TKDN:              bastTKDNService,
		Pemeriksaan:       bastPemeriksaanService,
		Items:             bastItemService,
		Rakorda:           bastRakordaService,
		Sosialisasi:       bastSosialisasiService,
		Training10:        bastTraining10Service,
		Training100:       bastTraining100Service,
		SessionSecret:     cfg.SessionSecret,
		Realtime:          realtimeHub,
	})
	handler := web.NewHandler(web.Dependencies{
		Auth:                authService,
		API:                 apiHandler,
		SessionSecret:       cfg.SessionSecret,
		SessionCookieSecure: cfg.SessionCookieSecure,
		SessionTTL:          cfg.SessionTTL,
		RememberTTL:         cfg.RememberTTL,
	})
	server := newHTTPServer(cfg.Addr, handler)

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

func startMediaMoveWorker(ctx context.Context, repository distribution.MediaMoveRepository, storage media.Storage) bool {
	movable, ok := storage.(media.MovableStorage)
	if !ok {
		return false
	}
	worker := distribution.NewMediaMoveWorker(repository, movable, distribution.MediaMoveWorkerOptions{})
	go worker.Run(ctx)
	return true
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       31 * time.Minute,
		WriteTimeout:      31 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
}
