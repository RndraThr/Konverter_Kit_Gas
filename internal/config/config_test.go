package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadReadsDotEnvWithoutOverridingProcessEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	dotEnv := []byte("APP_ADDR=:9090\n" +
		"DATABASE_URL=postgres://dotenv/konkit\n" +
		"SESSION_" + "SECRET=" + strings.Repeat("x", 32) + "\n")
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), dotEnv, 0o600); err != nil {
		t.Fatal(err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	restoreEnvironment(t, "APP_ADDR", "DATABASE_URL", "SESSION_SECRET")
	if err := os.Setenv("APP_ADDR", ":7070"); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("SESSION_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":7070" {
		t.Fatalf("expected process environment to win, got %q", cfg.Addr)
	}
	if cfg.DatabaseURL != "postgres://dotenv/konkit" {
		t.Fatalf("expected DATABASE_URL from .env, got %q", cfg.DatabaseURL)
	}
}

func TestLoadFromRejectsMissingDatabaseURL(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"SESSION_SECRET": "01234567890123456789012345678901",
	}))
	if !errors.Is(err, ErrDatabaseURLRequired) {
		t.Fatalf("expected ErrDatabaseURLRequired, got %v", err)
	}
}

func TestLoadFromRejectsShortSessionSecret(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"DATABASE_URL":   "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET": "too-short",
	}))
	if !errors.Is(err, ErrSessionSecretTooShort) {
		t.Fatalf("expected ErrSessionSecretTooShort, got %v", err)
	}
}

func TestLoadFromRejectsInvalidCookieBoolean(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"DATABASE_URL":          "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET":        "01234567890123456789012345678901",
		"SESSION_COOKIE_SECURE": "sometimes",
	}))
	if !errors.Is(err, ErrCookieSecureInvalid) {
		t.Fatalf("expected ErrCookieSecureInvalid, got %v", err)
	}
}

func TestLoadFromRejectsInvalidBaseURL(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"APP_BASE_URL":   "://invalid",
		"DATABASE_URL":   "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET": "01234567890123456789012345678901",
	}))
	if !errors.Is(err, ErrBaseURLInvalid) {
		t.Fatalf("expected ErrBaseURLInvalid, got %v", err)
	}
}

func TestLoadFromRejectsNonPositiveSessionTTL(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"DATABASE_URL":   "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET": "01234567890123456789012345678901",
		"SESSION_TTL":    "0s",
	}))
	if !errors.Is(err, ErrSessionTTLInvalid) {
		t.Fatalf("expected ErrSessionTTLInvalid, got %v", err)
	}
}

func TestLoadFromParsesLocalConfiguration(t *testing.T) {
	cfg, err := loadFrom(mapLookup(map[string]string{
		"APP_ENV":               "local",
		"APP_ADDR":              ":8080",
		"APP_BASE_URL":          "http://localhost:8080",
		"DATABASE_URL":          "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET":        "01234567890123456789012345678901",
		"SESSION_COOKIE_SECURE": "false",
		"SESSION_TTL":           "12h",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "local" || cfg.Addr != ":8080" || cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("unexpected application config: %+v", cfg)
	}
	if cfg.SessionTTL != 12*time.Hour || cfg.RememberTTL != 30*24*time.Hour {
		t.Fatalf("unexpected durations: %+v", cfg)
	}
	if cfg.SessionCookieSecure {
		t.Fatal("expected secure cookie to be disabled locally")
	}
	if cfg.StoragePath != "./storage" {
		t.Fatalf("unexpected local storage path: %q", cfg.StoragePath)
	}
}

func TestLoadFromRequiresAbsoluteStoragePathOutsideLocal(t *testing.T) {
	_, err := loadFrom(mapLookup(map[string]string{
		"APP_ENV":        "production",
		"DATABASE_URL":   "postgres://postgres:secret@127.0.0.1:5432/konkit?sslmode=disable",
		"SESSION_SECRET": "01234567890123456789012345678901",
		"STORAGE_PATH":   "./storage",
	}))
	if !errors.Is(err, ErrStoragePathAbsolute) {
		t.Fatalf("expected ErrStoragePathAbsolute, got %v", err)
	}
}

func mapLookup(values map[string]string) lookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func restoreEnvironment(t *testing.T, keys ...string) {
	t.Helper()

	type value struct {
		text string
		set  bool
	}
	original := make(map[string]value, len(keys))
	for _, key := range keys {
		text, set := os.LookupEnv(key)
		original[key] = value{text: text, set: set}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			item := original[key]
			if item.set {
				_ = os.Setenv(key, item.text)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
