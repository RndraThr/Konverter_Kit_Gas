package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHealthRejectsPostWithJSONResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if rec.Header().Get("Location") != "" {
		t.Fatal("API errors must not redirect to HTML routes")
	}
}
