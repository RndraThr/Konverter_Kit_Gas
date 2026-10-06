package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"konkit/internal/config"
)

func TestRunReturnsDatabaseConfigurationError(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:   "://invalid",
		SessionSecret: []byte("01234567890123456789012345678901"),
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected invalid database URL error")
	}
}

func TestRunFailsFastWhenGoogleDriveCredentialsAreInvalid(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	cfg := config.Config{
		DatabaseURL:             databaseURL,
		SessionSecret:           []byte("01234567890123456789012345678901"),
		StorageBackend:          "gdrive",
		GDriveOAuthClientID:     "irrelevant-for-this-test",
		GDriveOAuthClientSecret: "irrelevant-for-this-test",
		GDriveOAuthTokenJSON:    "/nonexistent/path/token.json",
		GDriveRootFolderID:      "irrelevant-for-this-test",
	}
	err := run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error when Google Drive OAuth token file does not exist")
	}
}

func TestNewHTTPServerUsesUploadSafeTimeouts(t *testing.T) {
	server := newHTTPServer(":8080", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 31*time.Minute || server.WriteTimeout != 31*time.Minute || server.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected server timeouts: header=%v read=%v write=%v idle=%v", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}
