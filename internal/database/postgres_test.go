package database

import (
	"context"
	"testing"
)

func TestOpenRejectsInvalidURL(t *testing.T) {
	_, err := Open(context.Background(), "://invalid")
	if err == nil {
		t.Fatal("expected invalid database URL error")
	}
}
