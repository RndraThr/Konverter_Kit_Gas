package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

var (
	ErrDatabaseURLRequired   = errors.New("DATABASE_URL is required")
	ErrSessionSecretTooShort = errors.New("SESSION_SECRET must be at least 32 bytes")
	ErrCookieSecureInvalid   = errors.New("SESSION_COOKIE_SECURE must be true or false")
	ErrSessionTTLInvalid     = errors.New("SESSION_TTL must be a positive duration")
	ErrBaseURLInvalid        = errors.New("APP_BASE_URL must be an absolute HTTP or HTTPS URL")
)

type Config struct {
	Env                 string
	Addr                string
	BaseURL             string
	DatabaseURL         string
	SessionSecret       []byte
	SessionCookieSecure bool
	SessionTTL          time.Duration
	RememberTTL         time.Duration
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
		Env:         valueOrDefault(lookup, "APP_ENV", "local"),
		Addr:        valueOrDefault(lookup, "APP_ADDR", ":8080"),
		BaseURL:     valueOrDefault(lookup, "APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL: valueOrDefault(lookup, "DATABASE_URL", ""),
		SessionTTL:  12 * time.Hour,
		RememberTTL: 30 * 24 * time.Hour,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, ErrDatabaseURLRequired
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

	return cfg, nil
}

func valueOrDefault(lookup lookupFunc, key, fallback string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return fallback
}
