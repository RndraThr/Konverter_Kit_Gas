package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecipientFilterIncludesZone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients?zone_id=zone-1", nil)
	filter := recipientFilterFromRequest(req)
	if filter.ZoneID != "zone-1" {
		t.Fatalf("expected zone_id forwarded to recipient filter, got %+v", filter)
	}
}

func TestRecipientFilterAcceptsAllPageSize(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients?page=7&page_size=all", nil)
	filter := recipientFilterFromRequest(req)

	if !filter.All {
		t.Fatalf("expected page_size=all to enable all results, got %+v", filter)
	}
}

func TestRecipientFilterDefaultsToFiftyResults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipients", nil)
	filter := recipientFilterFromRequest(req)

	if filter.PageSize != 50 {
		t.Fatalf("expected page_size=50 by default, got %+v", filter)
	}
}
