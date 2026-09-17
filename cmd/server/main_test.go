package main

import (
	"context"
	"os"
	"testing"

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
		DatabaseURL:              databaseURL,
		SessionSecret:            []byte("01234567890123456789012345678901"),
		StorageBackend:           "gdrive",
		GDriveServiceAccountJSON: "/nonexistent/path/credentials.json",
		GDriveRootFolderID:       "irrelevant-for-this-test",
	}
	err := run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error when Google Drive credentials file does not exist")
	}
}
