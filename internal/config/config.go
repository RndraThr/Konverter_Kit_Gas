package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	ErrDatabaseURLRequired              = errors.New("DATABASE_URL is required")
	ErrSessionSecretTooShort            = errors.New("SESSION_SECRET must be at least 32 bytes")
	ErrCookieSecureInvalid              = errors.New("SESSION_COOKIE_SECURE must be true or false")
	ErrSessionTTLInvalid                = errors.New("SESSION_TTL must be a positive duration")
	ErrBaseURLInvalid                   = errors.New("APP_BASE_URL must be an absolute HTTP or HTTPS URL")
	ErrStoragePathAbsolute              = errors.New("STORAGE_PATH must be absolute outside local environment")
	ErrStorageBackendInvalid            = errors.New("STORAGE_BACKEND must be 'local' or 'gdrive'")
	ErrGDriveSettingsIncomplete         = errors.New("GDRIVE_OAUTH_CLIENT_ID, GDRIVE_OAUTH_CLIENT_SECRET, GDRIVE_OAUTH_TOKEN_JSON, and GDRIVE_ROOT_FOLDER_ID are required when STORAGE_BACKEND=gdrive")
	ErrMaxConcurrentVideoUploadsInvalid = errors.New("MAX_CONCURRENT_VIDEO_UPLOADS must be an integer between 1 and 16")
)

type Config struct {
	Env                       string
	Addr                      string
	BaseURL                   string
	DatabaseURL               string
	SessionSecret             []byte
	SessionCookieSecure       bool
	SessionTTL                time.Duration
	RememberTTL               time.Duration
	StoragePath               string
	StorageBackend            string
	GDriveOAuthClientID       string
	GDriveOAuthClientSecret   string
	GDriveOAuthTokenJSON      string
	GDriveRootFolderID        string
	MaxConcurrentVideoUploads int
	GeocoderURL               string
	// Map tile sources handed to the browser by /api/v1/map/config. The API key is a public client key.
	MapStyleURL           string
	MapSatelliteTiles     string
	MapHybridOverlayTiles []string
	MapAttribution        string
	MapTileAPIKey         string
}

const (
	defaultMapStyleURL       = "https://tiles.openfreemap.org/styles/liberty"
	defaultMapSatelliteTiles = "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}"
	defaultMapAttribution    = "© OpenStreetMap contributors · Imagery © Esri · Batas wilayah: geoBoundaries (CC BY 3.0 IGO)"
)

var defaultMapHybridOverlayTiles = []string{
	"https://server.arcgisonline.com/ArcGIS/rest/services/Reference/World_Transportation/MapServer/tile/{z}/{y}/{x}",
	"https://server.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
}

// splitList splits a comma-separated list, trimming blanks and dropping empty items.
func splitList(raw string) []string {
	items := []string{}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

type lookupFunc func(string) (string, bool)

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	return loadFrom(os.LookupEnv)
}

func loadFrom(lookup lookupFunc) (Config, error) {
	cfg := Config{
		Env:                       valueOrDefault(lookup, "APP_ENV", "local"),
		Addr:                      valueOrDefault(lookup, "APP_ADDR", ":8080"),
		BaseURL:                   valueOrDefault(lookup, "APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL:               valueOrDefault(lookup, "DATABASE_URL", ""),
		SessionTTL:                12 * time.Hour,
		RememberTTL:               30 * 24 * time.Hour,
		StoragePath:               valueOrDefault(lookup, "STORAGE_PATH", "./storage"),
		StorageBackend:            valueOrDefault(lookup, "STORAGE_BACKEND", "local"),
		MaxConcurrentVideoUploads: 3,
		GeocoderURL:               valueOrDefault(lookup, "GEOCODER_URL", "https://nominatim.openstreetmap.org"),
		MapStyleURL:               valueOrDefault(lookup, "MAP_STYLE_URL", defaultMapStyleURL),
		MapSatelliteTiles:         valueOrDefault(lookup, "MAP_SATELLITE_TILES", defaultMapSatelliteTiles),
		MapHybridOverlayTiles:     splitList(valueOrDefault(lookup, "MAP_HYBRID_OVERLAY_TILES", strings.Join(defaultMapHybridOverlayTiles, ","))),
		MapAttribution:            valueOrDefault(lookup, "MAP_ATTRIBUTION", defaultMapAttribution),
		MapTileAPIKey:             valueOrDefault(lookup, "MAP_TILE_API_KEY", ""),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, ErrDatabaseURLRequired
	}
	if cfg.Env != "local" && !filepath.IsAbs(cfg.StoragePath) {
		return Config{}, ErrStoragePathAbsolute
	}

	if cfg.StorageBackend != "local" && cfg.StorageBackend != "gdrive" {
		return Config{}, ErrStorageBackendInvalid
	}
	if cfg.StorageBackend == "gdrive" {
		cfg.GDriveOAuthClientID, _ = lookup("GDRIVE_OAUTH_CLIENT_ID")
		cfg.GDriveOAuthClientSecret, _ = lookup("GDRIVE_OAUTH_CLIENT_SECRET")
		cfg.GDriveOAuthTokenJSON, _ = lookup("GDRIVE_OAUTH_TOKEN_JSON")
		cfg.GDriveRootFolderID, _ = lookup("GDRIVE_ROOT_FOLDER_ID")
		if cfg.GDriveOAuthClientID == "" || cfg.GDriveOAuthClientSecret == "" || cfg.GDriveOAuthTokenJSON == "" || cfg.GDriveRootFolderID == "" {
			return Config{}, ErrGDriveSettingsIncomplete
		}
	}

	secret, _ := lookup("SESSION_SECRET")
	if len([]byte(secret)) < 32 {
		return Config{}, ErrSessionSecretTooShort
	}
	cfg.SessionSecret = []byte(secret)

	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return Config{}, ErrBaseURLInvalid
	}

	if raw, ok := lookup("SESSION_COOKIE_SECURE"); ok && raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: %q", ErrCookieSecureInvalid, raw)
		}
		cfg.SessionCookieSecure = secure
	}

	if raw, ok := lookup("SESSION_TTL"); ok && raw != "" {
		duration, err := time.ParseDuration(raw)
		if err != nil || duration <= 0 {
			return Config{}, fmt.Errorf("%w: %q", ErrSessionTTLInvalid, raw)
		}
		cfg.SessionTTL = duration
	}

	if raw, ok := lookup("MAX_CONCURRENT_VIDEO_UPLOADS"); ok && raw != "" {
		maximum, err := strconv.Atoi(raw)
		if err != nil || maximum < 1 || maximum > 16 {
			return Config{}, fmt.Errorf("%w: %q", ErrMaxConcurrentVideoUploadsInvalid, raw)
		}
		cfg.MaxConcurrentVideoUploads = maximum
	}

	return cfg, nil
}

func valueOrDefault(lookup lookupFunc, key, fallback string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return fallback
}
