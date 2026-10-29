package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"konkit/internal/activities"
	"konkit/internal/administration"
	apihttp "konkit/internal/api"
	"konkit/internal/appreleases"
	"konkit/internal/audit"
	"konkit/internal/auth"
	"konkit/internal/bast"
	"konkit/internal/config"
	"konkit/internal/database"
	"konkit/internal/dcp3"
	"konkit/internal/distmap"
	"konkit/internal/distribution"
	"konkit/internal/finalreport"
	finalreporttemplate "konkit/internal/finalreport/template"
	"konkit/internal/geo"
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

	// The distribution map is non-critical: a failed (or stuck, e.g. behind a lock) boundary import
	// is logged, not fatal, and may not hold up startup for more than a minute.
	boundaryCtx, cancelBoundaries := context.WithTimeout(ctx, 60*time.Second)
	boundaries, err := geo.EnsureRegencyBoundaries(boundaryCtx, pool)
	cancelBoundaries()
	if err != nil {
		log.Printf("regency boundaries: import failed: %v", err)
	} else {
		log.Printf("regency boundaries: imported=%d matched=%d unmatched=%d", boundaries.Imported, boundaries.Matched, len(boundaries.Unmatched))
		if len(boundaries.Unmatched) > 0 {
			log.Printf("regency boundaries: master regencies without a boundary: %v", boundaries.Unmatched)
		}
	}

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
	// Zone boundaries are cached per programme; any change to zones or assignments drops that cache.
	distmapRepository := distmap.NewRepository(pool)
	programService.OnZoneAssignmentsChanged(distmapRepository.InvalidateZones)
	videoLimiter := media.NewVideoLimiter(cfg.MaxConcurrentVideoUploads)
	distributionRepository := distribution.NewRepository(pool)
	mediaMoveWorkerActive := startMediaMoveWorker(ctx, distributionRepository, mediaStorage)
	applicationLocation, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return fmt.Errorf("load application timezone: %w", err)
	}
	bastRepository := bast.NewRepository(pool)
	finalReportRepository := finalreport.NewRepository(pool)
	finalReportService := finalreport.NewService(finalReportRepository, programService, mediaStorage, finalreporttemplate.PPTX)
	bastService := bast.NewBundleService(bastRepository, mediaStorage, applicationLocation)
	bastBrandingService := bast.NewBrandingService(bastRepository, mediaStorage)
	bastScheduleSettingsService := bast.NewScheduleSettingsService(bastRepository)
	bastDP3Service := bast.NewDP3Service(bastRepository, mediaStorage, applicationLocation)
	bastDailyRecapService := bast.NewDailyRecapService(bastRepository, mediaStorage, applicationLocation)
	bastClosingService := bast.NewClosingTitikSerahService(bastRepository, mediaStorage, applicationLocation)
	bastClosingKabupatenService := bast.NewClosingKabupatenService(bastRepository, mediaStorage, applicationLocation)
	bastServisBerkalaService := bast.NewServisBerkalaService(bastRepository, mediaStorage, applicationLocation)
	bastGasElpijiService := bast.NewGasElpijiService(bastRepository, mediaStorage)
	bastTKDNService := bast.NewTKDNService(bastRepository, mediaStorage)
	bastPemeriksaanService := bast.NewPemeriksaanService(bastRepository, mediaStorage)
	bastItemService := bast.NewItemService(bastRepository)
	bastRakordaService := bast.NewRakordaService(bastRepository, mediaStorage)
	bastSosialisasiService := bast.NewSosialisasiService(bastRepository, mediaStorage)
	bastTraining10Service := bast.NewTraining10Service(bastRepository, mediaStorage)
	bastTraining100Service := bast.NewTraining100Service(bastRepository, mediaStorage)
	// Mobile app releases (APK updates outside the Play Store) always stay on
	// the server's own disk, whatever the media backend.
	appReleaseStorage, err := media.NewLocalStorage(filepath.Join(cfg.StoragePath, "app-releases"))
	if err != nil {
		return err
	}
	// Change signals for the mobile app: Postgres NOTIFY -> WebSocket clients.
	realtimeHub := realtime.NewHub()
	go realtime.Listen(ctx, cfg.DatabaseURL, realtimeHub)

	apiHandler := apihttp.NewHandler(apihttp.Dependencies{
		Auth:            authService,
		AppReleases:     appreleases.NewService(pool, appReleaseStorage),
		FinalReport:     finalReportService,
		Geo:             geo.NewReverser(cfg.GeocoderURL, &http.Client{Timeout: 5 * time.Second}, "Konkit/1.0 (Konverter Kit Gas; PT Kian Santang Muliatama Tbk)"),
		Profile:         profile.NewService(profile.NewRepository(pool)),
		Administration:  administration.NewService(administration.NewRepository(pool)),
		Settings:        settings.NewService(settings.NewRepository(pool)),
		Health:          health.NewService(health.NewPostgresProbe(pool), cfg.Env, "dev", cfg.StorageBackend, mediaMoveWorkerActive, time.Now()),
		Audit:           audit.NewRepository(pool),
		Programs:        programService,
		DCP3:            dcp3.NewImportService(dcp3.NewRepository(pool), dcp3.ParseLimits{MaxBytes: 10 << 20, MaxRows: 5000, MaxColumns: 100}),
		Distribution:    distribution.NewService(distributionRepository, mediaStorage, videoLimiter),
		Reports:         reports.NewService(reports.NewRepository(pool)),
		Recipients:      recipients.NewService(recipients.NewRepository(pool)),
		DistributionMap: distmap.NewService(distmapRepository),
		MapConfig: distmap.ClientConfig{
			StyleURL: cfg.MapStyleURL, SatelliteTiles: cfg.MapSatelliteTiles, HybridOverlayTiles: cfg.MapHybridOverlayTiles,
			Attribution: cfg.MapAttribution, APIKey: cfg.MapTileAPIKey,
		},
		Activities:    activities.NewService(activities.NewRepository(pool), mediaStorage, programService, videoLimiter),
		BAST:          bastService,
		BASTBranding:  bastBrandingService,
		BASTSettings:  bastScheduleSettingsService,
		BASTSignatory: bast.NewSignatoryService(bastRepository),
		BASTSignatureLayout: bast.NewSignatureLayoutService(map[string]bast.WetPreviewer{
			"dp3":                 bastDP3Service,
			"ba-perorangan":       bast.PreviewFunc(bastService.SchedulePreview),
			"rekap-harian":        bastDailyRecapService,
			"closing-titik-serah": bastClosingService,
			"closing-kabupaten":   bastClosingKabupatenService,
			"sosialisasi":         bastSosialisasiService,
			"training-10":         bastTraining10Service,
			"training-100":        bastTraining100Service,
			"ba-pemeriksaan":      bast.PreviewFunc(bastPemeriksaanService.SignaturePreview),
			"servis-berkala":      bastServisBerkalaService,
			"tkdn":                bastTKDNService,
			"gas-elpiji":          bast.PreviewFunc(bastGasElpijiService.SignaturePreview),
		}),
		BASTOverview: bastRepository,
		BASTHandover: bast.NewHandoverService(bastRepository),
		BASTWetSignature: bast.NewWetSignatureService(bastRepository, map[string]bast.WetPreviewer{
			"dp3":                 bastDP3Service,
			"rekap-harian":        bastDailyRecapService,
			"closing-titik-serah": bastClosingService,
			"closing-kabupaten":   bastClosingKabupatenService,
			"servis-berkala":      bastServisBerkalaService,
			"tkdn":                bastTKDNService,
			"gas-elpiji":          bast.PreviewFunc(bastGasElpijiService.SignaturePreview),
			"sosialisasi":         bastSosialisasiService,
			"training-10":         bastTraining10Service,
			"training-100":        bastTraining100Service,
			"ba-pemeriksaan":      bast.PreviewFunc(bastPemeriksaanService.SignaturePreview),
		}),
		DP3:               bastDP3Service,
		DailyRecap:        bastDailyRecapService,
		ClosingTitikSerah: bastClosingService,
		ClosingKabupaten:  bastClosingKabupatenService,
		ServisBerkala:     bastServisBerkalaService,
		GasElpiji:         bastGasElpijiService,
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
