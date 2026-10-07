package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"konkit/internal/config"
	"konkit/internal/distribution"
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

type workerRepositoryProbe struct{ claimed chan struct{} }

func (r *workerRepositoryProbe) ClaimMediaMove(context.Context, time.Time, time.Duration) (distribution.MediaMoveJob, bool, error) {
	select {
	case r.claimed <- struct{}{}:
	default:
	}
	return distribution.MediaMoveJob{}, false, nil
}
func (*workerRepositoryProbe) CompleteMediaMove(context.Context, string, int64) error { return nil }
func (*workerRepositoryProbe) FailMediaMove(context.Context, string, int64, string, time.Time) error {
	return nil
}

type serverStorageProbe struct{ movable bool }

func (*serverStorageProbe) Put(context.Context, string, []string, io.Reader) (string, int64, string, error) {
	return "key", 0, "checksum", nil
}
func (*serverStorageProbe) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (*serverStorageProbe) Delete(context.Context, string) error            { return nil }
func (*serverStorageProbe) EnsureFolders(context.Context, [][]string) error { return nil }

type movableServerStorageProbe struct{ serverStorageProbe }

func (*movableServerStorageProbe) Move(context.Context, string, []string) error { return nil }

func TestStartMediaMoveWorkerOnlyForMovableStorage(t *testing.T) {
	repository := &workerRepositoryProbe{claimed: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startMediaMoveWorker(ctx, repository, &serverStorageProbe{}) {
		t.Fatal("worker started for storage without move support")
	}
	if !startMediaMoveWorker(ctx, repository, &movableServerStorageProbe{}) {
		t.Fatal("worker did not start for movable storage")
	}
	select {
	case <-repository.claimed:
	case <-time.After(time.Second):
		t.Fatal("worker did not poll repository")
	}
}
