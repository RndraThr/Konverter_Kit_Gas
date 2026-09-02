package main

import (
	"context"
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
